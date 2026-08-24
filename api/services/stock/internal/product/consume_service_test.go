package product

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func TestService_Consume_Validation(t *testing.T) {
	tests := []struct {
		name  string
		input ConsumeInput
	}{
		{name: "zero invoice id", input: ConsumeInput{Items: []StockItem{{ProductID: 1, Quantity: 1}}}},
		{name: "empty items", input: ConsumeInput{InvoiceID: 1}},
		{name: "invalid product id", input: ConsumeInput{InvoiceID: 1, Items: []StockItem{{ProductID: 0, Quantity: 1}}}},
		{name: "invalid quantity", input: ConsumeInput{InvoiceID: 1, Items: []StockItem{{ProductID: 1, Quantity: 0}}}},
		{
			name: "duplicate product",
			input: ConsumeInput{InvoiceID: 1, Items: []StockItem{
				{ProductID: 1, Quantity: 1},
				{ProductID: 1, Quantity: 2},
			}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			storeCalled := false
			service := NewService(&mockStore{
				consumeFunc: func(context.Context, ConsumeInput) (map[int64]int, bool, error) {
					storeCalled = true
					return nil, false, nil
				},
			})

			_, _, err := service.Consume(context.Background(), tt.input)
			if !errors.Is(err, ErrValidation) {
				t.Fatalf("Consume() error = %v, want %v", err, ErrValidation)
			}
			if storeCalled {
				t.Fatal("Consume() called the store for invalid input")
			}
		})
	}
}

func TestService_Consume_DelegatesNormalizedItems(t *testing.T) {
	items := []StockItem{{ProductID: 7, Quantity: 3}, {ProductID: 2, Quantity: 5}}
	var gotID int64
	var gotItems []StockItem
	service := NewService(&mockStore{
		consumeFunc: func(_ context.Context, input ConsumeInput) (map[int64]int, bool, error) {
			gotID = input.InvoiceID
			gotItems = input.Items
			return map[int64]int{2: 0, 7: 4}, false, nil
		},
	})

	result, replayed, err := service.Consume(context.Background(), ConsumeInput{InvoiceID: 9, Items: items})
	if err != nil {
		t.Fatalf("Consume() returned error: %v", err)
	}
	if gotID != 9 {
		t.Errorf("invoice ID = %d, want 9", gotID)
	}
	if !reflect.DeepEqual(gotItems, items) {
		t.Errorf("items = %+v, want %+v", gotItems, items)
	}
	want := map[int64]int{2: 0, 7: 4}
	if !reflect.DeepEqual(result.Balances, want) {
		t.Errorf("balances = %+v, want %+v", result.Balances, want)
	}
	if replayed {
		t.Error("Consume() replayed = true, want false")
	}
}
