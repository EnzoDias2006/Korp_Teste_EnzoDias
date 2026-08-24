package product

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// testDB holds the test database connection
var testDB *pgxpool.Pool
var testDatabaseName string
var testAdminConn *pgx.Conn

// TestMain creates an isolated PostgreSQL database for this package.
func TestMain(m *testing.M) {
	dbURL := os.Getenv("STOCK_TEST_DATABASE_URL")
	if dbURL == "" {
		// Skip integration tests if database URL is not set
		os.Exit(m.Run())
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	config, err := pgxpool.ParseConfig(dbURL)
	if err != nil {
		os.Stderr.WriteString("Invalid database configuration: " + err.Error() + "\n")
		os.Exit(1)
	}

	adminConfig := config.ConnConfig.Copy()
	adminConfig.Database = "postgres"
	testAdminConn, err = pgx.ConnectConfig(ctx, adminConfig)
	if err != nil {
		os.Stderr.WriteString("Failed to connect stock test administrator: " + err.Error() + "\n")
		os.Exit(1)
	}

	testDatabaseName = fmt.Sprintf("stock_product_test_%d", time.Now().UnixNano())
	if _, err = testAdminConn.Exec(ctx, fmt.Sprintf(`DROP DATABASE IF EXISTS %q`, testDatabaseName)); err != nil {
		os.Stderr.WriteString("Failed to drop isolated stock test database: " + err.Error() + "\n")
		os.Exit(1)
	}
	if _, err = testAdminConn.Exec(ctx, fmt.Sprintf(`CREATE DATABASE %q`, testDatabaseName)); err != nil {
		os.Stderr.WriteString("Failed to create isolated stock test database: " + err.Error() + "\n")
		os.Exit(1)
	}

	config.ConnConfig.Database = testDatabaseName
	testDB, err = pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		os.Stderr.WriteString("Failed to connect isolated stock test database: " + err.Error() + "\n")
		os.Exit(1)
	}
	if err = testDB.Ping(ctx); err != nil {
		os.Stderr.WriteString("Failed to ping isolated stock test database: " + err.Error() + "\n")
		os.Exit(1)
	}
	if _, err = testDB.Exec(ctx, readProductMigration()); err != nil {
		os.Stderr.WriteString("Failed to apply product migration: " + err.Error() + "\n")
		os.Exit(1)
	}
	if _, err = testDB.Exec(ctx, readMigration("000002_create_consumption_operations.up.sql")); err != nil {
		os.Stderr.WriteString("Failed to apply consumption operations migration: " + err.Error() + "\n")
		os.Exit(1)
	}
	if _, err = testDB.Exec(ctx, readMigration("000003_create_consumption_operation_results.up.sql")); err != nil {
		os.Stderr.WriteString("Failed to apply consumption operation results migration: " + err.Error() + "\n")
		os.Exit(1)
	}

	code := m.Run()

	testDB.Close()
	cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
	if _, err = testAdminConn.Exec(cleanupCtx, fmt.Sprintf(`DROP DATABASE IF EXISTS %q WITH (FORCE)`, testDatabaseName)); err != nil {
		os.Stderr.WriteString("Failed to remove isolated stock test database: " + err.Error() + "\n")
		code = 1
	}
	cleanupCancel()
	testAdminConn.Close(context.Background())
	os.Exit(code)
}

func readProductMigration() string { return readMigration("000001_create_products.up.sql") }

func readMigration(name string) string {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		os.Stderr.WriteString("Failed to resolve product repository test path\n")
		os.Exit(1)
	}
	migrationPath := filepath.Join(filepath.Dir(currentFile), "..", "..", "migrations", name)
	migration, err := os.ReadFile(migrationPath)
	if err != nil {
		os.Stderr.WriteString("Failed to read product migration: " + err.Error() + "\n")
		os.Exit(1)
	}

	return string(migration)
}

// cleanupTestData removes all test data from the products table.
func cleanupTestData(ctx context.Context) error {
	_, err := testDB.Exec(ctx, `TRUNCATE TABLE consumption_operations, products RESTART IDENTITY CASCADE`)
	return err
}

func TestRepository_Create_Success(t *testing.T) {
	if testDB == nil {
		t.Skip("STOCK_TEST_DATABASE_URL not set, skipping integration test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	repo := NewRepository(testDB)

	// Clean up before test
	if err := cleanupTestData(ctx); err != nil {
		t.Fatalf("Failed to cleanup test data: %v", err)
	}

	input := CreateInput{
		Code:        "TEST001",
		Description: "Test Product",
		Balance:     100,
	}

	product, err := repo.Create(ctx, input.Code, input.Description, input.Balance)
	if err != nil {
		t.Fatalf("Failed to create product: %v", err)
	}

	if product.ID == 0 {
		t.Error("Expected product ID to be set")
	}
	if product.Code != "TEST001" {
		t.Errorf("Expected code 'TEST001', got '%s'", product.Code)
	}
	if product.Description != "Test Product" {
		t.Errorf("Expected description 'Test Product', got '%s'", product.Description)
	}
	if product.Balance != 100 {
		t.Errorf("Expected balance 100, got %d", product.Balance)
	}
	if product.CreatedAt.IsZero() {
		t.Error("Expected CreatedAt to be set")
	}
	if product.UpdatedAt.IsZero() {
		t.Error("Expected UpdatedAt to be set")
	}
}

func TestRepository_Create_DuplicateCode(t *testing.T) {
	if testDB == nil {
		t.Skip("STOCK_TEST_DATABASE_URL not set, skipping integration test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	repo := NewRepository(testDB)

	// Clean up before test
	if err := cleanupTestData(ctx); err != nil {
		t.Fatalf("Failed to cleanup test data: %v", err)
	}

	// Create first product
	_, err := repo.Create(ctx, "DUP001", "First Product", 50)
	if err != nil {
		t.Fatalf("Failed to create first product: %v", err)
	}

	// Try to create duplicate
	_, err = repo.Create(ctx, "DUP001", "Second Product", 50)
	if err == nil {
		t.Fatal("Expected error for duplicate code")
	}
	if !errors.Is(err, ErrCodeConflict) {
		t.Errorf("Expected ErrCodeConflict, got %v", err)
	}
}

func TestRepository_Create_NegativeBalanceConstraint(t *testing.T) {
	if testDB == nil {
		t.Skip("STOCK_TEST_DATABASE_URL not set, skipping integration test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := cleanupTestData(ctx); err != nil {
		t.Fatalf("Failed to cleanup test data: %v", err)
	}

	_, err := testDB.Exec(ctx, `
		INSERT INTO products (code, description, balance)
		VALUES ('NEGATIVE', 'Invalid balance', -1)
	`)
	if err == nil {
		t.Fatal("Expected database check constraint to reject a negative balance")
	}
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23514" || pgErr.ConstraintName != "products_balance_check" {
		t.Fatalf("Expected products_balance_check violation, got %v", err)
	}
}

func TestRepository_List_Empty(t *testing.T) {
	if testDB == nil {
		t.Skip("STOCK_TEST_DATABASE_URL not set, skipping integration test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	repo := NewRepository(testDB)

	// Clean up before test
	if err := cleanupTestData(ctx); err != nil {
		t.Fatalf("Failed to cleanup test data: %v", err)
	}

	products, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("Failed to list products: %v", err)
	}

	if products == nil {
		t.Error("Expected empty slice, got nil")
	}
	if len(products) != 0 {
		t.Errorf("Expected 0 products, got %d", len(products))
	}
}

func TestRepository_List_MultipleProducts(t *testing.T) {
	if testDB == nil {
		t.Skip("STOCK_TEST_DATABASE_URL not set, skipping integration test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	repo := NewRepository(testDB)

	// Clean up before test
	if err := cleanupTestData(ctx); err != nil {
		t.Fatalf("Failed to cleanup test data: %v", err)
	}

	// Create test products
	_, err := repo.Create(ctx, "LIST001", "Product 1", 10)
	if err != nil {
		t.Fatalf("Failed to create product 1: %v", err)
	}
	_, err = repo.Create(ctx, "LIST002", "Product 2", 20)
	if err != nil {
		t.Fatalf("Failed to create product 2: %v", err)
	}

	products, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("Failed to list products: %v", err)
	}

	if len(products) != 2 {
		t.Errorf("Expected 2 products, got %d", len(products))
	}

	// Verify ordering by ID
	if products[0].ID >= products[1].ID {
		t.Error("Expected products to be ordered by ID ascending")
	}
}

func TestRepository_GetByID_Success(t *testing.T) {
	if testDB == nil {
		t.Skip("STOCK_TEST_DATABASE_URL not set, skipping integration test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	repo := NewRepository(testDB)

	// Clean up before test
	if err := cleanupTestData(ctx); err != nil {
		t.Fatalf("Failed to cleanup test data: %v", err)
	}

	// Create test product
	created, err := repo.Create(ctx, "GET001", "Get Test Product", 30)
	if err != nil {
		t.Fatalf("Failed to create product: %v", err)
	}

	product, err := repo.GetByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("Failed to get product by ID: %v", err)
	}

	if product.ID != created.ID {
		t.Errorf("Expected ID %d, got %d", created.ID, product.ID)
	}
	if product.Code != "GET001" {
		t.Errorf("Expected code 'GET001', got '%s'", product.Code)
	}
}

func TestRepository_GetByID_NotFound(t *testing.T) {
	if testDB == nil {
		t.Skip("STOCK_TEST_DATABASE_URL not set, skipping integration test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	repo := NewRepository(testDB)

	// Clean up before test
	if err := cleanupTestData(ctx); err != nil {
		t.Fatalf("Failed to cleanup test data: %v", err)
	}

	_, err := repo.GetByID(ctx, 999999)
	if err == nil {
		t.Fatal("Expected error for non-existent product")
	}
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("Expected ErrNotFound, got %v", err)
	}
}

func TestIsCodeUniqueViolation(t *testing.T) {
	if !isCodeUniqueViolation(&pgconn.PgError{Code: "23505", ConstraintName: "products_code_key"}) {
		t.Fatal("expected products_code_key to map to ErrCodeConflict")
	}
	if isCodeUniqueViolation(&pgconn.PgError{Code: "23505", ConstraintName: "another_unique_key"}) {
		t.Fatal("unrelated unique constraint must not map to a product code conflict")
	}
}

func TestRepository_GetByIDs_Success(t *testing.T) {
	if testDB == nil {
		t.Skip("STOCK_TEST_DATABASE_URL not set, skipping integration test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	repo := NewRepository(testDB)

	if err := cleanupTestData(ctx); err != nil {
		t.Fatalf("Failed to cleanup test data: %v", err)
	}

	// Create test products
	p1, err := repo.Create(ctx, "RESOLVE001", "Resolve Product 1", 100)
	if err != nil {
		t.Fatalf("Failed to create product 1: %v", err)
	}
	p2, err := repo.Create(ctx, "RESOLVE002", "Resolve Product 2", 200)
	if err != nil {
		t.Fatalf("Failed to create product 2: %v", err)
	}

	// Get by IDs
	products, err := repo.GetByIDs(ctx, []int64{p1.ID, p2.ID})
	if err != nil {
		t.Fatalf("Failed to get products by IDs: %v", err)
	}

	if len(products) != 2 {
		t.Errorf("Expected 2 products, got %d", len(products))
	}

	// Verify product 1
	prod1, exists := products[p1.ID]
	if !exists {
		t.Fatalf("Expected product with ID %d to exist", p1.ID)
	}
	if prod1.Code != "RESOLVE001" {
		t.Errorf("Expected code 'RESOLVE001', got '%s'", prod1.Code)
	}

	// Verify product 2
	prod2, exists := products[p2.ID]
	if !exists {
		t.Fatalf("Expected product with ID %d to exist", p2.ID)
	}
	if prod2.Code != "RESOLVE002" {
		t.Errorf("Expected code 'RESOLVE002', got '%s'", prod2.Code)
	}
}

func TestRepository_GetByIDs_EmptyList(t *testing.T) {
	if testDB == nil {
		t.Skip("STOCK_TEST_DATABASE_URL not set, skipping integration test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	repo := NewRepository(testDB)

	products, err := repo.GetByIDs(ctx, []int64{})
	if err != nil {
		t.Fatalf("Failed to get products by empty IDs: %v", err)
	}

	if products == nil {
		t.Error("Expected empty map, got nil")
	}
	if len(products) != 0 {
		t.Errorf("Expected 0 products, got %d", len(products))
	}
}

func TestRepository_GetByIDs_WithMissing(t *testing.T) {
	if testDB == nil {
		t.Skip("STOCK_TEST_DATABASE_URL not set, skipping integration test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	repo := NewRepository(testDB)

	if err := cleanupTestData(ctx); err != nil {
		t.Fatalf("Failed to cleanup test data: %v", err)
	}

	// Create one product
	p1, err := repo.Create(ctx, "RESOLVE003", "Resolve Product 3", 300)
	if err != nil {
		t.Fatalf("Failed to create product: %v", err)
	}

	// Request with one existing and one non-existing ID
	products, err := repo.GetByIDs(ctx, []int64{p1.ID, 999999})
	if err != nil {
		t.Fatalf("Failed to get products by IDs: %v", err)
	}

	// Should only return the existing product
	if len(products) != 1 {
		t.Errorf("Expected 1 product, got %d", len(products))
	}

	if _, exists := products[p1.ID]; !exists {
		t.Fatalf("Expected product with ID %d to exist", p1.ID)
	}

	if _, exists := products[999999]; exists {
		t.Error("Expected non-existent product to not be in results")
	}
}
