package product

import (
	"context"
	"errors"
	"testing"
)

// mockStore is a mock implementation of Store for testing.
type mockStore struct {
	createFunc   func(ctx context.Context, code, description string, balance int) (Product, error)
	listFunc     func(ctx context.Context) ([]Product, error)
	getByIDFunc  func(ctx context.Context, id int64) (Product, error)
	getByIDsFunc func(ctx context.Context, ids []int64) (map[int64]Product, error)
	consumeFunc  func(ctx context.Context, input ConsumeInput) (map[int64]int, bool, error)
}

func (m *mockStore) Create(ctx context.Context, code, description string, balance int) (Product, error) {
	return m.createFunc(ctx, code, description, balance)
}

func (m *mockStore) List(ctx context.Context) ([]Product, error) {
	return m.listFunc(ctx)
}

func (m *mockStore) GetByID(ctx context.Context, id int64) (Product, error) {
	return m.getByIDFunc(ctx, id)
}

func (m *mockStore) GetByIDs(ctx context.Context, ids []int64) (map[int64]Product, error) {
	return m.getByIDsFunc(ctx, ids)
}

func (m *mockStore) Consume(ctx context.Context, input ConsumeInput) (map[int64]int, bool, error) {
	if m.consumeFunc == nil {
		return nil, false, errors.New("unexpected consume call")
	}
	return m.consumeFunc(ctx, input)
}

func TestService_Create_NormalizesCode(t *testing.T) {
	ctx := context.Background()
	service := NewService(&mockStore{
		createFunc: func(ctx context.Context, code, description string, balance int) (Product, error) {
			// Verify normalization was applied
			if code != "ABC" {
				t.Errorf("expected code to be normalized to 'ABC', got '%s'", code)
			}
			if description != "Test Product" {
				t.Errorf("expected description to be 'Test Product', got '%s'", description)
			}
			return Product{
				ID:          1,
				Code:        code,
				Description: description,
				Balance:     balance,
			}, nil
		},
	})

	input := CreateInput{
		Code:        "  abc  ",
		Description: "  Test Product  ",
		Balance:     100,
	}

	product, err := service.Create(ctx, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if product.Code != "ABC" {
		t.Errorf("expected code 'ABC', got '%s'", product.Code)
	}
	if product.Description != "Test Product" {
		t.Errorf("expected description 'Test Product', got '%s'", product.Description)
	}
}

func TestService_Create_RejectsEmptyCode(t *testing.T) {
	ctx := context.Background()
	service := NewService(&mockStore{})

	input := CreateInput{
		Code:        "",
		Description: "Valid Description",
		Balance:     100,
	}

	_, err := service.Create(ctx, input)
	if err == nil {
		t.Fatal("expected error for empty code")
	}
	if !errors.Is(err, ErrValidation) {
		t.Errorf("expected ErrValidation, got %v", err)
	}
}

func TestService_Create_RejectsEmptyDescription(t *testing.T) {
	ctx := context.Background()
	service := NewService(&mockStore{})

	input := CreateInput{
		Code:        "ABC",
		Description: "",
		Balance:     100,
	}

	_, err := service.Create(ctx, input)
	if err == nil {
		t.Fatal("expected error for empty description")
	}
	if !errors.Is(err, ErrValidation) {
		t.Errorf("expected ErrValidation, got %v", err)
	}
}

func TestService_Create_RejectsNegativeBalance(t *testing.T) {
	ctx := context.Background()
	service := NewService(&mockStore{})

	input := CreateInput{
		Code:        "ABC",
		Description: "Valid Description",
		Balance:     -1,
	}

	_, err := service.Create(ctx, input)
	if err == nil {
		t.Fatal("expected error for negative balance")
	}
	if !errors.Is(err, ErrValidation) {
		t.Errorf("expected ErrValidation, got %v", err)
	}
}

func TestService_Create_DelegatesConflict(t *testing.T) {
	ctx := context.Background()
	service := NewService(&mockStore{
		createFunc: func(ctx context.Context, code, description string, balance int) (Product, error) {
			return Product{}, ErrCodeConflict
		},
	})

	input := CreateInput{
		Code:        "ABC",
		Description: "Test Product",
		Balance:     100,
	}

	_, err := service.Create(ctx, input)
	if err == nil {
		t.Fatal("expected error for code conflict")
	}
	if !errors.Is(err, ErrCodeConflict) {
		t.Errorf("expected ErrCodeConflict, got %v", err)
	}
}

func TestService_Create_DelegatesNotFound(t *testing.T) {
	ctx := context.Background()
	service := NewService(&mockStore{
		getByIDFunc: func(ctx context.Context, id int64) (Product, error) {
			return Product{}, ErrNotFound
		},
	})

	_, err := service.GetByID(ctx, 999)
	if err == nil {
		t.Fatal("expected error for not found")
	}
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestService_List_Delegates(t *testing.T) {
	ctx := context.Background()
	expected := []Product{
		{ID: 1, Code: "A", Description: "Desc A", Balance: 10},
		{ID: 2, Code: "B", Description: "Desc B", Balance: 20},
	}

	service := NewService(&mockStore{
		listFunc: func(ctx context.Context) ([]Product, error) {
			return expected, nil
		},
	})

	products, err := service.List(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(products) != len(expected) {
		t.Errorf("expected %d products, got %d", len(expected), len(products))
	}
}

func TestService_GetByID_Delegates(t *testing.T) {
	ctx := context.Background()
	expected := Product{
		ID:          1,
		Code:        "ABC",
		Description: "Test Product",
		Balance:     100,
	}

	service := NewService(&mockStore{
		getByIDFunc: func(ctx context.Context, id int64) (Product, error) {
			return expected, nil
		},
	})

	product, err := service.GetByID(ctx, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if product.ID != expected.ID {
		t.Errorf("expected ID %d, got %d", expected.ID, product.ID)
	}
}

func TestProduct_ImplementsStoreInterface(t *testing.T) {
	// Compile-time check that Repository implements Store
	var _ Store = (*Repository)(nil)
}

func TestService_Resolve_Success(t *testing.T) {
	ctx := context.Background()
	products := map[int64]Product{
		1: {ID: 1, Code: "P001", Description: "Product 1", Balance: 100},
		2: {ID: 2, Code: "P002", Description: "Product 2", Balance: 200},
	}

	service := NewService(&mockStore{
		getByIDsFunc: func(ctx context.Context, ids []int64) (map[int64]Product, error) {
			return products, nil
		},
	})

	input := ResolveInput{IDs: []int64{1, 2}}
	result, err := service.Resolve(ctx, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Products) != 2 {
		t.Errorf("expected 2 products, got %d", len(result.Products))
	}

	if result.Products[1].Code != "P001" {
		t.Errorf("expected product 1 code 'P001', got '%s'", result.Products[1].Code)
	}

	if result.Products[2].Code != "P002" {
		t.Errorf("expected product 2 code 'P002', got '%s'", result.Products[2].Code)
	}

	if len(result.Missing) != 0 {
		t.Errorf("expected 0 missing, got %d", len(result.Missing))
	}
}

func TestService_Resolve_WithMissing(t *testing.T) {
	ctx := context.Background()
	products := map[int64]Product{
		1: {ID: 1, Code: "P001", Description: "Product 1", Balance: 100},
	}

	service := NewService(&mockStore{
		getByIDsFunc: func(ctx context.Context, ids []int64) (map[int64]Product, error) {
			return products, nil
		},
	})

	input := ResolveInput{IDs: []int64{1, 999}}
	result, err := service.Resolve(ctx, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Products) != 1 {
		t.Errorf("expected 1 product, got %d", len(result.Products))
	}

	if len(result.Missing) != 1 {
		t.Errorf("expected 1 missing, got %d", len(result.Missing))
	}

	if result.Missing[0] != 999 {
		t.Errorf("expected missing ID 999, got %d", result.Missing[0])
	}
}

func TestService_Resolve_EmptyInput(t *testing.T) {
	ctx := context.Background()

	service := NewService(&mockStore{
		getByIDsFunc: func(ctx context.Context, ids []int64) (map[int64]Product, error) {
			return map[int64]Product{}, nil
		},
	})

	input := ResolveInput{IDs: []int64{}}
	result, err := service.Resolve(ctx, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Products) != 0 {
		t.Errorf("expected 0 products, got %d", len(result.Products))
	}

	if len(result.Missing) != 0 {
		t.Errorf("expected 0 missing, got %d", len(result.Missing))
	}
}

func TestService_Resolve_InvalidID(t *testing.T) {
	ctx := context.Background()

	service := NewService(&mockStore{})

	input := ResolveInput{IDs: []int64{0, -1}}
	_, err := service.Resolve(ctx, input)
	if err == nil {
		t.Fatal("expected error for invalid ID")
	}
	if !errors.Is(err, ErrInvalidID) {
		t.Errorf("expected ErrInvalidID, got %v", err)
	}
}

func TestService_Resolve_StoreError(t *testing.T) {
	ctx := context.Background()
	expectedErr := errors.New("store error")

	service := NewService(&mockStore{
		getByIDsFunc: func(ctx context.Context, ids []int64) (map[int64]Product, error) {
			return nil, expectedErr
		},
	})

	input := ResolveInput{IDs: []int64{1, 2}}
	_, err := service.Resolve(ctx, input)
	if err == nil {
		t.Fatal("expected error from store")
	}
}
