package invoice

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/EnzoDias2006/korp-api/services/billing/internal/stock"
)

// Status is the persisted invoice lifecycle state.
type Status string

const (
	// StatusOpen is the initial state of every invoice.
	StatusOpen Status = "OPEN"
	// StatusClosed is reserved for the later finalization workflow.
	StatusClosed Status = "CLOSED"
)

var (
	// ErrValidation classifies invalid invoice input.
	ErrValidation = errors.New("invoice validation failed")
	// ErrItemsRequired reports an invoice without items.
	ErrItemsRequired = fmt.Errorf("%w: at least one item is required", ErrValidation)
	// ErrInvalidInvoiceID reports a non-positive invoice identity.
	ErrInvalidInvoiceID = fmt.Errorf("%w: invoice id must be positive", ErrValidation)
	// ErrInvalidProductID reports a non-positive product identity.
	ErrInvalidProductID = fmt.Errorf("%w: product id must be positive", ErrValidation)
	// ErrInvalidQuantity reports a non-positive item quantity.
	ErrInvalidQuantity = fmt.Errorf("%w: item quantity must be positive", ErrValidation)
	// ErrInvalidProductSnapshot reports missing historical product data.
	ErrInvalidProductSnapshot = fmt.Errorf("%w: product snapshot is required", ErrValidation)
	// ErrDuplicateProduct reports repeated product identities in one invoice.
	ErrDuplicateProduct = fmt.Errorf("%w: duplicate product", ErrValidation)
	// ErrNotFound reports a missing invoice.
	ErrNotFound = errors.New("invoice not found")
	// ErrNotOpen reports a print attempt on a non-OPEN invoice.
	ErrNotOpen = errors.New("invoice is not open")
	// ErrIdempotencyConflict reports a finalization operation reused for another command.
	ErrIdempotencyConflict = errors.New("finalization idempotency conflict")
)

// StockConsumer is Billing's narrow consumer-side boundary to Stock.
type StockConsumer interface {
	Consume(ctx context.Context, request stock.ConsumeRequest, requestID string) error
}

// Item is the historical product snapshot and quantity persisted on an invoice.
type Item struct {
	ProductID          int64
	ProductCode        string
	ProductDescription string
	Quantity           int
}

// Invoice is a Billing-owned invoice and its deterministically ordered items.
type Invoice struct {
	ID        int64
	Number    int64
	Status    Status
	CreatedAt time.Time
	ClosedAt  *time.Time
	Items     []Item
}

// CreateItem is a caller-supplied product snapshot for invoice creation.
type CreateItem struct {
	ProductID          int64
	ProductCode        string
	ProductDescription string
	Quantity           int
}

// CreateInput contains the items for a new invoice.
type CreateInput struct {
	Items []CreateItem
}

// FinalizationOperation is the durable identity of one logical invoice print.
type FinalizationOperation struct {
	InvoiceID   int64
	OperationID [16]byte
}

// Store is the persistence boundary consumed by Service.
type Store interface {
	Create(ctx context.Context, items []CreateItem) (Invoice, error)
	List(ctx context.Context) ([]Invoice, error)
	GetByID(ctx context.Context, id int64) (Invoice, error)
	ListConsumptions(ctx context.Context, invoiceID int64) ([]stock.ConsumeItem, error)
	StartFinalization(ctx context.Context, invoiceID int64) (FinalizationOperation, error)
	CompleteFinalization(ctx context.Context, invoiceID int64, operationID [16]byte) (Invoice, bool, error)
}

// Service validates invoice operations before persistence.
type Service struct {
	store Store
}

// NewService creates an invoice Service.
func NewService(store Store) *Service {
	return &Service{store: store}
}

// Create validates and normalizes caller-supplied snapshots, then atomically
// persists an OPEN invoice and its items through the Store.
func (s *Service) Create(ctx context.Context, input CreateInput) (Invoice, error) {
	if len(input.Items) == 0 {
		return Invoice{}, ErrItemsRequired
	}

	items := make([]CreateItem, len(input.Items))
	seenProductIDs := make(map[int64]struct{}, len(input.Items))
	for index, item := range input.Items {
		if item.ProductID <= 0 {
			return Invoice{}, fmt.Errorf("item %d: %w", index, ErrInvalidProductID)
		}
		if item.Quantity <= 0 {
			return Invoice{}, fmt.Errorf("item %d: %w", index, ErrInvalidQuantity)
		}

		item.ProductCode = strings.TrimSpace(item.ProductCode)
		item.ProductDescription = strings.TrimSpace(item.ProductDescription)
		if item.ProductCode == "" || item.ProductDescription == "" {
			return Invoice{}, fmt.Errorf("item %d: %w", index, ErrInvalidProductSnapshot)
		}
		if _, duplicate := seenProductIDs[item.ProductID]; duplicate {
			return Invoice{}, fmt.Errorf("item %d product %d: %w", index, item.ProductID, ErrDuplicateProduct)
		}

		seenProductIDs[item.ProductID] = struct{}{}
		items[index] = item
	}

	return s.store.Create(ctx, items)
}

// List returns invoices in the Store's deterministic order.
func (s *Service) List(ctx context.Context) ([]Invoice, error) {
	return s.store.List(ctx)
}

// GetByID validates id and returns an invoice with its items.
func (s *Service) GetByID(ctx context.Context, id int64) (Invoice, error) {
	if id <= 0 {
		return Invoice{}, ErrInvalidInvoiceID
	}
	return s.store.GetByID(ctx, id)
}

// Print finalizes an OPEN invoice through one durable operation identity. A retry
// reuses that identity so Stock can replay the original outcome without a second decrement.
func (s *Service) Print(ctx context.Context, invoiceID int64, consumer StockConsumer, requestID string) (Invoice, [16]byte, error) {
	if invoiceID <= 0 {
		return Invoice{}, [16]byte{}, ErrInvalidInvoiceID
	}

	current, err := s.store.GetByID(ctx, invoiceID)
	if err != nil {
		return Invoice{}, [16]byte{}, err
	}
	if current.Status != StatusOpen {
		return Invoice{}, [16]byte{}, ErrNotOpen
	}

	items, err := s.store.ListConsumptions(ctx, invoiceID)
	if err != nil {
		return Invoice{}, [16]byte{}, fmt.Errorf("load invoice consumptions: %w", err)
	}
	if len(items) == 0 {
		return Invoice{}, [16]byte{}, ErrItemsRequired
	}
	for _, item := range items {
		if item.ProductID <= 0 {
			return Invoice{}, [16]byte{}, ErrInvalidProductID
		}
		if item.Quantity <= 0 {
			return Invoice{}, [16]byte{}, ErrInvalidQuantity
		}
	}

	operation, err := s.store.StartFinalization(ctx, invoiceID)
	if err != nil {
		return Invoice{}, [16]byte{}, fmt.Errorf("start invoice finalization: %w", err)
	}

	request := stock.ConsumeRequest{
		InvoiceID:   invoiceID,
		Items:       items,
		OperationID: operation.OperationID,
	}
	if err := consumer.Consume(ctx, request, requestID); err != nil {
		var serviceError *stock.ServiceError
		if errors.As(err, &serviceError) && serviceError.Status == http.StatusConflict && serviceError.Code == "IDEMPOTENCY_CONFLICT" {
			return Invoice{}, operation.OperationID, ErrIdempotencyConflict
		}
		return Invoice{}, operation.OperationID, err
	}

	closed, updated, err := s.store.CompleteFinalization(ctx, invoiceID, operation.OperationID)
	if err != nil {
		return closed, operation.OperationID, fmt.Errorf("complete invoice finalization: %w", err)
	}
	if !updated {
		return closed, operation.OperationID, ErrNotOpen
	}
	return closed, operation.OperationID, nil
}
