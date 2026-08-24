package product

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func createConsumeProduct(t *testing.T, code string, balance int) int64 {
	t.Helper()

	product, err := testDBProductRepository().Create(context.Background(), code, code, balance)
	if err != nil {
		t.Fatalf("create product %s: %v", code, err)
	}
	return product.ID
}

func requireConsumeError(t *testing.T, err error, target error) {
	t.Helper()

	if !errors.Is(err, target) {
		t.Fatalf("error = %v, want %v", err, target)
	}
}

func testDBProductRepository() *Repository {
	return NewRepository(testDB)
}

func productOperationID(seed int) OperationID {
	operationID := OperationID{}
	for index := range operationID {
		operationID[index] = byte(seed + index)
	}
	return operationID
}

func testConsumeInput(invoiceID int64, items []StockItem, operationID ...OperationID) ConsumeInput {
	input := ConsumeInput{InvoiceID: invoiceID, Items: items}
	if len(operationID) == 0 {
		input.OperationID = productOperationID(int(invoiceID))
		return input
	}
	input.OperationID = operationID[0]
	return input
}

func TestRepository_Consume_SuccessRollbackAndMissing(t *testing.T) {
	if testDB == nil {
		t.Skip("STOCK_TEST_DATABASE_URL not set, skipping integration test")
	}

	ctx := context.Background()
	repository := testDBProductRepository()
	first := createConsumeProduct(t, "CONSUME-ONE", 5)
	second := createConsumeProduct(t, "CONSUME-TWO", 2)

	balances, _, err := repository.Consume(ctx, testConsumeInput(1, []StockItem{
		{ProductID: second, Quantity: 2},
		{ProductID: first, Quantity: 3},
	}))
	if err != nil {
		t.Fatalf("consume: %v", err)
	}
	if balances[first] != 2 || balances[second] != 0 {
		t.Fatalf("balances = %#v", balances)
	}

	if err := cleanupTestData(ctx); err != nil {
		t.Fatalf("cleanup before failure: %v", err)
	}
	first = createConsumeProduct(t, "ROLLBACK-ONE", 10)
	second = createConsumeProduct(t, "ROLLBACK-TWO", 4)
	_, _, err = repository.Consume(ctx, testConsumeInput(2, []StockItem{
		{ProductID: first, Quantity: 5},
		{ProductID: second, Quantity: 5},
	}))
	if !errors.Is(err, ErrInsufficientStock) {
		t.Fatalf("error = %v, want %v", err, ErrInsufficientStock)
	}

	var balance int
	if err := testDB.QueryRow(ctx, "SELECT balance FROM products WHERE id = $1", first).Scan(&balance); err != nil {
		t.Fatalf("query first balance: %v", err)
	}
	if balance != 10 {
		t.Fatalf("first balance = %d, want unchanged 10", balance)
	}

	if err := cleanupTestData(ctx); err != nil {
		t.Fatalf("cleanup before missing: %v", err)
	}
	existing := createConsumeProduct(t, "MISSING-ONE", 5)
	_, _, err = repository.Consume(ctx, testConsumeInput(3, []StockItem{
		{ProductID: existing, Quantity: 1}, {ProductID: existing + 1, Quantity: 1},
	}))
	requireConsumeError(t, err, ErrNotFound)
}

func TestRepository_Consume_ReplayReturnsStoredBalance(t *testing.T) {
	if testDB == nil {
		t.Skip("STOCK_TEST_DATABASE_URL not set, skipping integration test")
	}

	ctx := context.Background()
	if err := cleanupTestData(ctx); err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	repository := testDBProductRepository()
	productID := createConsumeProduct(t, "STABLE-REPLAY", 5)
	firstInput := testConsumeInput(10, []StockItem{{ProductID: productID, Quantity: 2}}, productOperationID(10))

	firstBalances, replayed, err := repository.Consume(ctx, firstInput)
	if err != nil {
		t.Fatalf("first consume: %v", err)
	}
	if replayed {
		t.Fatal("first consume unexpectedly reported replay")
	}
	if firstBalances[productID] != 3 {
		t.Fatalf("first balance = %d, want 3", firstBalances[productID])
	}

	secondBalances, replayed, err := repository.Consume(ctx, testConsumeInput(
		11,
		[]StockItem{{ProductID: productID, Quantity: 1}},
		productOperationID(11),
	))
	if err != nil {
		t.Fatalf("later consume: %v", err)
	}
	if replayed {
		t.Fatal("later consume unexpectedly reported replay")
	}
	if secondBalances[productID] != 2 {
		t.Fatalf("later balance = %d, want 2", secondBalances[productID])
	}

	replayBalances, replayed, err := repository.Consume(ctx, firstInput)
	if err != nil {
		t.Fatalf("replay first consume: %v", err)
	}
	if !replayed {
		t.Fatal("replayed consume was not reported as replay")
	}
	if replayBalances[productID] != 3 {
		t.Fatalf("replayed balance = %d, want stored result 3", replayBalances[productID])
	}

	var currentBalance int
	if err := testDB.QueryRow(ctx, "SELECT balance FROM products WHERE id = $1", productID).Scan(&currentBalance); err != nil {
		t.Fatalf("query current balance: %v", err)
	}
	if currentBalance != 2 {
		t.Fatalf("current balance = %d, want unchanged 2", currentBalance)
	}
}

func TestRepository_Consume_RejectsConflictingPayload(t *testing.T) {
	if testDB == nil {
		t.Skip("STOCK_TEST_DATABASE_URL not set, skipping integration test")
	}

	ctx := context.Background()
	if err := cleanupTestData(ctx); err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	repository := testDBProductRepository()
	productID := createConsumeProduct(t, "PAYLOAD-CONFLICT", 5)
	operationID := productOperationID(20)

	_, _, err := repository.Consume(ctx, testConsumeInput(
		20,
		[]StockItem{{ProductID: productID, Quantity: 1}},
		operationID,
	))
	if err != nil {
		t.Fatalf("first consume: %v", err)
	}

	_, _, err = repository.Consume(ctx, testConsumeInput(
		20,
		[]StockItem{{ProductID: productID, Quantity: 2}},
		operationID,
	))
	requireConsumeError(t, err, ErrIdempotencyConflict)

	var currentBalance int
	if err := testDB.QueryRow(ctx, "SELECT balance FROM products WHERE id = $1", productID).Scan(&currentBalance); err != nil {
		t.Fatalf("query current balance: %v", err)
	}
	if currentBalance != 4 {
		t.Fatalf("current balance = %d, want unchanged 4", currentBalance)
	}
}

func TestRepository_Consume_RejectsLegacyOperationWithoutResult(t *testing.T) {
	if testDB == nil {
		t.Skip("STOCK_TEST_DATABASE_URL not set, skipping integration test")
	}

	ctx := context.Background()
	if err := cleanupTestData(ctx); err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	repository := testDBProductRepository()
	productID := createConsumeProduct(t, "LEGACY-RESULT", 5)
	input := testConsumeInput(30, []StockItem{{ProductID: productID, Quantity: 1}}, productOperationID(30))
	fingerprint := ConsumptionFingerprint(input.InvoiceID, input.Items)
	if _, err := testDB.Exec(ctx, `
		INSERT INTO consumption_operations (operation_id, invoice_id, fingerprint)
		VALUES ($1, $2, $3)
	`, input.OperationID, input.InvoiceID, fingerprint[:]); err != nil {
		t.Fatalf("insert legacy operation: %v", err)
	}

	_, replayed, err := repository.Consume(ctx, input)
	if err == nil {
		t.Fatal("legacy operation without a durable result unexpectedly replayed")
	}
	if replayed {
		t.Fatal("incomplete legacy operation reported as a successful replay")
	}
	if errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("incomplete result mapped to payload conflict: %v", err)
	}

	var currentBalance int
	if err := testDB.QueryRow(ctx, "SELECT balance FROM products WHERE id = $1", productID).Scan(&currentBalance); err != nil {
		t.Fatalf("query current balance: %v", err)
	}
	if currentBalance != 5 {
		t.Fatalf("current balance = %d, want unchanged 5", currentBalance)
	}
}

func TestRepository_Consume_ConcurrentDistinctOperationsBalanceOne(t *testing.T) {
	if testDB == nil {
		t.Skip("STOCK_TEST_DATABASE_URL not set, skipping integration test")
	}

	ctx := context.Background()
	if err := cleanupTestData(ctx); err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	repository := testDBProductRepository()
	productID := createConsumeProduct(t, "RACE-ONE", 1)

	const workers = 2
	results := make(chan error, workers)
	start := make(chan struct{})
	var ready sync.WaitGroup
	ready.Add(workers)
	for worker := 0; worker < workers; worker++ {
		go func(worker int) {
			ready.Done()
			<-start
			consumeContext, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			invoiceID := int64(100 + worker)
			_, _, err := repository.Consume(consumeContext, testConsumeInput(
				invoiceID,
				[]StockItem{{ProductID: productID, Quantity: 1}},
				productOperationID(100+worker),
			))
			results <- err
		}(worker)
	}
	ready.Wait()
	close(start)

	var successes int
	var insufficient int
	for worker := 0; worker < workers; worker++ {
		err := <-results
		switch {
		case err == nil:
			successes++
		case errors.Is(err, ErrInsufficientStock):
			insufficient++
		default:
			t.Fatalf("unexpected consume error: %v", err)
		}
	}
	if successes != 1 || insufficient != 1 {
		t.Fatalf("successes = %d, insufficient = %d; want 1 and 1", successes, insufficient)
	}

	var balance int
	if err := testDB.QueryRow(ctx, "SELECT balance FROM products WHERE id = $1", productID).Scan(&balance); err != nil {
		t.Fatalf("query final balance: %v", err)
	}
	if balance != 0 {
		t.Fatalf("final balance = %d, want 0", balance)
	}
}
