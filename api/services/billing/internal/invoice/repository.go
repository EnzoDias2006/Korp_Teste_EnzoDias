package invoice

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/EnzoDias2006/korp-api/services/billing/internal/stock"
)

// Repository persists invoices in Billing's PostgreSQL database.
type Repository struct {
	pool *pgxpool.Pool
}

// NewRepository creates an invoice Repository.
func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

// Create persists an OPEN invoice and every item in one local transaction.
// PostgreSQL generates the unique sequential invoice number.
func (r *Repository) Create(ctx context.Context, items []CreateItem) (created Invoice, err error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return Invoice{}, fmt.Errorf("begin invoice transaction: %w", err)
	}
	defer func() {
		if rollbackErr := tx.Rollback(ctx); rollbackErr != nil && !errors.Is(rollbackErr, pgx.ErrTxClosed) && err == nil {
			err = fmt.Errorf("roll back invoice transaction: %w", rollbackErr)
		}
	}()

	const insertInvoice = `
		INSERT INTO invoices DEFAULT VALUES
		RETURNING id, number, status, created_at, closed_at
	`
	if err = tx.QueryRow(ctx, insertInvoice).Scan(
		&created.ID,
		&created.Number,
		&created.Status,
		&created.CreatedAt,
		&created.ClosedAt,
	); err != nil {
		return Invoice{}, fmt.Errorf("insert invoice: %w", err)
	}

	const insertItem = `
		INSERT INTO invoice_items (
			invoice_id,
			product_id,
			product_code_snapshot,
			product_description_snapshot,
			quantity
		)
		VALUES ($1, $2, $3, $4, $5)
	`
	created.Items = make([]Item, 0, len(items))
	for _, item := range items {
		if _, err = tx.Exec(
			ctx,
			insertItem,
			created.ID,
			item.ProductID,
			item.ProductCode,
			item.ProductDescription,
			item.Quantity,
		); err != nil {
			return Invoice{}, fmt.Errorf("insert invoice item for product %d: %w", item.ProductID, err)
		}
		created.Items = append(created.Items, Item(item))
	}

	if err = tx.Commit(ctx); err != nil {
		return Invoice{}, fmt.Errorf("commit invoice transaction: %w", err)
	}
	return created, nil
}

// List returns every invoice ordered by number and every item ordered by its
// insertion identity. Empty results are returned as non-nil slices.
func (r *Repository) List(ctx context.Context) ([]Invoice, error) {
	const query = `
		SELECT id, number, status, created_at, closed_at
		FROM invoices
		ORDER BY number ASC, id ASC
	`
	rows, err := r.pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list invoices: %w", err)
	}
	defer rows.Close()

	invoices := make([]Invoice, 0)
	invoiceIndexes := make(map[int64]int)
	invoiceIDs := make([]int64, 0)
	for rows.Next() {
		var current Invoice
		if err := rows.Scan(
			&current.ID,
			&current.Number,
			&current.Status,
			&current.CreatedAt,
			&current.ClosedAt,
		); err != nil {
			return nil, fmt.Errorf("scan invoice: %w", err)
		}
		current.Items = []Item{}
		invoiceIndexes[current.ID] = len(invoices)
		invoiceIDs = append(invoiceIDs, current.ID)
		invoices = append(invoices, current)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate invoices: %w", err)
	}
	if len(invoiceIDs) == 0 {
		return invoices, nil
	}

	itemsByInvoice, err := r.listItems(ctx, invoiceIDs)
	if err != nil {
		return nil, err
	}
	for invoiceID, items := range itemsByInvoice {
		invoices[invoiceIndexes[invoiceID]].Items = items
	}
	return invoices, nil
}

// GetByID returns one invoice and its items, or ErrNotFound.
func (r *Repository) GetByID(ctx context.Context, id int64) (Invoice, error) {
	return getInvoiceByID(ctx, r.pool, id)
}

func (r *Repository) listItems(ctx context.Context, invoiceIDs []int64) (map[int64][]Item, error) {
	const query = `
		SELECT invoice_id, product_id, product_code_snapshot, product_description_snapshot, quantity
		FROM invoice_items
		WHERE invoice_id = ANY($1)
		ORDER BY invoice_id ASC, id ASC
	`
	rows, err := r.pool.Query(ctx, query, invoiceIDs)
	if err != nil {
		return nil, fmt.Errorf("list invoice items: %w", err)
	}
	defer rows.Close()

	itemsByInvoice := make(map[int64][]Item, len(invoiceIDs))
	for rows.Next() {
		var invoiceID int64
		var item Item
		if err := rows.Scan(
			&invoiceID,
			&item.ProductID,
			&item.ProductCode,
			&item.ProductDescription,
			&item.Quantity,
		); err != nil {
			return nil, fmt.Errorf("scan invoice item: %w", err)
		}
		itemsByInvoice[invoiceID] = append(itemsByInvoice[invoiceID], item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate invoice items: %w", err)
	}
	return itemsByInvoice, nil
}

// StartFinalization atomically creates or returns the one durable operation for
// an OPEN invoice. PostgreSQL's unique invoice_id key serializes concurrent callers.
func (r *Repository) StartFinalization(ctx context.Context, invoiceID int64) (FinalizationOperation, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return FinalizationOperation{}, fmt.Errorf("begin finalization claim: %w", err)
	}
	defer tx.Rollback(ctx)

	const lockQuery = `SELECT status FROM invoices WHERE id = $1 FOR UPDATE`
	var status Status
	if err := tx.QueryRow(ctx, lockQuery, invoiceID).Scan(&status); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return FinalizationOperation{}, ErrNotFound
		}
		return FinalizationOperation{}, fmt.Errorf("lock invoice for finalization: %w", err)
	}
	if status != StatusOpen {
		return FinalizationOperation{}, ErrNotOpen
	}

	operation := FinalizationOperation{InvoiceID: invoiceID}
	const upsertQuery = `
		INSERT INTO invoice_finalizations (invoice_id, operation_id)
		VALUES ($1, gen_random_uuid())
		ON CONFLICT (invoice_id) DO UPDATE
		SET invoice_id = invoice_finalizations.invoice_id
		RETURNING operation_id
	`
	var operationID [16]byte
	if err := tx.QueryRow(ctx, upsertQuery, invoiceID).Scan(&operationID); err != nil {
		return FinalizationOperation{}, fmt.Errorf("claim finalization operation: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return FinalizationOperation{}, fmt.Errorf("commit finalization claim: %w", err)
	}
	operation.OperationID = operationID
	return operation, nil
}

// CompleteFinalization atomically transitions an OPEN invoice to CLOSED, marks
// its matching durable operation complete, and loads the response invoice. The
// second return value is true only when this caller performed the one winning
// transition. Any completion or item-loading failure rolls back the close.
func (r *Repository) CompleteFinalization(ctx context.Context, invoiceID int64, operationID [16]byte) (closed Invoice, updated bool, err error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return Invoice{}, false, fmt.Errorf("begin invoice completion: %w", err)
	}
	defer func() {
		if rollbackErr := tx.Rollback(ctx); rollbackErr != nil && !errors.Is(rollbackErr, pgx.ErrTxClosed) && err == nil {
			err = fmt.Errorf("roll back invoice completion: %w", rollbackErr)
		}
	}()

	const closeQuery = `
		UPDATE invoices
		SET status = 'CLOSED', closed_at = now()
		WHERE id = $1 AND status = 'OPEN'
		RETURNING id, number, status, created_at, closed_at
	`
	if err = tx.QueryRow(ctx, closeQuery, invoiceID).Scan(
		&closed.ID,
		&closed.Number,
		&closed.Status,
		&closed.CreatedAt,
		&closed.ClosedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			closed, err = getInvoiceByID(ctx, tx, invoiceID)
			if err != nil {
				return Invoice{}, false, err
			}
			if err = tx.Commit(ctx); err != nil {
				return Invoice{}, false, fmt.Errorf("commit unchanged invoice completion: %w", err)
			}
			return closed, false, nil
		}
		return Invoice{}, false, fmt.Errorf("close open invoice: %w", err)
	}

	const completeQuery = `
		UPDATE invoice_finalizations
		SET completed_at = now()
		WHERE invoice_id = $1 AND operation_id = $2
	`
	result, err := tx.Exec(ctx, completeQuery, invoiceID, operationID)
	if err != nil {
		return Invoice{}, false, fmt.Errorf("complete finalization operation: %w", err)
	}
	if result.RowsAffected() != 1 {
		return Invoice{}, false, fmt.Errorf("complete finalization operation: operation not found")
	}

	closed.Items, err = listInvoiceItems(ctx, tx, invoiceID)
	if err != nil {
		return Invoice{}, false, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Invoice{}, false, fmt.Errorf("commit invoice completion: %w", err)
	}
	return closed, true, nil
}

type invoiceQuerier interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

func getInvoiceByID(ctx context.Context, querier invoiceQuerier, invoiceID int64) (Invoice, error) {
	const query = `
		SELECT id, number, status, created_at, closed_at
		FROM invoices
		WHERE id = $1
	`
	var found Invoice
	if err := querier.QueryRow(ctx, query, invoiceID).Scan(
		&found.ID,
		&found.Number,
		&found.Status,
		&found.CreatedAt,
		&found.ClosedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Invoice{}, ErrNotFound
		}
		return Invoice{}, fmt.Errorf("get invoice by id: %w", err)
	}
	items, err := listInvoiceItems(ctx, querier, invoiceID)
	if err != nil {
		return Invoice{}, err
	}
	found.Items = items
	return found, nil
}

func listInvoiceItems(ctx context.Context, querier invoiceQuerier, invoiceID int64) ([]Item, error) {
	const query = `
		SELECT product_id, product_code_snapshot, product_description_snapshot, quantity
		FROM invoice_items
		WHERE invoice_id = $1
		ORDER BY id ASC
	`
	rows, err := querier.Query(ctx, query, invoiceID)
	if err != nil {
		return nil, fmt.Errorf("list invoice items: %w", err)
	}
	defer rows.Close()

	items := make([]Item, 0)
	for rows.Next() {
		var item Item
		if err := rows.Scan(
			&item.ProductID,
			&item.ProductCode,
			&item.ProductDescription,
			&item.Quantity,
		); err != nil {
			return nil, fmt.Errorf("scan invoice item: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate invoice items: %w", err)
	}
	return items, nil
}

// ListConsumptions returns the persisted command for an invoice in item order.
func (r *Repository) ListConsumptions(ctx context.Context, invoiceID int64) ([]stock.ConsumeItem, error) {
	const query = `
		SELECT product_id, quantity
		FROM invoice_items
		WHERE invoice_id = $1
		ORDER BY id ASC
	`
	rows, err := r.pool.Query(ctx, query, invoiceID)
	if err != nil {
		return nil, fmt.Errorf("list invoice consumptions: %w", err)
	}
	defer rows.Close()

	items := make([]stock.ConsumeItem, 0)
	for rows.Next() {
		var item stock.ConsumeItem
		if err := rows.Scan(&item.ProductID, &item.Quantity); err != nil {
			return nil, fmt.Errorf("scan invoice consumption: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate invoice consumptions: %w", err)
	}
	return items, nil
}

var _ Store = (*Repository)(nil)
