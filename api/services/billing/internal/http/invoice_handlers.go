package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/EnzoDias2006/korp-api/services/billing/internal/invoice"
	"github.com/EnzoDias2006/korp-api/services/billing/internal/stock"
	"github.com/gin-gonic/gin"
)

const maxInvoiceCreateBodyBytes = 64 << 10

// InvoiceHandlers translates Billing invoice HTTP requests to application calls.
type InvoiceHandlers struct {
	invoices invoiceService
	products productResolver
	logger   *slog.Logger
}

type invoiceCreateRequest struct {
	Items []invoiceCreateItemRequest `json:"items"`
}

type invoiceCreateItemRequest struct {
	ProductID int64 `json:"product_id"`
	Quantity  int   `json:"quantity"`
}

type invoiceItemResponse struct {
	ProductID          int64  `json:"product_id"`
	ProductCode        string `json:"product_code"`
	ProductDescription string `json:"product_description"`
	Quantity           int    `json:"quantity"`
}

type invoiceSummaryResponse struct {
	ID        int64   `json:"id"`
	Number    int64   `json:"number"`
	Status    string  `json:"status"`
	CreatedAt string  `json:"created_at"`
	ClosedAt  *string `json:"closed_at"`
}

type invoiceResponse struct {
	ID        int64                 `json:"id"`
	Number    int64                 `json:"number"`
	Status    string                `json:"status"`
	CreatedAt string                `json:"created_at"`
	ClosedAt  *string               `json:"closed_at"`
	Items     []invoiceItemResponse `json:"items"`
}

// NewInvoiceHandlers creates the Billing invoice HTTP handlers.
func NewInvoiceHandlers(invoices invoiceService, products productResolver, logger *slog.Logger) *InvoiceHandlers {
	return &InvoiceHandlers{invoices: invoices, products: products, logger: logger}
}

// Create handles POST /api/v1/invoices.
func (h *InvoiceHandlers) Create(c *gin.Context) {
	requestID := GetRequestID(c.Request.Context())

	var request invoiceCreateRequest
	if err := decodeInvoiceCreateRequest(c, &request); err != nil {
		writeInvoiceError(c, http.StatusBadRequest, "MALFORMED_REQUEST", "The request body is malformed or contains invalid JSON.")
		return
	}
	productIDs, err := validateInvoiceCreateRequest(request)
	if err != nil {
		writeInvoiceError(c, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "The provided input is semantically invalid.")
		return
	}

	resolved, err := h.products.ResolveProducts(c.Request.Context(), productIDs, requestID)
	if err != nil {
		h.handleResolveError(c, err, requestID)
		return
	}

	items := make([]invoice.CreateItem, len(request.Items))
	for index, requested := range request.Items {
		product, ok := resolved[requested.ProductID]
		if !ok {
			h.logError("stock resolve response omitted a requested product",
				"request_id", requestID,
				"product_id", requested.ProductID,
				"operation", "create_invoice",
			)
			writeInvoiceError(c, http.StatusServiceUnavailable, "STOCK_SERVICE_UNAVAILABLE", "Could not resolve invoice products.")
			return
		}
		items[index] = invoice.CreateItem{
			ProductID:          product.ID,
			ProductCode:        product.Code,
			ProductDescription: product.Description,
			Quantity:           requested.Quantity,
		}
	}

	created, err := h.invoices.Create(c.Request.Context(), invoice.CreateInput{Items: items})
	if err != nil {
		if errors.Is(err, invoice.ErrValidation) {
			writeInvoiceError(c, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "The provided input is semantically invalid.")
			return
		}
		h.logError("invoice creation failed",
			"request_id", requestID,
			"operation", "create_invoice",
			"error", err,
		)
		writeInvoiceError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "An unexpected error occurred while processing the request.")
		return
	}

	c.JSON(http.StatusCreated, invoiceDetailFromDomain(created))
}

// List handles GET /api/v1/invoices.
func (h *InvoiceHandlers) List(c *gin.Context) {
	requestID := GetRequestID(c.Request.Context())
	invoices, err := h.invoices.List(c.Request.Context())
	if err != nil {
		h.logError("invoice list failed",
			"request_id", requestID,
			"operation", "list_invoices",
			"error", err,
		)
		writeInvoiceError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "An unexpected error occurred while processing the request.")
		return
	}

	response := make([]invoiceSummaryResponse, len(invoices))
	for index, current := range invoices {
		response[index] = invoiceSummaryFromDomain(current)
	}
	c.JSON(http.StatusOK, response)
}

// GetByID handles GET /api/v1/invoices/:id.
func (h *InvoiceHandlers) GetByID(c *gin.Context) {
	requestID := GetRequestID(c.Request.Context())
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		writeInvoiceError(c, http.StatusBadRequest, "INVALID_INVOICE_ID", "The invoice ID must be a positive integer.")
		return
	}

	found, err := h.invoices.GetByID(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, invoice.ErrNotFound) {
			writeInvoiceError(c, http.StatusNotFound, "INVOICE_NOT_FOUND", "The requested invoice was not found.")
			return
		}
		if errors.Is(err, invoice.ErrValidation) {
			writeInvoiceError(c, http.StatusBadRequest, "INVALID_INVOICE_ID", "The invoice ID must be a positive integer.")
			return
		}
		h.logError("invoice query failed",
			"request_id", requestID,
			"invoice_id", id,
			"operation", "get_invoice",
			"error", err,
		)
		writeInvoiceError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "An unexpected error occurred while processing the request.")
		return
	}

	c.JSON(http.StatusOK, invoiceDetailFromDomain(found))
}

// Print handles POST /api/v1/invoices/:id/print.
func (h *InvoiceHandlers) Print(c *gin.Context) {
	requestID := GetRequestID(c.Request.Context())
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		writeInvoiceError(c, http.StatusBadRequest, "INVALID_INVOICE_ID", "The invoice ID must be a positive integer.")
		return
	}

	var consumer invoice.StockConsumer
	if resolverWithConsume, ok := h.products.(invoice.StockConsumer); ok {
		consumer = resolverWithConsume
	}

	if consumer == nil {
		h.logPrintError("stock consumer is not configured", requestID, id, [16]byte{}, http.StatusServiceUnavailable, nil)
		writeInvoiceError(c, http.StatusServiceUnavailable, "STOCK_SERVICE_UNAVAILABLE", "Could not update product stock.")
		return
	}

	finalized, operationID, err := h.invoices.Print(c.Request.Context(), id, consumer, requestID)
	if err != nil {
		switch {
		case errors.Is(err, invoice.ErrNotFound):
			writeInvoiceError(c, http.StatusNotFound, "INVOICE_NOT_FOUND", "The requested invoice was not found.")
		case errors.Is(err, invoice.ErrNotOpen):
			writeInvoiceError(c, http.StatusConflict, "INVOICE_NOT_OPEN", "Only an OPEN invoice can be printed.")
		case errors.Is(err, invoice.ErrValidation):
			h.logPrintError("invoice print data is invalid", requestID, id, operationID, http.StatusInternalServerError, err)
			writeInvoiceError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "An unexpected error occurred while processing the request.")
		case errors.Is(err, stock.ErrUnavailable):
			h.logPrintError("stock consumption unavailable", requestID, id, operationID, http.StatusServiceUnavailable, err)
			writeInvoiceError(c, http.StatusServiceUnavailable, "STOCK_SERVICE_UNAVAILABLE", "Could not update product stock.")
		default:
			var serviceError *stock.ServiceError
			if errors.As(err, &serviceError) && serviceError.Status == http.StatusConflict && serviceError.Code == "INSUFFICIENT_STOCK" {
				writeInvoiceError(c, http.StatusConflict, "INSUFFICIENT_STOCK", "There is insufficient stock to print the invoice.")
				return
			}
			if errors.Is(err, invoice.ErrIdempotencyConflict) {
				writeInvoiceError(c, http.StatusConflict, "IDEMPOTENCY_CONFLICT", "The finalization operation conflicts with a prior command.")
				return
			}
			h.logPrintError("invoice print failed", requestID, id, operationID, http.StatusInternalServerError, err)
			writeInvoiceError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "An unexpected error occurred while processing the request.")
		}
		return
	}

	c.JSON(http.StatusOK, invoiceDetailFromDomain(finalized))
}

func (h *InvoiceHandlers) logPrintError(
	message string,
	requestID string,
	invoiceID int64,
	operationID [16]byte,
	status int,
	err error,
) {
	attributes := []any{
		"request_id", requestID,
		"invoice_id", invoiceID,
		"operation", "print_invoice",
		"status", status,
	}
	if operationID != ([16]byte{}) {
		operationText, marshalErr := stock.UUID(operationID).MarshalText()
		if marshalErr == nil {
			attributes = append(attributes, "operation_id", string(operationText))
		} else {
			attributes = append(attributes, "operation_id", operationID)
		}
	}
	if err != nil {
		attributes = append(attributes, "error", err)
	}
	h.logError(message, attributes...)
}

func (h *InvoiceHandlers) handleResolveError(c *gin.Context, err error, requestID string) {
	var serviceError *stock.ServiceError
	if errors.As(err, &serviceError) {
		switch {
		case serviceError.Status == http.StatusNotFound && serviceError.Code == "PRODUCT_NOT_FOUND":
			writeInvoiceError(c, http.StatusNotFound, "PRODUCT_NOT_FOUND", "One or more requested products were not found.")
			return
		case serviceError.Status == http.StatusUnprocessableEntity && serviceError.Code == "VALIDATION_ERROR":
			writeInvoiceError(c, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "The provided input is semantically invalid.")
			return
		}
	}

	h.logError("stock product resolution failed",
		"request_id", requestID,
		"operation", "resolve_invoice_products",
		"error", err,
	)
	writeInvoiceError(c, http.StatusServiceUnavailable, "STOCK_SERVICE_UNAVAILABLE", "Could not resolve invoice products.")
}

func (h *InvoiceHandlers) logError(message string, attributes ...any) {
	if h.logger != nil {
		h.logger.Error(message, append([]any{"service", "billing"}, attributes...)...)
	}
}

func decodeInvoiceCreateRequest(c *gin.Context, request *invoiceCreateRequest) error {
	mediaType, _, err := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if err != nil || (mediaType != "application/json" && !strings.HasSuffix(mediaType, "+json")) {
		return errors.New("content type is not JSON")
	}

	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxInvoiceCreateBodyBytes)
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(request); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("request body must contain one JSON object")
	}
	return nil
}

func validateInvoiceCreateRequest(request invoiceCreateRequest) ([]int64, error) {
	if len(request.Items) == 0 {
		return nil, invoice.ErrItemsRequired
	}

	productIDs := make([]int64, len(request.Items))
	seen := make(map[int64]struct{}, len(request.Items))
	for index, item := range request.Items {
		if item.ProductID <= 0 {
			return nil, invoice.ErrInvalidProductID
		}
		if item.Quantity <= 0 {
			return nil, invoice.ErrInvalidQuantity
		}
		if _, duplicate := seen[item.ProductID]; duplicate {
			return nil, invoice.ErrDuplicateProduct
		}
		seen[item.ProductID] = struct{}{}
		productIDs[index] = item.ProductID
	}
	return productIDs, nil
}

func writeInvoiceError(c *gin.Context, status int, code, message string) {
	ErrorHandler(c, nil, code, message, status)
}

func invoiceSummaryFromDomain(current invoice.Invoice) invoiceSummaryResponse {
	return invoiceSummaryResponse{
		ID:        current.ID,
		Number:    current.Number,
		Status:    string(current.Status),
		CreatedAt: current.CreatedAt.UTC().Format(time.RFC3339Nano),
		ClosedAt:  formatOptionalTime(current.ClosedAt),
	}
}

func invoiceDetailFromDomain(current invoice.Invoice) invoiceResponse {
	items := make([]invoiceItemResponse, len(current.Items))
	for index, item := range current.Items {
		items[index] = invoiceItemResponse{
			ProductID:          item.ProductID,
			ProductCode:        item.ProductCode,
			ProductDescription: item.ProductDescription,
			Quantity:           item.Quantity,
		}
	}

	return invoiceResponse{
		ID:        current.ID,
		Number:    current.Number,
		Status:    string(current.Status),
		CreatedAt: current.CreatedAt.UTC().Format(time.RFC3339Nano),
		ClosedAt:  formatOptionalTime(current.ClosedAt),
		Items:     items,
	}
}

func formatOptionalTime(value *time.Time) *string {
	if value == nil {
		return nil
	}
	formatted := value.UTC().Format(time.RFC3339Nano)
	return &formatted
}
