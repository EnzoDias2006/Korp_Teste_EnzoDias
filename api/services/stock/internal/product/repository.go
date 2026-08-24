package product

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository implements the Store interface using pgxpool.
type Repository struct {
	pool *pgxpool.Pool
}

// NewRepository creates a new product Repository with the given connection pool.
func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

// Consume opens one transaction, locks requested products in deterministic ID
// order, validates every balance, and updates all rows or rolls everything back.
func (r *Repository) Consume(ctx context.Context, input ConsumeInput) (map[int64]int, bool, error) {
	items := input.Items
	fingerprint := ConsumptionFingerprint(input.InvoiceID, items)

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, false, fmt.Errorf("failed to begin stock consumption: %w", err)
	}
	defer tx.Rollback(ctx)

	const claimQuery = `
		INSERT INTO consumption_operations (operation_id, invoice_id, fingerprint)
		VALUES ($1, $2, $3)
		ON CONFLICT (operation_id) DO NOTHING
		RETURNING created_at
	`
	var operationCreatedAt time.Time
	err = tx.QueryRow(
		ctx,
		claimQuery,
		input.OperationID,
		input.InvoiceID,
		fingerprint[:],
	).Scan(&operationCreatedAt)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		var existingFingerprint []byte
		if err := tx.QueryRow(ctx,
			`SELECT fingerprint FROM consumption_operations WHERE operation_id = $1`,
			input.OperationID,
		).Scan(&existingFingerprint); err != nil {
			return nil, false, fmt.Errorf("failed to read consumption idempotency record: %w", err)
		}
		if !bytes.Equal(existingFingerprint, fingerprint[:]) {
			return nil, false, ErrIdempotencyConflict
		}

		const replayBalanceQuery = `
			SELECT product_id, balance
			FROM consumption_operation_results
			WHERE operation_id = $1
			ORDER BY product_id ASC
		`
		replayRows, queryErr := tx.Query(ctx, replayBalanceQuery, input.OperationID)
		if queryErr != nil {
			return nil, false, fmt.Errorf("failed to read replay balances: %w", queryErr)
		}
		balances := make(map[int64]int, len(items))
		for replayRows.Next() {
			var productID int64
			var balance int
			if scanErr := replayRows.Scan(&productID, &balance); scanErr != nil {
				replayRows.Close()
				return nil, false, fmt.Errorf("failed to scan replay balance: %w", scanErr)
			}
			balances[productID] = balance
		}
		if rowsErr := replayRows.Err(); rowsErr != nil {
			replayRows.Close()
			return nil, false, fmt.Errorf("failed to iterate replay balances: %w", rowsErr)
		}
		replayRows.Close()

		if len(balances) != len(items) {
			return nil, false, fmt.Errorf(
				"incomplete consumption idempotency result: operation %s has %d of %d balances",
				input.OperationID, len(balances), len(items),
			)
		}
		for _, item := range items {
			if _, found := balances[item.ProductID]; !found {
				return nil, false, fmt.Errorf(
					"incomplete consumption idempotency result: operation %s has no balance for product %d",
					input.OperationID, item.ProductID,
				)
			}
		}
		if err := tx.Rollback(ctx); err != nil {
			return nil, false, fmt.Errorf("failed to roll back idempotent stock replay: %w", err)
		}
		return balances, true, nil
	case err != nil:
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "consumption_operations_invoice_fingerprint_key" {
			return nil, false, ErrIdempotencyConflict
		}
		return nil, false, fmt.Errorf("failed to persist consumption idempotency record: %w", err)
	}

	productIDs := make([]int64, len(items))
	quantities := make(map[int64]int, len(items))
	for index, item := range items {
		productIDs[index] = item.ProductID
		quantities[item.ProductID] = item.Quantity
	}
	slices.Sort(productIDs)

	const lockQuery = `
		SELECT id, balance
		FROM products
		WHERE id = ANY($1)
		ORDER BY id ASC
		FOR UPDATE
	`
	rows, err := tx.Query(ctx, lockQuery, productIDs)
	if err != nil {
		return nil, false, fmt.Errorf("failed to lock products: %w", err)
	}

	balances := make(map[int64]int, len(items))
	var missingIDs []int64
	for rows.Next() {
		var productID int64
		var balance int
		if err := rows.Scan(&productID, &balance); err != nil {
			rows.Close()
			return nil, false, fmt.Errorf("failed to scan locked product: %w", err)
		}
		balances[productID] = balance
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, false, fmt.Errorf("failed to iterate locked products: %w", err)
	}
	rows.Close()

	for _, productID := range productIDs {
		if _, found := balances[productID]; !found {
			missingIDs = append(missingIDs, productID)
		}
	}
	if len(missingIDs) > 0 {
		return nil, false, fmt.Errorf("%w: ids=%v", ErrNotFound, missingIDs)
	}

	for _, productID := range productIDs {
		quantity := quantities[productID]
		const updateQuery = `
			UPDATE products
			SET balance = balance - $1,
				updated_at = now()
			WHERE id = $2 AND balance >= $1
			RETURNING balance
		`
		var balance int
		err := tx.QueryRow(ctx, updateQuery, quantity, productID).Scan(&balance)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, false, fmt.Errorf(
				"%w: id=%d required=%d available=%d",
				ErrInsufficientStock, productID, quantity, balances[productID],
			)
		}
		if err != nil {
			return nil, false, fmt.Errorf("failed to consume product stock: %w", err)
		}
		balances[productID] = balance
	}

	const persistResultQuery = `
		INSERT INTO consumption_operation_results (operation_id, product_id, balance)
		VALUES ($1, $2, $3)
	`
	for _, productID := range productIDs {
		if _, err := tx.Exec(ctx, persistResultQuery, input.OperationID, productID, balances[productID]); err != nil {
			return nil, false, fmt.Errorf("failed to persist consumption result: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, false, fmt.Errorf("failed to commit stock consumption: %w", err)
	}
	return balances, false, nil
}

// Create inserts a new product into the database.
// It returns the created product with database-generated fields.
func (r *Repository) Create(ctx context.Context, code, description string, balance int) (Product, error) {
	const query = `
		INSERT INTO products (code, description, balance)
		VALUES ($1, $2, $3)
		RETURNING id, code, description, balance, created_at, updated_at
	`

	var p Product
	err := r.pool.QueryRow(ctx, query, code, description, balance).Scan(
		&p.ID, &p.Code, &p.Description, &p.Balance, &p.CreatedAt, &p.UpdatedAt,
	)
	if err != nil {
		if isCodeUniqueViolation(err) {
			return Product{}, fmt.Errorf("%w: %s", ErrCodeConflict, code)
		}
		return Product{}, fmt.Errorf("failed to create product: %w", err)
	}

	return p, nil
}

// List returns all products ordered by ID.
// It returns an empty slice if no products exist, never nil.
func (r *Repository) List(ctx context.Context) ([]Product, error) {
	const query = `
		SELECT id, code, description, balance, created_at, updated_at
		FROM products
		ORDER BY id ASC
	`

	rows, err := r.pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to list products: %w", err)
	}
	defer rows.Close()

	var products []Product
	for rows.Next() {
		var p Product
		if err := rows.Scan(
			&p.ID, &p.Code, &p.Description, &p.Balance, &p.CreatedAt, &p.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan product: %w", err)
		}
		products = append(products, p)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating products: %w", err)
	}

	// Return empty slice instead of nil for JSON-friendly behavior
	if products == nil {
		products = []Product{}
	}

	return products, nil
}

// GetByID returns a product by its ID.
// It returns ErrNotFound if the product does not exist.
func (r *Repository) GetByID(ctx context.Context, id int64) (Product, error) {
	const query = `
		SELECT id, code, description, balance, created_at, updated_at
		FROM products
		WHERE id = $1
	`

	var p Product
	err := r.pool.QueryRow(ctx, query, id).Scan(
		&p.ID, &p.Code, &p.Description, &p.Balance, &p.CreatedAt, &p.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Product{}, ErrNotFound
		}
		return Product{}, fmt.Errorf("failed to get product by id: %w", err)
	}

	return p, nil
}

// GetByIDs returns multiple products by their IDs in a single query.
// It returns a map of ID to Product for found products.
// Missing IDs are not included in the result.
func (r *Repository) GetByIDs(ctx context.Context, ids []int64) (map[int64]Product, error) {
	if len(ids) == 0 {
		return map[int64]Product{}, nil
	}

	const query = `
		SELECT id, code, description, balance, created_at, updated_at
		FROM products
		WHERE id = ANY($1)
	`

	rows, err := r.pool.Query(ctx, query, ids)
	if err != nil {
		return nil, fmt.Errorf("failed to get products by ids: %w", err)
	}
	defer rows.Close()

	products := make(map[int64]Product)
	for rows.Next() {
		var p Product
		if err := rows.Scan(
			&p.ID, &p.Code, &p.Description, &p.Balance, &p.CreatedAt, &p.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan product: %w", err)
		}
		products[p.ID] = p
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating products: %w", err)
	}

	return products, nil
}

// isCodeUniqueViolation reports only the unique constraint owned by Product.code.
func isCodeUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23505" && pgErr.ConstraintName == "products_code_key"
	}
	return false
}
