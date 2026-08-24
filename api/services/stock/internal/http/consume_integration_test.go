package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/EnzoDias2006/korp-api/services/stock/internal/product"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var consumeTestDB *pgxpool.Pool
var testDatabaseName string
var testAdminConn *pgx.Conn

func TestMain(m *testing.M) {
	databaseURL := os.Getenv("STOCK_TEST_DATABASE_URL")
	if databaseURL == "" {
		os.Exit(m.Run())
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		fmt.Fprintln(os.Stderr, "invalid stock test database configuration:", err)
		os.Exit(1)
	}
	adminConfig := config.ConnConfig.Copy()
	adminConfig.Database = "postgres"
	testAdminConn, err = pgx.ConnectConfig(ctx, adminConfig)
	if err != nil {
		fmt.Fprintln(os.Stderr, "connect stock test administrator:", err)
		os.Exit(1)
	}

	testDatabaseName = fmt.Sprintf("stock_consume_test_%d", time.Now().UnixNano())
	if _, err = testAdminConn.Exec(ctx, fmt.Sprintf(`DROP DATABASE IF EXISTS %q`, testDatabaseName)); err != nil {
		fmt.Fprintln(os.Stderr, "drop isolated stock test database:", err)
		os.Exit(1)
	}
	if _, err = testAdminConn.Exec(ctx, fmt.Sprintf(`CREATE DATABASE %q`, testDatabaseName)); err != nil {
		fmt.Fprintln(os.Stderr, "create isolated stock test database:", err)
		os.Exit(1)
	}

	config.ConnConfig.Database = testDatabaseName
	consumeTestDB, err = pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		fmt.Fprintln(os.Stderr, "connect isolated stock test database:", err)
		os.Exit(1)
	}
	if err = consumeTestDB.Ping(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "ping isolated stock test database:", err)
		os.Exit(1)
	}
	if err = applyConsumeMigration(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "apply stock consume test migration:", err)
		os.Exit(1)
	}

	code := m.Run()

	consumeTestDB.Close()
	cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
	if _, err = testAdminConn.Exec(cleanupCtx, fmt.Sprintf(`DROP DATABASE IF EXISTS %q WITH (FORCE)`, testDatabaseName)); err != nil {
		fmt.Fprintln(os.Stderr, "remove isolated stock test database:", err)
		code = 1
	}
	cleanupCancel()
	testAdminConn.Close(context.Background())
	os.Exit(code)
}

func applyConsumeMigration(ctx context.Context) error {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		return errors.New("resolve consume integration test path")
	}

	conn, err := consumeTestDB.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("acquire migration connection: %w", err)
	}
	defer conn.Release()

	const advisoryLock = `SELECT pg_advisory_lock(727272001)`
	if _, err = conn.Exec(ctx, advisoryLock); err != nil {
		return fmt.Errorf("lock test migrations: %w", err)
	}
	defer conn.Exec(ctx, `SELECT pg_advisory_unlock(727272001)`)

	var productsExist bool
	if err = conn.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_schema = current_schema() AND table_name = 'products')`).Scan(&productsExist); err != nil {
		return fmt.Errorf("check products table: %w", err)
	}
	if !productsExist {
		productsMigration := filepath.Join(filepath.Dir(currentFile), "..", "..", "migrations", "000001_create_products.up.sql")
		rawMigration, readErr := os.ReadFile(productsMigration)
		if readErr != nil {
			return fmt.Errorf("read product migration: %w", readErr)
		}
		if _, err = conn.Exec(ctx, string(rawMigration)); err != nil {
			return fmt.Errorf("apply product migration: %w", err)
		}
	}

	var operationsExist bool
	if err = conn.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_schema = current_schema() AND table_name = 'consumption_operations')`).Scan(&operationsExist); err != nil {
		return fmt.Errorf("check consumption operations table: %w", err)
	}
	if !operationsExist {
		operationsMigration := filepath.Join(filepath.Dir(currentFile), "..", "..", "migrations", "000002_create_consumption_operations.up.sql")
		rawMigration, readErr := os.ReadFile(operationsMigration)
		if readErr != nil {
			return fmt.Errorf("read consumption operations migration: %w", readErr)
		}
		if _, err = conn.Exec(ctx, string(rawMigration)); err != nil {
			return fmt.Errorf("apply consumption operations migration: %w", err)
		}
	}

	var operationResultsExist bool
	if err = conn.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_schema = current_schema() AND table_name = 'consumption_operation_results')`).Scan(&operationResultsExist); err != nil {
		return fmt.Errorf("check consumption operation results table: %w", err)
	}
	if !operationResultsExist {
		operationResultsMigration := filepath.Join(filepath.Dir(currentFile), "..", "..", "migrations", "000003_create_consumption_operation_results.up.sql")
		rawMigration, readErr := os.ReadFile(operationResultsMigration)
		if readErr != nil {
			return fmt.Errorf("read consumption operation results migration: %w", readErr)
		}
		if _, err = conn.Exec(ctx, string(rawMigration)); err != nil {
			return fmt.Errorf("apply consumption operation results migration: %w", err)
		}
	}
	return nil
}

func TestConsumeStock_ConcurrentBalanceOne(t *testing.T) {
	if consumeTestDB == nil {
		t.Skip("STOCK_TEST_DATABASE_URL not set, skipping integration test")
	}

	if _, err := consumeTestDB.Exec(context.Background(), "TRUNCATE TABLE consumption_operations, products RESTART IDENTITY CASCADE"); err != nil {
		t.Fatalf("truncate products: %v", err)
	}
	repository := product.NewRepository(consumeTestDB)
	created, err := repository.Create(context.Background(), "HTTP-RACE", "HTTP Race", 1)
	if err != nil {
		t.Fatalf("create product: %v", err)
	}
	router := NewRouter(nil, slog.New(slog.DiscardHandler), nil, product.NewService(repository))

	const workers = 2
	results := make(chan int, workers)
	start := make(chan struct{})
	var ready sync.WaitGroup
	ready.Add(workers)
	for worker := 0; worker < workers; worker++ {
		go func() {
			ready.Done()
			<-start
			request := httptest.NewRequest(http.MethodPost, "/internal/v1/stock/consume", consumeRaceBody(t, created.ID))
			request.Header.Set("Content-Type", "application/json")
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, request)
			results <- recorder.Code
		}()
	}
	ready.Wait()
	close(start)

	var successes int
	for worker := 0; worker < workers; worker++ {
		switch status := <-results; status {
		case http.StatusOK:
			successes++
		default:
			t.Fatalf("unexpected HTTP status %d", status)
		}
	}
	if successes != 2 {
		t.Fatalf("successes = %d, want one execution and one replay", successes)
	}

	var balance int
	if err := consumeTestDB.QueryRow(context.Background(), "SELECT balance FROM products WHERE id = $1", created.ID).Scan(&balance); err != nil {
		t.Fatalf("query final balance: %v", err)
	}
	if balance != 0 {
		t.Fatalf("final balance = %d, want 0", balance)
	}
}

func TestConsumeStock_ReplayReturnsOriginalResponseAfterLaterConsumption(t *testing.T) {
	if consumeTestDB == nil {
		t.Skip("STOCK_TEST_DATABASE_URL not set, skipping integration test")
	}

	ctx := context.Background()
	if _, err := consumeTestDB.Exec(ctx, "TRUNCATE TABLE consumption_operations, products RESTART IDENTITY CASCADE"); err != nil {
		t.Fatalf("truncate products: %v", err)
	}
	repository := product.NewRepository(consumeTestDB)
	created, err := repository.Create(ctx, "HTTP-STABLE-REPLAY", "HTTP Stable Replay", 5)
	if err != nil {
		t.Fatalf("create product: %v", err)
	}
	router := NewRouter(nil, slog.New(slog.DiscardHandler), nil, product.NewService(repository))

	firstRequest := stockConsumeInput{
		InvoiceID:   pointerTo(int64(201)),
		OperationID: pointerTo(product.OperationID{2, 1}),
		Items: []stockConsumeItem{{
			ProductID: pointerTo(created.ID),
			Quantity:  pointerTo(2),
		}},
	}
	requireConsumeBalance(t, router, firstRequest, created.ID, 3)

	laterRequest := stockConsumeInput{
		InvoiceID:   pointerTo(int64(202)),
		OperationID: pointerTo(product.OperationID{2, 2}),
		Items: []stockConsumeItem{{
			ProductID: pointerTo(created.ID),
			Quantity:  pointerTo(1),
		}},
	}
	requireConsumeBalance(t, router, laterRequest, created.ID, 2)
	requireConsumeBalance(t, router, firstRequest, created.ID, 3)

	var currentBalance int
	if err := consumeTestDB.QueryRow(ctx, "SELECT balance FROM products WHERE id = $1", created.ID).Scan(&currentBalance); err != nil {
		t.Fatalf("query final balance: %v", err)
	}
	if currentBalance != 2 {
		t.Fatalf("final balance = %d, want unchanged 2", currentBalance)
	}
}

func TestConsumeStock_LegacyOperationWithoutResultReturnsSafeError(t *testing.T) {
	if consumeTestDB == nil {
		t.Skip("STOCK_TEST_DATABASE_URL not set, skipping integration test")
	}

	ctx := context.Background()
	if _, err := consumeTestDB.Exec(ctx, "TRUNCATE TABLE consumption_operations, products RESTART IDENTITY CASCADE"); err != nil {
		t.Fatalf("truncate products: %v", err)
	}
	repository := product.NewRepository(consumeTestDB)
	created, err := repository.Create(ctx, "HTTP-LEGACY-RESULT", "HTTP Legacy Result", 5)
	if err != nil {
		t.Fatalf("create product: %v", err)
	}
	request := stockConsumeInput{
		InvoiceID:   pointerTo(int64(203)),
		OperationID: pointerTo(product.OperationID{2, 3}),
		Items: []stockConsumeItem{{
			ProductID: pointerTo(created.ID),
			Quantity:  pointerTo(1),
		}},
	}
	fingerprint := product.ConsumptionFingerprint(
		*request.InvoiceID,
		[]product.StockItem{{ProductID: created.ID, Quantity: 1}},
	)
	if _, err := consumeTestDB.Exec(ctx, `
		INSERT INTO consumption_operations (operation_id, invoice_id, fingerprint)
		VALUES ($1, $2, $3)
	`, *request.OperationID, *request.InvoiceID, fingerprint[:]); err != nil {
		t.Fatalf("insert legacy operation: %v", err)
	}

	router := NewRouter(nil, slog.New(slog.DiscardHandler), nil, product.NewService(repository))
	recorder, response := postConsume(t, router, "application/json", request)
	if recorder.Code != http.StatusInternalServerError || requireError(t, response, "INTERNAL_ERROR") == "" {
		t.Fatalf("legacy replay status = %d, response = %#v", recorder.Code, response)
	}
	if bytes.Contains(recorder.Body.Bytes(), []byte("incomplete consumption")) {
		t.Fatalf("response leaked internal replay details: %s", recorder.Body.String())
	}

	var currentBalance int
	if err := consumeTestDB.QueryRow(ctx, "SELECT balance FROM products WHERE id = $1", created.ID).Scan(&currentBalance); err != nil {
		t.Fatalf("query final balance: %v", err)
	}
	if currentBalance != 5 {
		t.Fatalf("final balance = %d, want unchanged 5", currentBalance)
	}
}

func requireConsumeBalance(
	t *testing.T,
	router http.Handler,
	request stockConsumeInput,
	productID int64,
	wantBalance int,
) {
	t.Helper()

	recorder, response := postConsume(t, router, "application/json", request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("consume status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	rawResponse, err := json.Marshal(response)
	if err != nil {
		t.Fatalf("marshal decoded consume response: %v", err)
	}
	var result stockConsumeResponse
	if err := json.Unmarshal(rawResponse, &result); err != nil {
		t.Fatalf("decode typed consume response: %v", err)
	}
	if len(result.Balances) != 1 || result.Balances[0] != (stockBalanceResponse{ProductID: productID, Balance: wantBalance}) {
		t.Fatalf("balances = %#v, want product %d balance %d", result.Balances, productID, wantBalance)
	}
}

func consumeRaceBody(t *testing.T, productID int64) *bytes.Reader {
	t.Helper()

	invoiceID := int64(77)
	operationID := product.OperationID{7}
	quantity := 1
	body, err := json.Marshal(stockConsumeInput{
		InvoiceID:   &invoiceID,
		OperationID: &operationID,
		Items: []stockConsumeItem{{
			ProductID: &productID,
			Quantity:  &quantity,
		}},
	})
	if err != nil {
		t.Fatalf("marshal consume body: %v", err)
	}
	return bytes.NewReader(body)
}
