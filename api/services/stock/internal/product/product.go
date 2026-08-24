package product

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
)

// ErrValidation is returned when input validation fails.
var ErrValidation = errors.New("validation failed")

// ErrCodeConflict is returned when a product code already exists.
var ErrCodeConflict = errors.New("product code already exists")

// ErrNotFound is returned when a product is not found.
var ErrNotFound = errors.New("product not found")

// ErrInsufficientStock is returned when one or more products lack balance.
var ErrInsufficientStock = errors.New("insufficient stock")

// ErrIdempotencyConflict is returned when an operation ID was already used for
// a different durable consumption command.
var ErrIdempotencyConflict = errors.New("idempotency conflict")

// StockItem identifies one product quantity to consume.
type StockItem struct {
	ProductID int64
	Quantity  int
}

// OperationID is the caller's stable logical command identity.
type OperationID [16]byte

func (id OperationID) MarshalJSON() ([]byte, error) {
	return json.Marshal(id.String())
}

func (id *OperationID) UnmarshalJSON(data []byte) error {
	var value string
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	cleaned := make([]byte, 0, len(value))
	for _, character := range value {
		if character != '-' {
			cleaned = append(cleaned, byte(character))
		}
	}
	if len(cleaned) != 32 {
		return errors.New("invalid operation ID length")
	}
	if _, err := hex.Decode(id[:], cleaned); err != nil {
		return fmt.Errorf("decode operation ID: %w", err)
	}
	return nil
}

func (id OperationID) String() string {
	text := make([]byte, 36)
	hex.Encode(text[0:8], id[0:4])
	text[8] = '-'
	hex.Encode(text[9:13], id[4:6])
	text[13] = '-'
	hex.Encode(text[14:18], id[6:8])
	text[18] = '-'
	hex.Encode(text[19:23], id[8:10])
	text[23] = '-'
	hex.Encode(text[24:], id[10:])
	return string(text)
}

// ConsumeInput represents an atomic multi-product stock consumption command.
type ConsumeInput struct {
	InvoiceID   int64
	Items       []StockItem
	OperationID OperationID
}

// ConsumeResult is the resulting stock balance for every consumed product.
type ConsumeResult struct {
	Balances map[int64]int
}

// Product represents a product with stock balance.
type Product struct {
	ID          int64
	Code        string
	Description string
	Balance     int
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// CreateInput represents the input for creating a new product.
type CreateInput struct {
	Code        string
	Description string
	Balance     int
}

// Store defines the interface for product persistence.
type Store interface {
	Create(ctx context.Context, code, description string, balance int) (Product, error)
	List(ctx context.Context) ([]Product, error)
	GetByID(ctx context.Context, id int64) (Product, error)
	GetByIDs(ctx context.Context, ids []int64) (map[int64]Product, error)
	Consume(ctx context.Context, input ConsumeInput) (map[int64]int, bool, error)
}

// Service provides the product domain logic.
type Service struct {
	store Store
}

// NewService creates a new product Service with the given Store.
func NewService(store Store) *Service {
	return &Service{store: store}
}

// Create creates a new product with normalized code and description.
// It validates the input and returns the created product or an error.
func (s *Service) Create(ctx context.Context, input CreateInput) (Product, error) {
	code := strings.TrimSpace(strings.ToUpper(input.Code))
	description := strings.TrimSpace(input.Description)

	if code == "" {
		return Product{}, ErrValidation
	}
	if description == "" {
		return Product{}, ErrValidation
	}
	if input.Balance < 0 {
		return Product{}, ErrValidation
	}

	return s.store.Create(ctx, code, description, input.Balance)
}

// List returns all products ordered by ID.
func (s *Service) List(ctx context.Context) ([]Product, error) {
	return s.store.List(ctx)
}

// GetByID returns a product by its ID.
func (s *Service) GetByID(ctx context.Context, id int64) (Product, error) {
	return s.store.GetByID(ctx, id)
}

// Consume atomically consumes all requested quantities or none of them.
func (s *Service) Consume(ctx context.Context, input ConsumeInput) (ConsumeResult, bool, error) {
	if input.InvoiceID <= 0 || len(input.Items) == 0 {
		return ConsumeResult{}, false, ErrValidation
	}

	items := make([]StockItem, len(input.Items))
	copy(items, input.Items)
	for _, item := range items {
		if item.ProductID <= 0 || item.Quantity <= 0 {
			return ConsumeResult{}, false, ErrValidation
		}
	}
	if hasDuplicateStockIDs(items) {
		return ConsumeResult{}, false, ErrValidation
	}

	balances, replayed, err := s.store.Consume(ctx, input)
	if err != nil {
		if errors.Is(err, ErrIdempotencyConflict) {
			err = fmt.Errorf("%w: operation fingerprint does not match", ErrIdempotencyConflict)
		}
		return ConsumeResult{}, false, err
	}
	return ConsumeResult{Balances: balances}, replayed, nil
}

func hasDuplicateStockIDs(items []StockItem) bool {
	seen := make(map[int64]bool, len(items))
	for _, item := range items {
		if seen[item.ProductID] {
			return true
		}
		seen[item.ProductID] = true
	}
	return false
}

// ConsumptionFingerprint canonically hashes the logical command. Sorting makes
// JSON array order irrelevant while preserving every quantity.
func ConsumptionFingerprint(invoiceID int64, items []StockItem) [sha256.Size]byte {
	sorted := make([]StockItem, len(items))
	copy(sorted, items)
	slices.SortFunc(sorted, func(a, b StockItem) int {
		switch {
		case a.ProductID < b.ProductID:
			return -1
		case a.ProductID > b.ProductID:
			return 1
		default:
			return a.Quantity - b.Quantity
		}
	})

	digest := sha256.New()
	var invoiceBytes [8]byte
	binary.BigEndian.PutUint64(invoiceBytes[:], uint64(invoiceID))
	digest.Write(invoiceBytes[:])
	for _, item := range sorted {
		var value [8]byte
		binary.BigEndian.PutUint64(value[:], uint64(item.ProductID))
		digest.Write(value[:])
		binary.BigEndian.PutUint32(value[:4], uint32(item.Quantity))
		digest.Write(value[:4])
	}

	var fingerprint [sha256.Size]byte
	copy(fingerprint[:], digest.Sum(nil))
	return fingerprint
}

// ErrInvalidID is returned when an ID validation fails.
var ErrInvalidID = errors.New("invalid product id")

// ResolveInput represents the input for resolving multiple products.
type ResolveInput struct {
	IDs []int64
}

// ResolveResult contains the resolved products indexed by ID.
type ResolveResult struct {
	Products map[int64]Product
	Missing  []int64
}

// Resolve resolves multiple products by their IDs in a single operation.
// It validates the input IDs and returns found products and missing IDs.
// Returns an error only for system failures, not for missing products.
func (s *Service) Resolve(ctx context.Context, input ResolveInput) (ResolveResult, error) {
	// Validate input
	if len(input.IDs) == 0 {
		return ResolveResult{
			Products: make(map[int64]Product),
			Missing:  []int64{},
		}, nil
	}

	// Validate all IDs are positive
	for _, id := range input.IDs {
		if id <= 0 {
			return ResolveResult{}, ErrInvalidID
		}
	}

	// Get products in a single query
	products, err := s.store.GetByIDs(ctx, input.IDs)
	if err != nil {
		return ResolveResult{}, fmt.Errorf("failed to resolve products: %w", err)
	}

	// Determine missing IDs
	missing := make([]int64, 0, len(input.IDs))
	for _, id := range input.IDs {
		if _, exists := products[id]; !exists {
			missing = append(missing, id)
		}
	}

	return ResolveResult{
		Products: products,
		Missing:  missing,
	}, nil
}
