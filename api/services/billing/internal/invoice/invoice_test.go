package invoice

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"testing"
	"time"

	"github.com/EnzoDias2006/korp-api/services/billing/internal/stock"
)

type fakeStore struct {
	create               func(context.Context, []CreateItem) (Invoice, error)
	list                 func(context.Context) ([]Invoice, error)
	get                  func(context.Context, int64) (Invoice, error)
	listConsumptions     func(context.Context, int64) ([]stock.ConsumeItem, error)
	startFinalization    func(context.Context, int64) (FinalizationOperation, error)
	completeFinalization func(context.Context, int64, [16]byte) (Invoice, bool, error)
}

func (f *fakeStore) Create(ctx context.Context, items []CreateItem) (Invoice, error) {
	return f.create(ctx, items)
}

func (f *fakeStore) List(ctx context.Context) ([]Invoice, error) {
	return f.list(ctx)
}

func (f *fakeStore) GetByID(ctx context.Context, id int64) (Invoice, error) {
	return f.get(ctx, id)
}

func (f *fakeStore) ListConsumptions(ctx context.Context, id int64) ([]stock.ConsumeItem, error) {
	if f.listConsumptions == nil {
		return nil, errors.New("unexpected ListConsumptions call")
	}
	return f.listConsumptions(ctx, id)
}

func (f *fakeStore) StartFinalization(ctx context.Context, id int64) (FinalizationOperation, error) {
	if f.startFinalization == nil {
		return FinalizationOperation{}, errors.New("unexpected StartFinalization call")
	}
	return f.startFinalization(ctx, id)
}

func (f *fakeStore) CompleteFinalization(ctx context.Context, invoiceID int64, operationID [16]byte) (Invoice, bool, error) {
	if f.completeFinalization == nil {
		return Invoice{}, false, errors.New("unexpected CompleteFinalization call")
	}
	return f.completeFinalization(ctx, invoiceID, operationID)
}

func TestServiceCreateValidatesInput(t *testing.T) {
	tests := []struct {
		name  string
		input CreateInput
		want  error
	}{
		{name: "items required", input: CreateInput{}, want: ErrItemsRequired},
		{
			name: "positive product id",
			input: CreateInput{Items: []CreateItem{{
				ProductID: 0, ProductCode: "P1", ProductDescription: "Product", Quantity: 1,
			}}},
			want: ErrInvalidProductID,
		},
		{
			name: "positive quantity",
			input: CreateInput{Items: []CreateItem{{
				ProductID: 1, ProductCode: "P1", ProductDescription: "Product", Quantity: 0,
			}}},
			want: ErrInvalidQuantity,
		},
		{
			name: "code snapshot required",
			input: CreateInput{Items: []CreateItem{{
				ProductID: 1, ProductCode: " ", ProductDescription: "Product", Quantity: 1,
			}}},
			want: ErrInvalidProductSnapshot,
		},
		{
			name: "description snapshot required",
			input: CreateInput{Items: []CreateItem{{
				ProductID: 1, ProductCode: "P1", ProductDescription: " ", Quantity: 1,
			}}},
			want: ErrInvalidProductSnapshot,
		},
		{
			name: "duplicate product",
			input: CreateInput{Items: []CreateItem{
				{ProductID: 1, ProductCode: "P1", ProductDescription: "Product", Quantity: 1},
				{ProductID: 1, ProductCode: "P1", ProductDescription: "Product", Quantity: 2},
			}},
			want: ErrDuplicateProduct,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			service := NewService(&fakeStore{})
			_, err := service.Create(context.Background(), test.input)
			if !errors.Is(err, test.want) {
				t.Fatalf("Create() error = %v, want %v", err, test.want)
			}
			if !errors.Is(err, ErrValidation) {
				t.Fatalf("Create() error = %v, want ErrValidation classification", err)
			}
		})
	}
}

func TestServiceCreateNormalizesSnapshotsAndDelegates(t *testing.T) {
	wantItems := []CreateItem{{
		ProductID:          7,
		ProductCode:        "SKU-7",
		ProductDescription: "Product seven",
		Quantity:           3,
	}}
	store := &fakeStore{
		create: func(_ context.Context, items []CreateItem) (Invoice, error) {
			if !reflect.DeepEqual(items, wantItems) {
				t.Fatalf("Create() items = %#v, want %#v", items, wantItems)
			}
			return Invoice{ID: 11, Number: 42, Status: StatusOpen}, nil
		},
	}
	service := NewService(store)

	created, err := service.Create(context.Background(), CreateInput{Items: []CreateItem{{
		ProductID:          7,
		ProductCode:        "  SKU-7 ",
		ProductDescription: " Product seven  ",
		Quantity:           3,
	}}})
	if err != nil {
		t.Fatalf("Create() unexpected error: %v", err)
	}
	if created.Number != 42 || created.Status != StatusOpen {
		t.Fatalf("Create() = %#v, want number 42 and OPEN", created)
	}
}

func TestServiceQueriesDelegate(t *testing.T) {
	want := Invoice{ID: 9, Number: 3, Status: StatusOpen}
	store := &fakeStore{
		list: func(context.Context) ([]Invoice, error) {
			return []Invoice{want}, nil
		},
		get: func(_ context.Context, id int64) (Invoice, error) {
			if id != want.ID {
				t.Fatalf("GetByID() id = %d, want %d", id, want.ID)
			}
			return want, nil
		},
	}
	service := NewService(store)

	gotList, err := service.List(context.Background())
	if err != nil || !reflect.DeepEqual(gotList, []Invoice{want}) {
		t.Fatalf("List() = %#v, %v", gotList, err)
	}
	got, err := service.GetByID(context.Background(), want.ID)
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("GetByID() = %#v, %v", got, err)
	}
}

type fakeConsumer struct {
	consume func(context.Context, stock.ConsumeRequest, string) error
}

func fakeConsumerFunc(consume func(context.Context, stock.ConsumeRequest, string) error) *fakeConsumer {
	return &fakeConsumer{consume: consume}
}

func (f *fakeConsumer) Consume(ctx context.Context, request stock.ConsumeRequest, requestID string) error {
	if f.consume == nil {
		return errors.New("unexpected Consume call")
	}
	return f.consume(ctx, request, requestID)
}

func TestServicePrintValidatesBeforeStock(t *testing.T) {
	tests := []struct {
		name    string
		id      int64
		store   *fakeStore
		wantErr error
	}{
		{
			name: "missing invoice",
			id:   9,
			store: &fakeStore{get: func(context.Context, int64) (Invoice, error) {
				return Invoice{}, ErrNotFound
			}},
			wantErr: ErrNotFound,
		},
		{
			name: "closed invoice",
			id:   9,
			store: &fakeStore{get: func(context.Context, int64) (Invoice, error) {
				return Invoice{ID: 9, Status: StatusClosed}, nil
			}},
			wantErr: ErrNotOpen,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var consumerCalled bool
			service := NewService(test.store)
			_, _, err := service.Print(context.Background(), test.id, &fakeConsumer{
				consume: func(context.Context, stock.ConsumeRequest, string) error {
					consumerCalled = true
					return nil
				},
			}, "request-123")
			if !errors.Is(err, test.wantErr) {
				t.Fatalf("Print() error = %v, want %v", err, test.wantErr)
			}
			if consumerCalled {
				t.Fatal("Stock was called after invalid invoice state")
			}
		})
	}
}

func TestServicePrintBuildsPersistedCommandAndCompletesAfterSuccess(t *testing.T) {
	closedAt := time.Date(2026, time.August, 22, 10, 0, 0, 0, time.UTC)
	store := &fakeStore{
		get: func(_ context.Context, id int64) (Invoice, error) {
			if id != 11 {
				t.Fatalf("GetByID() id = %d, want 11", id)
			}
			return Invoice{ID: 11, Status: StatusOpen}, nil
		},
		listConsumptions: func(_ context.Context, id int64) ([]stock.ConsumeItem, error) {
			if id != 11 {
				t.Fatalf("ListConsumptions() id = %d, want 11", id)
			}
			return []stock.ConsumeItem{{ProductID: 7, Quantity: 2}, {ProductID: 8, Quantity: 1}}, nil
		},
		startFinalization: func(_ context.Context, id int64) (FinalizationOperation, error) {
			if id != 11 {
				t.Fatalf("StartFinalization() id = %d, want 11", id)
			}
			return FinalizationOperation{InvoiceID: 11, OperationID: [16]byte{1}}, nil
		},
		completeFinalization: func(_ context.Context, invoiceID int64, operationID [16]byte) (Invoice, bool, error) {
			if invoiceID != 11 || operationID != ([16]byte{1}) {
				t.Fatalf("CompleteFinalization() invoice/operation = %d/%v, want %d/%v", invoiceID, operationID, 11, [16]byte{1})
			}
			return Invoice{ID: 11, Status: StatusClosed, ClosedAt: &closedAt}, true, nil
		},
	}
	var consumeCalls int
	service := NewService(store)

	finalized, _, err := service.Print(context.Background(), 11, &fakeConsumer{
		consume: func(_ context.Context, request stock.ConsumeRequest, requestID string) error {
			consumeCalls++
			if requestID != "print-123" || len(request.Items) != 2 ||
				request.Items[0] != (stock.ConsumeItem{ProductID: 7, Quantity: 2}) ||
				request.Items[1] != (stock.ConsumeItem{ProductID: 8, Quantity: 1}) {
				t.Fatalf("Consume(request = %#v, requestID = %q)", request, requestID)
			}
			return nil
		},
	}, "print-123")
	if err != nil {
		t.Fatalf("Print() unexpected error: %v", err)
	}
	if consumeCalls != 1 {
		t.Fatalf("consume calls = %d, want exactly one", consumeCalls)
	}
	if finalized.Status != StatusClosed || finalized.ClosedAt == nil || !finalized.ClosedAt.Equal(closedAt) {
		t.Fatalf("Print() = %#v, want CLOSED at %s", finalized, closedAt)
	}
}

func TestServicePrintMapsStockRejectionAndLostTransition(t *testing.T) {
	t.Run("insufficient stock keeps invoice open", func(t *testing.T) {
		var completeCalled bool
		service := NewService(&fakeStore{
			get: func(context.Context, int64) (Invoice, error) {
				return Invoice{ID: 5, Status: StatusOpen}, nil
			},
			listConsumptions: func(context.Context, int64) ([]stock.ConsumeItem, error) {
				return []stock.ConsumeItem{{ProductID: 4, Quantity: 3}}, nil
			},
			startFinalization: func(context.Context, int64) (FinalizationOperation, error) {
				return FinalizationOperation{InvoiceID: 5, OperationID: [16]byte{2}}, nil
			},
			completeFinalization: func(context.Context, int64, [16]byte) (Invoice, bool, error) {
				completeCalled = true
				return Invoice{}, false, nil
			},
		})

		_, _, err := service.Print(context.Background(), 5, &fakeConsumer{
			consume: func(context.Context, stock.ConsumeRequest, string) error {
				return &stock.ServiceError{
					Code:    "INSUFFICIENT_STOCK",
					Status:  http.StatusConflict,
					Message: "Insufficient stock.",
				}
			},
		}, "conflict-123")
		var serviceError *stock.ServiceError
		if !errors.As(err, &serviceError) || serviceError.Code != "INSUFFICIENT_STOCK" {
			t.Fatalf("Print() error = %#v, want INSUFFICIENT_STOCK ServiceError", serviceError)
		}
		if completeCalled {
			t.Fatal("atomic completion was called before Stock success")
		}
	})

	t.Run("lost OPEN transition returns not open", func(t *testing.T) {
		current := Invoice{ID: 5, Status: StatusClosed}
		service := NewService(&fakeStore{
			get: func(context.Context, int64) (Invoice, error) {
				return Invoice{ID: 5, Status: StatusOpen}, nil
			},
			listConsumptions: func(context.Context, int64) ([]stock.ConsumeItem, error) {
				return []stock.ConsumeItem{{ProductID: 4, Quantity: 1}}, nil
			},
			completeFinalization: func(context.Context, int64, [16]byte) (Invoice, bool, error) {
				return current, false, nil
			},
			startFinalization: func(context.Context, int64) (FinalizationOperation, error) {
				return FinalizationOperation{InvoiceID: 5, OperationID: [16]byte{3}}, nil
			},
		})

		finalized, _, err := service.Print(context.Background(), 5, &fakeConsumer{
			consume: func(context.Context, stock.ConsumeRequest, string) error { return nil },
		}, "race-123")
		if !errors.Is(err, ErrNotOpen) || finalized.ID != current.ID {
			t.Fatalf("Print() = %#v, %v; want current invoice and ErrNotOpen", finalized, err)
		}
	})
}

func TestServicePrintLostResponseRecoversWithSameOperation(t *testing.T) {
	var operationID [16]byte
	operationID[0] = 11
	startCalls := 0
	started := false
	consumeRequests := make([]stock.ConsumeRequest, 0, 2)
	store := &fakeStore{
		get: func(_ context.Context, id int64) (Invoice, error) {
			if id != 21 {
				t.Fatalf("GetByID() id = %d, want 21", id)
			}
			return Invoice{ID: 21, Status: StatusOpen}, nil
		},
		listConsumptions: func(context.Context, int64) ([]stock.ConsumeItem, error) {
			return []stock.ConsumeItem{{ProductID: 8, Quantity: 1}}, nil
		},
		startFinalization: func(context.Context, int64) (FinalizationOperation, error) {
			if started {
				return FinalizationOperation{InvoiceID: 21, OperationID: operationID}, nil
			}
			startCalls++
			started = true
			if startCalls > 1 {
				t.Fatal("retry created a second finalization operation")
			}
			return FinalizationOperation{InvoiceID: 21, OperationID: operationID}, nil
		},
		completeFinalization: func(_ context.Context, id int64, completedID [16]byte) (Invoice, bool, error) {
			closedAt := time.Date(2026, time.August, 23, 12, 0, 0, 0, time.UTC)
			if len(consumeRequests) != 2 || consumeRequests[0].OperationID != operationID || consumeRequests[1].OperationID != operationID {
				t.Fatalf("consume requests = %#v, want same durable identity", consumeRequests)
			}
			if completedID != operationID {
				t.Fatalf("CompleteFinalization() operation = %v, want %v", completedID, operationID)
			}
			return Invoice{ID: id, Status: StatusClosed, ClosedAt: &closedAt}, true, nil
		},
	}

	attempt := 0
	service := NewService(store)
	finalized, returnedOperationID, err := service.Print(context.Background(), 21, fakeConsumerFunc(
		func(_ context.Context, request stock.ConsumeRequest, _ string) error {
			attempt++
			consumeRequests = append(consumeRequests, request)
			if attempt == 1 {
				return fmt.Errorf("%w: simulated lost response after Stock commit", stock.ErrUnavailable)
			}
			return nil
		}), "recovery-request")
	if err == nil || !errors.Is(err, stock.ErrUnavailable) || attempt != 1 {
		t.Fatalf("first Print() = %#v, %v, attempts %d; want unavailable once", finalized, err, attempt)
	}
	if returnedOperationID != operationID {
		t.Fatalf("returned operation ID = %v, want %v", returnedOperationID, operationID)
	}

	finalized, returnedOperationID, err = service.Print(context.Background(), 21, fakeConsumerFunc(
		func(_ context.Context, request stock.ConsumeRequest, _ string) error {
			attempt++
			consumeRequests = append(consumeRequests, request)
			return nil
		},
	), "retry-request")
	if err != nil || finalized.Status != StatusClosed {
		t.Fatalf("retry Print() = %#v, %v; want CLOSED", finalized, err)
	}
	if returnedOperationID != operationID || startCalls != 1 {
		t.Fatalf("retry operation = %v, starts = %d; want stable one operation", returnedOperationID, startCalls)
	}
}

func TestServiceGetByIDRejectsNonPositiveID(t *testing.T) {
	service := NewService(&fakeStore{})
	_, err := service.GetByID(context.Background(), 0)
	if !errors.Is(err, ErrInvalidInvoiceID) || !errors.Is(err, ErrValidation) {
		t.Fatalf("GetByID() error = %v, want ErrInvalidInvoiceID and ErrValidation", err)
	}
}
