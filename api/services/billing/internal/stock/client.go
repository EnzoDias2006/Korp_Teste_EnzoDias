// Package stock provides Billing's HTTP client boundary to the Stock Service.
package stock

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strings"
	"time"
)

const (
	// DefaultClientTimeout is the default timeout for Stock Service HTTP calls.
	DefaultClientTimeout = 5 * time.Second
	maxResponseBodyBytes = 1 << 20
)

var (
	// ErrUnavailable classifies a network failure or an unusable Stock response.
	ErrUnavailable = errors.New("stock service unavailable")
	// ErrInvalidResponse classifies a Stock response that violates the resolve contract.
	ErrInvalidResponse = errors.New("invalid stock service response")
)

// Client is the HTTP client boundary for the Stock Service.
type Client struct {
	client         *http.Client
	baseURL        string
	defaultTimeout time.Duration
}

// ClientConfig holds configuration for the Stock Service client.
type ClientConfig struct {
	BaseURL string
	Timeout time.Duration
}

// ResolveRequest is the Stock batch-resolve request DTO.
type ResolveRequest struct {
	IDs []int64 `json:"ids"`
}

// ResolvedProduct is the trusted Stock snapshot returned to Billing.
type ResolvedProduct struct {
	ID          int64  `json:"id"`
	Code        string `json:"code"`
	Description string `json:"description"`
	Balance     int    `json:"balance"`
}

// ResolveResponse is the Stock batch-resolve response DTO.
type ResolveResponse struct {
	Products map[int64]ResolvedProduct `json:"products"`
	Missing  []int64                   `json:"missing"`
}

// ConsumeItem is one product and quantity to consume in a Stock command.
type ConsumeItem struct {
	ProductID int64 `json:"product_id"`
	Quantity  int   `json:"quantity"`
}

// ConsumeRequest is the Stock atomic-consume request DTO.
type ConsumeRequest struct {
	InvoiceID   int64         `json:"invoice_id"`
	Items       []ConsumeItem `json:"items"`
	OperationID UUID          `json:"operation_id"`
}

type consumeResponse struct {
	Balances []consumeBalance `json:"balances"`
}

type consumeBalance struct {
	ProductID int64 `json:"product_id"`
	Balance   *int  `json:"balance"`
}

type UUID [16]byte

func (id UUID) MarshalJSON() ([]byte, error) {
	text, err := id.MarshalText()
	if err != nil {
		return nil, err
	}
	return json.Marshal(string(text))
}

func (id UUID) MarshalText() ([]byte, error) {
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
	return text, nil
}

func (id *UUID) UnmarshalText(text []byte) error {
	cleaned := make([]byte, 0, len(text))
	for _, character := range text {
		if character != '-' {
			cleaned = append(cleaned, character)
		}
	}
	if len(cleaned) != 32 {
		return errors.New("invalid UUID length")
	}
	if _, err := hex.Decode(id[:], cleaned); err != nil {
		return fmt.Errorf("decode UUID: %w", err)
	}
	return nil
}

// ErrorResponse is the nested error envelope returned by Stock.
type ErrorResponse struct {
	Error ErrorBody `json:"error"`
}

// ErrorBody contains the stable Stock error fields.
type ErrorBody struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	Details   any    `json:"details"`
	RequestID string `json:"request_id"`
}

// ServiceError is a valid business rejection returned by Stock.
type ServiceError struct {
	Code      string
	Message   string
	Details   any
	RequestID string
	Status    int
}

func (e *ServiceError) Error() string {
	return fmt.Sprintf("stock service rejected request with %s (status %d)", e.Code, e.Status)
}

// NewClient creates a Stock Service HTTP client with an explicit timeout.
func NewClient(config ClientConfig) *Client {
	if config.Timeout <= 0 {
		config.Timeout = DefaultClientTimeout
	}

	return &Client{
		client: &http.Client{
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
			Transport: &http.Transport{
				MaxIdleConns:        10,
				MaxIdleConnsPerHost: 5,
				IdleConnTimeout:     30 * time.Second,
			},
			Timeout: config.Timeout,
		},
		baseURL:        strings.TrimRight(config.BaseURL, "/"),
		defaultTimeout: config.Timeout,
	}
}

// Do executes an absolute HTTP request with the supplied context.
// Callers must close every returned response body.
func (c *Client) Do(ctx context.Context, req *http.Request) (*http.Response, error) {
	if req.URL.Scheme == "" || req.URL.Host == "" {
		return nil, errors.New("absolute URL required")
	}
	return c.client.Do(req.WithContext(ctx))
}

// ResolveProducts resolves all requested products in one Stock call without changing balance.
func (c *Client) ResolveProducts(ctx context.Context, productIDs []int64, requestID string) (map[int64]ResolvedProduct, error) {
	body, err := json.Marshal(ResolveRequest{IDs: productIDs})
	if err != nil {
		return nil, fmt.Errorf("encode stock resolve request: %w", err)
	}

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		c.baseURL+"/internal/v1/products/resolve",
		bytes.NewReader(body),
	)
	if err != nil {
		return nil, fmt.Errorf("build stock resolve request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if requestID != "" {
		req.Header.Set("X-Request-ID", requestID)
	}

	response, err := c.Do(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("%w: execute resolve request: %v", ErrUnavailable, err)
	}
	defer response.Body.Close()

	responseBody, err := readResponseBody(response)
	if err != nil {
		return nil, err
	}

	if response.StatusCode >= http.StatusInternalServerError {
		return nil, fmt.Errorf("%w: unusable resolve response with status %d", ErrUnavailable, response.StatusCode)
	}

	if response.StatusCode >= http.StatusBadRequest {
		var stockError ErrorResponse
		if err := decodeJSON(responseBody, &stockError); err != nil || stockError.Error.Code == "" {
			return nil, fmt.Errorf("%w: unusable error response with status %d", ErrUnavailable, response.StatusCode)
		}
		return nil, &ServiceError{
			Code:      stockError.Error.Code,
			Message:   stockError.Error.Message,
			Details:   stockError.Error.Details,
			RequestID: stockError.Error.RequestID,
			Status:    response.StatusCode,
		}
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("%w: unusable resolve response with status %d", ErrUnavailable, response.StatusCode)
	}

	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: unexpected success status %d", ErrUnavailable, response.StatusCode)
	}

	var resolved ResolveResponse
	if err := decodeJSON(responseBody, &resolved); err != nil {
		return nil, fmt.Errorf("%w: decode resolve response: %v", ErrUnavailable, err)
	}
	if len(resolved.Missing) > 0 {
		return nil, &ServiceError{
			Code:    "PRODUCT_NOT_FOUND",
			Message: "One or more requested products were not found.",
			Status:  http.StatusNotFound,
		}
	}
	if err := validateResolvedProducts(productIDs, resolved.Products); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrUnavailable, err)
	}

	return resolved.Products, nil
}

// Consume atomically consumes all requested quantities in exactly one Stock call.
func (c *Client) Consume(ctx context.Context, request ConsumeRequest, requestID string) error {
	body, err := json.Marshal(request)
	if err != nil {
		return fmt.Errorf("encode stock consume request: %w", err)
	}

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		c.baseURL+"/internal/v1/stock/consume",
		bytes.NewReader(body),
	)
	if err != nil {
		return fmt.Errorf("build stock consume request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if requestID != "" {
		req.Header.Set("X-Request-ID", requestID)
	}

	response, err := c.Do(ctx, req)
	if err != nil {
		return fmt.Errorf("%w: execute consume request: %v", ErrUnavailable, err)
	}
	defer response.Body.Close()

	responseBody, err := readResponseBody(response)
	if err != nil {
		return err
	}

	if response.StatusCode >= http.StatusInternalServerError {
		return fmt.Errorf("%w: unusable consume response with status %d", ErrUnavailable, response.StatusCode)
	}

	if response.StatusCode >= http.StatusBadRequest {
		var stockError ErrorResponse
		if err := decodeJSON(responseBody, &stockError); err != nil || stockError.Error.Code == "" {
			return fmt.Errorf("%w: unusable error response with status %d", ErrUnavailable, response.StatusCode)
		}
		return &ServiceError{
			Code:      stockError.Error.Code,
			Message:   stockError.Error.Message,
			Details:   stockError.Error.Details,
			RequestID: stockError.Error.RequestID,
			Status:    response.StatusCode,
		}
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("%w: unusable consume response with status %d", ErrUnavailable, response.StatusCode)
	}

	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("%w: unusable consume response with status %d", ErrUnavailable, response.StatusCode)
	}

	var result consumeResponse
	if err := decodeJSON(responseBody, &result); err != nil {
		return fmt.Errorf("%w: decode consume response: %v", ErrUnavailable, err)
	}
	if err := validateConsumeBalances(request.Items, result.Balances); err != nil {
		return fmt.Errorf("%w: %w", ErrUnavailable, err)
	}
	return nil
}

func readResponseBody(response *http.Response) ([]byte, error) {
	if response.StatusCode == http.StatusNoContent {
		return nil, nil
	}

	mediaType, _, err := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if err != nil || (mediaType != "application/json" && !strings.HasSuffix(mediaType, "+json")) {
		return nil, fmt.Errorf("%w: response content type is not JSON", ErrUnavailable)
	}

	body, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBodyBytes+1))
	if err != nil {
		return nil, fmt.Errorf("%w: read response body: %v", ErrUnavailable, err)
	}
	if len(body) > maxResponseBodyBytes {
		return nil, fmt.Errorf("%w: response body exceeds %d bytes", ErrUnavailable, maxResponseBodyBytes)
	}
	return body, nil
}

func decodeJSON(body []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("response body must contain one JSON value")
	}
	return nil
}

func validateResolvedProducts(requested []int64, products map[int64]ResolvedProduct) error {
	if products == nil || len(products) != len(requested) {
		return ErrInvalidResponse
	}

	for _, id := range requested {
		resolved, ok := products[id]
		if !ok || resolved.ID != id || strings.TrimSpace(resolved.Code) == "" ||
			strings.TrimSpace(resolved.Description) == "" || resolved.Balance < 0 {
			return ErrInvalidResponse
		}
	}
	return nil
}

func validateConsumeBalances(requested []ConsumeItem, balances []consumeBalance) error {
	requestedIDs := make(map[int64]struct{}, len(requested))
	for _, item := range requested {
		requestedIDs[item.ProductID] = struct{}{}
	}
	if len(requestedIDs) == 0 || len(balances) != len(requestedIDs) {
		return ErrInvalidResponse
	}

	seen := make(map[int64]struct{}, len(balances))
	for _, balance := range balances {
		if _, requested := requestedIDs[balance.ProductID]; !requested || balance.Balance == nil || *balance.Balance < 0 {
			return ErrInvalidResponse
		}
		if _, duplicate := seen[balance.ProductID]; duplicate {
			return ErrInvalidResponse
		}
		seen[balance.ProductID] = struct{}{}
	}

	return nil
}

// GetBaseURL returns the configured Stock base URL.
func (c *Client) GetBaseURL() string {
	return c.baseURL
}

// GetTimeout returns the configured request timeout.
func (c *Client) GetTimeout() time.Duration {
	return c.defaultTimeout
}

// CloseIdleConnections closes idle Stock client connections.
func (c *Client) CloseIdleConnections() {
	c.client.CloseIdleConnections()
}
