package invoice

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/EnzoDias2006/korp-api/services/billing/internal/stock"
)

var invoiceTestDB *pgxpool.Pool

// TestMain applies the real Billing migration when BILLING_TEST_DATABASE_URL is
// available. Integration tests otherwise report an explicit skip.
func TestMain(m *testing.M) {
	databaseURL := os.Getenv("BILLING_TEST_DATABASE_URL")
	if databaseURL == "" {
		os.Exit(m.Run())
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "invalid Billing test database configuration: %v\n", err)
		os.Exit(1)
	}
	invoiceTestDB, err = pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		fmt.Fprintf(os.Stderr, "open Billing test database: %v\n", err)
		os.Exit(1)
	}
	if err := invoiceTestDB.Ping(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "ping Billing test database: %v\n", err)
		os.Exit(1)
	}
	if _, err := invoiceTestDB.Exec(ctx, "DROP TABLE IF EXISTS invoice_finalizations CASCADE"); err != nil {
		fmt.Fprintf(os.Stderr, "drop Billing finalization test schema: %v\n", err)
		os.Exit(1)
	}
	if err := applyInvoiceMigration(ctx, "000001_create_invoices.down.sql"); err != nil {
		fmt.Fprintf(os.Stderr, "reset Billing test schema: %v\n", err)
		os.Exit(1)
	}
	if err := applyInvoiceMigration(ctx, "000001_create_invoices.up.sql"); err != nil {
		fmt.Fprintf(os.Stderr, "apply Billing test migration: %v\n", err)
		os.Exit(1)
	}
	if err := applyInvoiceMigration(ctx, "000002_create_invoice_finalizations.up.sql"); err != nil {
		fmt.Fprintf(os.Stderr, "apply Billing finalization test migration: %v\n", err)
		os.Exit(1)
	}
	code := m.Run()

	cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
	if _, err := invoiceTestDB.Exec(cleanupCtx, "DROP TABLE IF EXISTS invoice_finalizations CASCADE"); err != nil {
		fmt.Fprintf(os.Stderr, "drop Billing finalization test schema: %v\n", err)
		code = 1
	}
	if err := applyInvoiceMigration(cleanupCtx, "000001_create_invoices.down.sql"); err != nil {
		fmt.Fprintf(os.Stderr, "roll back Billing test migration: %v\n", err)
		code = 1
	}
	cleanupCancel()
	os.Exit(code)
}

func applyInvoiceMigration(ctx context.Context, name string) error {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		return errors.New("resolve invoice repository test path")
	}
	migrationPath := filepath.Join(filepath.Dir(currentFile), "..", "..", "migrations", name)
	migration, err := os.ReadFile(migrationPath)
	if err != nil {
		return fmt.Errorf("read %s: %w", name, err)
	}
	_, err = invoiceTestDB.Exec(ctx, string(migration))
	return err
}

func requireInvoiceTestDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	if invoiceTestDB == nil {
		t.Skip("BILLING_TEST_DATABASE_URL not set; skipping PostgreSQL integration test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	if _, err := invoiceTestDB.Exec(ctx, "CREATE TABLE IF NOT EXISTS invoice_finalizations (invoice_id BIGINT PRIMARY KEY REFERENCES invoices (id) ON DELETE CASCADE, operation_id UUID NOT NULL UNIQUE, started_at TIMESTAMPTZ NOT NULL DEFAULT now(), completed_at TIMESTAMPTZ)"); err != nil {
		t.Fatalf("ensure finalization table: %v", err)
	}
	if _, err := invoiceTestDB.Exec(ctx, "TRUNCATE TABLE invoice_finalizations, invoices RESTART IDENTITY CASCADE"); err != nil {
		t.Fatalf("reset invoice test data: %v", err)
	}
	return invoiceTestDB
}

func testCreateItems(seed int64) []CreateItem {
	return []CreateItem{
		{ProductID: seed + 10, ProductCode: fmt.Sprintf("P-%d-A", seed), ProductDescription: "First", Quantity: 2},
		{ProductID: seed + 20, ProductCode: fmt.Sprintf("P-%d-B", seed), ProductDescription: "Second", Quantity: 1},
	}
}

func TestMigrationUsesPostgreSQLIdentityForInvoiceNumber(t *testing.T) {
	db := requireInvoiceTestDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var isIdentity, identityGeneration string
	if err := db.QueryRow(ctx, `
		SELECT is_identity, identity_generation
		FROM information_schema.columns
		WHERE table_schema = current_schema()
		  AND table_name = 'invoices'
		  AND column_name = 'number'
	`).Scan(&isIdentity, &identityGeneration); err != nil {
		t.Fatalf("inspect invoice number column: %v", err)
	}
	if isIdentity != "YES" || identityGeneration != "ALWAYS" {
		t.Fatalf("invoice number identity = %s/%s, want YES/ALWAYS", isIdentity, identityGeneration)
	}

	var sequenceName *string
	if err := db.QueryRow(ctx, "SELECT pg_get_serial_sequence('invoices', 'number')").Scan(&sequenceName); err != nil {
		t.Fatalf("resolve invoice number sequence: %v", err)
	}
	if sequenceName == nil || *sequenceName == "" {
		t.Fatal("invoice number is not backed by a PostgreSQL sequence")
	}
}

func TestRepositoryCreateListAndGet(t *testing.T) {
	db := requireInvoiceTestDB(t)
	repository := NewRepository(db)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	first, err := repository.Create(ctx, testCreateItems(1))
	if err != nil {
		t.Fatalf("create first invoice: %v", err)
	}
	second, err := repository.Create(ctx, testCreateItems(100))
	if err != nil {
		t.Fatalf("create second invoice: %v", err)
	}
	if first.Number != 1 || second.Number != 2 {
		t.Fatalf("invoice numbers = %d, %d; want 1, 2", first.Number, second.Number)
	}
	if first.Status != StatusOpen || first.ClosedAt != nil {
		t.Fatalf("first invoice state = %s/%v, want OPEN/nil", first.Status, first.ClosedAt)
	}

	listed, err := repository.List(ctx)
	if err != nil {
		t.Fatalf("list invoices: %v", err)
	}
	if len(listed) != 2 || listed[0].ID != first.ID || listed[1].ID != second.ID {
		t.Fatalf("List() order = %#v, want first then second", listed)
	}
	if !reflect.DeepEqual(listed[0].Items, []Item{
		{ProductID: 11, ProductCode: "P-1-A", ProductDescription: "First", Quantity: 2},
		{ProductID: 21, ProductCode: "P-1-B", ProductDescription: "Second", Quantity: 1},
	}) {
		t.Fatalf("List() first items = %#v", listed[0].Items)
	}

	found, err := repository.GetByID(ctx, second.ID)
	if err != nil {
		t.Fatalf("get second invoice: %v", err)
	}
	if !reflect.DeepEqual(found, second) {
		t.Fatalf("GetByID() = %#v, want %#v", found, second)
	}
}

func TestRepositoryListEmptyReturnsNonNilSlice(t *testing.T) {
	db := requireInvoiceTestDB(t)
	repository := NewRepository(db)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	listed, err := repository.List(ctx)
	if err != nil {
		t.Fatalf("List(): %v", err)
	}
	if listed == nil || len(listed) != 0 {
		t.Fatalf("List() = %#v, want non-nil empty slice", listed)
	}
}

func TestRepositoryGetByIDNotFound(t *testing.T) {
	db := requireInvoiceTestDB(t)
	repository := NewRepository(db)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := repository.GetByID(ctx, 999)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetByID() error = %v, want ErrNotFound", err)
	}
}

func TestRepositoryCreateRollsBackInvoiceAndItems(t *testing.T) {
	db := requireInvoiceTestDB(t)
	repository := NewRepository(db)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	duplicateItems := []CreateItem{
		{ProductID: 5, ProductCode: "P5", ProductDescription: "First", Quantity: 1},
		{ProductID: 5, ProductCode: "P5", ProductDescription: "Duplicate", Quantity: 1},
	}
	if _, err := repository.Create(ctx, duplicateItems); err == nil {
		t.Fatal("Create() expected invoice item constraint failure")
	}

	var invoiceCount, itemCount int
	if err := db.QueryRow(ctx, "SELECT count(*) FROM invoices").Scan(&invoiceCount); err != nil {
		t.Fatalf("count invoices: %v", err)
	}
	if err := db.QueryRow(ctx, "SELECT count(*) FROM invoice_items").Scan(&itemCount); err != nil {
		t.Fatalf("count invoice items: %v", err)
	}
	if invoiceCount != 0 || itemCount != 0 {
		t.Fatalf("rollback left %d invoices and %d items", invoiceCount, itemCount)
	}

	created, err := repository.Create(ctx, testCreateItems(1))
	if err != nil {
		t.Fatalf("create after rollback: %v", err)
	}
	if created.Number != 2 {
		t.Fatalf("number after rollback = %d, want 2 to prove PostgreSQL sequence gaps are not transactional", created.Number)
	}
}

func TestRepositoryConcurrentCreatesUseUniqueNumbers(t *testing.T) {
	db := requireInvoiceTestDB(t)
	repository := NewRepository(db)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	const count = 8
	numbers := make(chan int64, count)
	errorsFound := make(chan error, count)
	var waitGroup sync.WaitGroup
	for index := int64(0); index < count; index++ {
		waitGroup.Add(1)
		go func(seed int64) {
			defer waitGroup.Done()
			created, err := repository.Create(ctx, testCreateItems(seed*100))
			if err != nil {
				errorsFound <- err
				return
			}
			numbers <- created.Number
		}(index + 1)
	}
	waitGroup.Wait()
	close(numbers)
	close(errorsFound)
	for err := range errorsFound {
		t.Errorf("concurrent Create(): %v", err)
	}
	if t.Failed() {
		return
	}

	got := make([]int, 0, count)
	for number := range numbers {
		got = append(got, int(number))
	}
	sort.Ints(got)
	for index, number := range got {
		if number != index+1 {
			t.Fatalf("concurrent numbers = %v, want contiguous 1..%d", got, count)
		}
	}
}

func TestRepositoryCompleteFinalizationAllowsExactlyOneWinner(t *testing.T) {
	db := requireInvoiceTestDB(t)
	repository := NewRepository(db)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	created, err := repository.Create(ctx, testCreateItems(31))
	if err != nil {
		t.Fatalf("Create(): %v", err)
	}
	operation, err := repository.StartFinalization(ctx, created.ID)
	if err != nil {
		t.Fatalf("StartFinalization(): %v", err)
	}

	const callers = 8
	winners := make(chan bool, callers)
	errorsFound := make(chan error, callers)
	var waitGroup sync.WaitGroup
	for index := 0; index < callers; index++ {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			_, updated, err := repository.CompleteFinalization(ctx, created.ID, operation.OperationID)
			if err != nil {
				errorsFound <- err
				return
			}
			winners <- updated
		}()
	}
	waitGroup.Wait()
	close(winners)
	close(errorsFound)
	for err := range errorsFound {
		t.Errorf("CompleteFinalization(): %v", err)
	}
	if t.Failed() {
		return
	}

	winningUpdates := 0
	for updated := range winners {
		if updated {
			winningUpdates++
		}
	}
	if winningUpdates != 1 {
		t.Fatalf("winning updates = %d, want exactly one", winningUpdates)
	}

	var closedAt, completedAt *time.Time
	if err := db.QueryRow(ctx, `
		SELECT invoices.closed_at, invoice_finalizations.completed_at
		FROM invoices
		JOIN invoice_finalizations ON invoice_finalizations.invoice_id = invoices.id
		WHERE invoices.id = $1
	`, created.ID).Scan(&closedAt, &completedAt); err != nil {
		t.Fatalf("query completed invoice: %v", err)
	}
	if closedAt == nil || completedAt == nil {
		t.Fatalf("completion timestamps = closed %v/completed %v; want both set", closedAt, completedAt)
	}
}

func TestRepositoryListConsumptionsUsesPersistedCommand(t *testing.T) {
	db := requireInvoiceTestDB(t)
	repository := NewRepository(db)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	created, err := repository.Create(ctx, testCreateItems(41))
	if err != nil {
		t.Fatalf("Create(): %v", err)
	}

	items, err := repository.ListConsumptions(ctx, created.ID)
	if err != nil {
		t.Fatalf("ListConsumptions(): %v", err)
	}
	want := []stock.ConsumeItem{
		{ProductID: 51, Quantity: 2},
		{ProductID: 61, Quantity: 1},
	}
	if len(items) != len(want) || items[0] != want[0] || items[1] != want[1] {
		t.Fatalf("ListConsumptions() = %#v, want %#v", items, want)
	}
}

func TestRepositoryRepeatedCompletionReportsNotUpdated(t *testing.T) {
	db := requireInvoiceTestDB(t)
	repository := NewRepository(db)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	created, err := repository.Create(ctx, testCreateItems(71))
	if err != nil {
		t.Fatalf("Create(): %v", err)
	}
	operation, err := repository.StartFinalization(ctx, created.ID)
	if err != nil {
		t.Fatalf("StartFinalization(): %v", err)
	}
	if _, updated, err := repository.CompleteFinalization(ctx, created.ID, operation.OperationID); err != nil || !updated {
		t.Fatalf("first CompleteFinalization() updated = %t, error = %v", updated, err)
	}
	current, updated, err := repository.CompleteFinalization(ctx, created.ID, operation.OperationID)
	if err != nil || updated || current.Status != StatusClosed {
		t.Fatalf("second CompleteFinalization() = %#v, %t, %v; want CLOSED, false, nil", current, updated, err)
	}
}

func TestRepositoryFinalizationClaimAndCompletion(t *testing.T) {
	db := requireInvoiceTestDB(t)
	repository := NewRepository(db)
	ctx := context.Background()

	created, err := repository.Create(ctx, testCreateItems(81))
	if err != nil {
		t.Fatalf("Create(): %v", err)
	}
	first, err := repository.StartFinalization(ctx, created.ID)
	if err != nil {
		t.Fatalf("StartFinalization(): %v", err)
	}
	second, err := repository.StartFinalization(ctx, created.ID)
	if err != nil {
		t.Fatalf("retry StartFinalization(): %v", err)
	}
	if first.OperationID == ([16]byte{}) || first.OperationID != second.OperationID {
		t.Fatalf("operation IDs = %v/%v; want one stable non-zero identity", first.OperationID, second.OperationID)
	}
	completed, updated, err := repository.CompleteFinalization(ctx, created.ID, first.OperationID)
	if err != nil || !updated {
		t.Fatalf("CompleteFinalization() = %#v, %t, %v; want updated", completed, updated, err)
	}
	if completed.Status != StatusClosed || completed.ClosedAt == nil || !reflect.DeepEqual(completed.Items, created.Items) {
		t.Fatalf("CompleteFinalization() response = %#v, want closed invoice with items %#v", completed, created.Items)
	}
	var completedAt *time.Time
	if err := db.QueryRow(ctx,
		`SELECT completed_at FROM invoice_finalizations WHERE invoice_id = $1`, created.ID,
	).Scan(&completedAt); err != nil {
		t.Fatalf("query finalization: %v", err)
	}
	if completedAt == nil {
		t.Fatal("completed_at is nil after successful completion")
	}
}

func TestRepositoryMissingCompletionOperationRollsBackInvoiceClose(t *testing.T) {
	db := requireInvoiceTestDB(t)
	repository := NewRepository(db)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	created, err := repository.Create(ctx, testCreateItems(91))
	if err != nil {
		t.Fatalf("Create(): %v", err)
	}
	if _, updated, err := repository.CompleteFinalization(ctx, created.ID, [16]byte{99}); err == nil || updated {
		t.Fatalf("CompleteFinalization() updated = %t, error = %v; want missing-operation error", updated, err)
	}

	var status Status
	var closedAt *time.Time
	if err := db.QueryRow(ctx, `SELECT status, closed_at FROM invoices WHERE id = $1`, created.ID).Scan(&status, &closedAt); err != nil {
		t.Fatalf("query invoice after failed completion: %v", err)
	}
	if status != StatusOpen || closedAt != nil {
		t.Fatalf("invoice after failed completion = %s/%v, want OPEN/nil", status, closedAt)
	}

	operation, err := repository.StartFinalization(ctx, created.ID)
	if err != nil {
		t.Fatalf("StartFinalization() after rollback: %v", err)
	}
	completed, updated, err := repository.CompleteFinalization(ctx, created.ID, operation.OperationID)
	if err != nil || !updated || completed.Status != StatusClosed {
		t.Fatalf("retry CompleteFinalization() = %#v, %t, %v; want recoverable success", completed, updated, err)
	}
}
