// Package httpapi provides HTTP handlers for the stock service.
package httpapi

import (
	"cmp"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/EnzoDias2006/korp-api/services/stock/internal/http/middleware"
	"github.com/EnzoDias2006/korp-api/services/stock/internal/product"
	"github.com/gin-gonic/gin"
)

// ProductHandlers handles product-related HTTP requests.
type ProductHandlers struct {
	service productService
	logger  *slog.Logger
}

// NewProductHandlers creates a new ProductHandlers instance.
func NewProductHandlers(service productService, logger *slog.Logger) *ProductHandlers {
	return &ProductHandlers{service: service, logger: logger}
}

// productResolveInput is the transport input for batch product resolution.
type productResolveInput struct {
	IDs []int64 `json:"ids"`
}

// productResolveResponse is a single product DTO in the resolve response.
type productResolveResponse struct {
	ID          int64  `json:"id"`
	Code        string `json:"code"`
	Description string `json:"description"`
	Balance     int    `json:"balance"`
}

// resolveResponse is the complete response for batch product resolution.
type resolveResponse struct {
	Products map[int64]productResolveResponse `json:"products"`
	Missing  []int64                          `json:"missing"`
}

// stockConsumeItem is one product quantity in the internal consume request.
type stockConsumeItem struct {
	ProductID *int64 `json:"product_id"`
	Quantity  *int   `json:"quantity"`
}

// stockConsumeInput is the strict transport input for atomic consumption.
type stockConsumeInput struct {
	InvoiceID   *int64               `json:"invoice_id"`
	OperationID *product.OperationID `json:"operation_id"`
	Items       []stockConsumeItem   `json:"items"`
}

// stockBalanceResponse is the balance resulting from consumption.
type stockBalanceResponse struct {
	ProductID int64 `json:"product_id"`
	Balance   int   `json:"balance"`
}

// stockConsumeResponse is the success DTO for atomic consumption.
type stockConsumeResponse struct {
	Balances []stockBalanceResponse `json:"balances"`
}

// productCreateInput is the transport input for product creation.
// It must match exactly the fields code, description, balance.
type productCreateInput struct {
	Code        string `json:"code"`
	Description string `json:"description"`
	Balance     *int   `json:"balance"`
}

// productResponse is the response DTO for a product.
type productResponse struct {
	ID          int64  `json:"id"`
	Code        string `json:"code"`
	Description string `json:"description"`
	Balance     int    `json:"balance"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

const maxProductCreateBodyBytes = 64 << 10
const maxStockConsumeBodyBytes = 64 << 10

// createProductFromDomain converts a domain Product to a response DTO.
func createProductFromDomain(p product.Product) productResponse {
	return productResponse{
		ID:          p.ID,
		Code:        p.Code,
		Description: p.Description,
		Balance:     p.Balance,
		CreatedAt:   p.CreatedAt.UTC().Format(time.RFC3339Nano),
		UpdatedAt:   p.UpdatedAt.UTC().Format(time.RFC3339Nano),
	}
}

// CreateProduct handles POST /api/v1/products.
func (h *ProductHandlers) CreateProduct(c *gin.Context) {
	requestID := middleware.GetRequestID(c)

	var input productCreateInput
	if err := decodeProductCreateInput(c, &input); err != nil {
		writeMalformedRequest(c, requestID)
		return
	}
	if input.Balance == nil {
		writeValidationError(c, requestID)
		return
	}

	createInput := product.CreateInput{
		Code:        input.Code,
		Description: input.Description,
		Balance:     *input.Balance,
	}

	p, err := h.service.Create(c.Request.Context(), createInput)
	if err != nil {
		h.handleProductError(c, err, requestID)
		return
	}

	response := createProductFromDomain(p)
	c.Header("X-Request-ID", requestID)
	c.JSON(http.StatusCreated, response)
}

func decodeProductCreateInput(c *gin.Context, input *productCreateInput) error {
	mediaType, _, err := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if err != nil || (mediaType != "application/json" && !strings.HasSuffix(mediaType, "+json")) {
		return errors.New("content type is not JSON")
	}

	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxProductCreateBodyBytes)
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(input); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("request body must contain one JSON object")
	}
	return nil
}

func writeMalformedRequest(c *gin.Context, requestID string) {
	middleware.WriteError(c, http.StatusBadRequest, middleware.NewErrorResponse(
		"MALFORMED_REQUEST",
		"The request body is malformed or contains invalid JSON.",
		nil,
		requestID,
	))
}

func writeValidationError(c *gin.Context, requestID string) {
	middleware.WriteError(c, http.StatusUnprocessableEntity, middleware.NewErrorResponse(
		"VALIDATION_ERROR",
		"The provided input is semantically invalid.",
		nil,
		requestID,
	))
}

// handleProductError maps domain errors to HTTP responses.
func (h *ProductHandlers) handleProductError(c *gin.Context, err error, requestID string) {
	if errors.Is(err, product.ErrValidation) {
		writeValidationError(c, requestID)
		return
	}

	if errors.Is(err, product.ErrCodeConflict) {
		middleware.WriteError(c, http.StatusConflict, middleware.NewErrorResponse(
			"PRODUCT_CODE_CONFLICT",
			"A product with the same code already exists.",
			nil,
			requestID,
		))
		return
	}

	h.logger.Error("product operation failed",
		"service", "stock",
		"request_id", requestID,
		"operation", "create_product",
		"error", err,
	)
	middleware.WriteError(c, http.StatusInternalServerError, middleware.NewErrorResponse(
		"INTERNAL_ERROR",
		"An unexpected error occurred while processing the request.",
		nil,
		requestID,
	))
}

// ListProducts handles GET /api/v1/products.
func (h *ProductHandlers) ListProducts(c *gin.Context) {
	requestID := middleware.GetRequestID(c)

	products, err := h.service.List(c.Request.Context())
	if err != nil {
		h.logger.Error("product operation failed",
			"service", "stock",
			"request_id", requestID,
			"operation", "list_products",
			"error", err,
		)
		middleware.WriteError(c, http.StatusInternalServerError, middleware.NewErrorResponse(
			"INTERNAL_ERROR",
			"An unexpected error occurred while processing the request.",
			nil,
			requestID,
		))
		return
	}

	responses := make([]productResponse, len(products))
	for i, p := range products {
		responses[i] = createProductFromDomain(p)
	}

	c.Header("X-Request-ID", requestID)
	c.JSON(http.StatusOK, responses)
}

// GetProduct handles GET /api/v1/products/:id.
func (h *ProductHandlers) GetProduct(c *gin.Context) {
	requestID := middleware.GetRequestID(c)

	// Parse ID from path parameter
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		middleware.WriteError(c, http.StatusBadRequest, middleware.NewErrorResponse(
			"INVALID_PRODUCT_ID",
			"The product ID must be a positive integer.",
			nil,
			requestID,
		))
		return
	}

	if id <= 0 {
		middleware.WriteError(c, http.StatusBadRequest, middleware.NewErrorResponse(
			"INVALID_PRODUCT_ID",
			"The product ID must be a positive integer.",
			nil,
			requestID,
		))
		return
	}

	p, err := h.service.GetByID(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, product.ErrNotFound) {
			middleware.WriteError(c, http.StatusNotFound, middleware.NewErrorResponse(
				"PRODUCT_NOT_FOUND",
				"The requested product was not found.",
				nil,
				requestID,
			))
			return
		}

		h.logger.Error("product operation failed",
			"service", "stock",
			"request_id", requestID,
			"product_id", id,
			"operation", "get_product",
			"error", err,
		)
		middleware.WriteError(c, http.StatusInternalServerError, middleware.NewErrorResponse(
			"INTERNAL_ERROR",
			"An unexpected error occurred while processing the request.",
			nil,
			requestID,
		))
		return
	}

	response := createProductFromDomain(p)
	c.Header("X-Request-ID", requestID)
	c.JSON(http.StatusOK, response)
}

// createResolveResponseFromDomain converts domain Products to resolve response DTOs.
func createResolveResponseFromDomain(products map[int64]product.Product) map[int64]productResolveResponse {
	result := make(map[int64]productResolveResponse, len(products))
	for id, p := range products {
		result[id] = productResolveResponse{
			ID:          p.ID,
			Code:        p.Code,
			Description: p.Description,
			Balance:     p.Balance,
		}
	}
	return result
}

const maxResolveBodyBytes = 64 << 10

// ResolveProducts handles POST /internal/v1/products/resolve.
func (h *ProductHandlers) ResolveProducts(c *gin.Context) {
	requestID := middleware.GetRequestID(c)

	var input productResolveInput
	if err := decodeResolveInput(c, &input); err != nil {
		middleware.WriteError(c, http.StatusBadRequest, middleware.NewErrorResponse(
			"MALFORMED_REQUEST",
			"The request body is malformed or contains invalid JSON.",
			nil,
			requestID,
		))
		return
	}

	// Validate semantic input: empty list is valid (returns empty products and missing)
	// But we need to check for duplicates
	if hasDuplicateIDs(input.IDs) {
		middleware.WriteError(c, http.StatusUnprocessableEntity, middleware.NewErrorResponse(
			"VALIDATION_ERROR",
			"The request contains duplicate product IDs.",
			nil,
			requestID,
		))
		return
	}

	resolveInput := product.ResolveInput{IDs: input.IDs}
	result, err := h.service.Resolve(c.Request.Context(), resolveInput)
	if err != nil {
		if errors.Is(err, product.ErrInvalidID) {
			middleware.WriteError(c, http.StatusUnprocessableEntity, middleware.NewErrorResponse(
				"VALIDATION_ERROR",
				"The provided input is semantically invalid.",
				nil,
				requestID,
			))
			return
		}

		h.logger.Error("product resolve operation failed",
			"service", "stock",
			"request_id", requestID,
			"operation", "resolve_products",
			"error", err,
		)
		middleware.WriteError(c, http.StatusInternalServerError, middleware.NewErrorResponse(
			"INTERNAL_ERROR",
			"An unexpected error occurred while processing the request.",
			nil,
			requestID,
		))
		return
	}

	// Check if there are any missing products
	if len(result.Missing) > 0 {
		middleware.WriteError(c, http.StatusNotFound, middleware.NewErrorResponse(
			"PRODUCT_NOT_FOUND",
			"One or more requested products were not found.",
			nil,
			requestID,
		))
		return
	}

	response := resolveResponse{
		Products: createResolveResponseFromDomain(result.Products),
		Missing:  result.Missing,
	}

	c.Header("X-Request-ID", requestID)
	c.JSON(http.StatusOK, response)
}

// decodeResolveInput decodes and validates the resolve request body.
func decodeResolveInput(c *gin.Context, input *productResolveInput) error {
	mediaType, _, err := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if err != nil || (mediaType != "application/json" && !strings.HasSuffix(mediaType, "+json")) {
		return errors.New("content type is not JSON")
	}

	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxResolveBodyBytes)
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(input); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("request body must contain one JSON object")
	}
	return nil
}

// hasDuplicateIDs checks if the slice contains duplicate IDs.
func hasDuplicateIDs(ids []int64) bool {
	seen := make(map[int64]bool, len(ids))
	for _, id := range ids {
		if seen[id] {
			return true
		}
		seen[id] = true
	}
	return false
}

func decodeStockConsumeInput(c *gin.Context, input *stockConsumeInput) error {
	mediaType, _, err := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if err != nil || (mediaType != "application/json" && !strings.HasSuffix(mediaType, "+json")) {
		return errors.New("content type is not JSON")
	}

	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxStockConsumeBodyBytes)
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(input); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("request body must contain one JSON object")
	}
	return nil
}

// ConsumeStock handles POST /internal/v1/stock/consume.
func (h *ProductHandlers) ConsumeStock(c *gin.Context) {
	requestID := middleware.GetRequestID(c)

	var input stockConsumeInput
	if err := decodeStockConsumeInput(c, &input); err != nil {
		writeMalformedRequest(c, requestID)
		return
	}
	if !isValidStockTransportInput(input) {
		writeValidationError(c, requestID)
		return
	}

	items := make([]product.StockItem, len(input.Items))
	for index, item := range input.Items {
		items[index] = product.StockItem{
			ProductID: *item.ProductID,
			Quantity:  *item.Quantity,
		}
	}

	result, _, err := h.service.Consume(c.Request.Context(), product.ConsumeInput{
		InvoiceID:   *input.InvoiceID,
		Items:       items,
		OperationID: *input.OperationID,
	})
	if err != nil {
		h.handleConsumeError(c, err, requestID, *input.InvoiceID, *input.OperationID)
		return
	}

	c.Header("X-Request-ID", requestID)
	c.JSON(http.StatusOK, createStockConsumeResponse(result.Balances))
}

func isValidStockTransportInput(input stockConsumeInput) bool {
	if input.InvoiceID == nil || *input.InvoiceID <= 0 || input.OperationID == nil || isZeroUUID(*input.OperationID) || len(input.Items) == 0 {
		return false
	}
	productIDs := make(map[int64]bool, len(input.Items))
	for _, item := range input.Items {
		if item.ProductID == nil || item.Quantity == nil || *item.ProductID <= 0 || *item.Quantity <= 0 {
			return false
		}
		if productIDs[*item.ProductID] {
			return false
		}
		productIDs[*item.ProductID] = true
	}
	return true
}

func isZeroUUID(value [16]byte) bool { return value == [16]byte{} }

func createStockConsumeResponse(balances map[int64]int) stockConsumeResponse {
	response := stockConsumeResponse{Balances: make([]stockBalanceResponse, 0, len(balances))}
	for productID, balance := range balances {
		response.Balances = append(response.Balances, stockBalanceResponse{
			ProductID: productID,
			Balance:   balance,
		})
	}
	slices.SortFunc(response.Balances, func(a, b stockBalanceResponse) int {
		return cmp.Compare(a.ProductID, b.ProductID)
	})
	return response
}

func (h *ProductHandlers) handleConsumeError(
	c *gin.Context,
	err error,
	requestID string,
	invoiceID int64,
	operationID product.OperationID,
) {
	switch {
	case errors.Is(err, product.ErrValidation):
		writeValidationError(c, requestID)
	case errors.Is(err, product.ErrNotFound):
		middleware.WriteError(c, http.StatusNotFound, middleware.NewErrorResponse(
			"PRODUCT_NOT_FOUND",
			"One or more requested products were not found.",
			nil,
			requestID,
		))
	case errors.Is(err, product.ErrIdempotencyConflict):
		middleware.WriteError(c, http.StatusConflict, middleware.NewErrorResponse(
			"IDEMPOTENCY_CONFLICT",
			"The operation ID was already used with a different command.",
			nil,
			requestID,
		))
	case errors.Is(err, product.ErrInsufficientStock):
		middleware.WriteError(c, http.StatusConflict, middleware.NewErrorResponse(
			"INSUFFICIENT_STOCK",
			"One or more products do not have enough stock.",
			nil,
			requestID,
		))
	default:
		h.logger.Error("stock consume operation failed",
			"service", "stock",
			"request_id", requestID,
			"invoice_id", invoiceID,
			"operation_id", operationID,
			"operation", "consume_stock",
			"status", http.StatusInternalServerError,
			"error", err,
		)
		middleware.WriteError(c, http.StatusInternalServerError, middleware.NewErrorResponse(
			"INTERNAL_ERROR",
			"An unexpected error occurred while processing the request.",
			nil,
			requestID,
		))
	}
}
