package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	defaultMaxRetries = 10
	defaultRetryPause = 10 * time.Second
)

// Client is the PatchKit API HTTP client.
type Client struct {
	BaseURL    string
	APIKey     string
	HTTPClient *http.Client
	Debug      bool

	// Retry configuration (0 values use defaults).
	MaxRetries int
	RetryPause time.Duration

	// DebugLog is called with debug messages when Debug is true.
	DebugLog func(msg string)
}

func (c *Client) maxRetries() int {
	if c.MaxRetries > 0 {
		return c.MaxRetries
	}
	return defaultMaxRetries
}

func (c *Client) retryPause() time.Duration {
	if c.RetryPause > 0 {
		return c.RetryPause
	}
	return defaultRetryPause
}

// NewClient creates a new API client.
func NewClient(baseURL, apiKey string) *Client {
	return &Client{
		BaseURL: strings.TrimRight(baseURL, "/"),
		APIKey:  apiKey,
		HTTPClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (c *Client) debugf(format string, args ...interface{}) {
	if c.Debug && c.DebugLog != nil {
		c.DebugLog(fmt.Sprintf(format, args...))
	}
}

// buildURL constructs the full API URL with api_key query parameter.
func (c *Client) buildURL(path string) string {
	var fullURL string
	if strings.HasPrefix(path, "/") {
		fullURL = c.BaseURL + path
	} else {
		fullURL = c.BaseURL + "/" + path
	}

	if c.APIKey != "" {
		sep := "?"
		if strings.Contains(fullURL, "?") {
			sep = "&"
		}
		fullURL += sep + "api_key=" + url.QueryEscape(c.APIKey)
	}

	return fullURL
}

// Get performs a GET request with retry logic.
func (c *Client) Get(ctx context.Context, path string, result interface{}) error {
	return c.doWithRetry(ctx, "GET", path, nil, nil, result)
}

// GetWithHeaders performs a GET request with custom headers.
func (c *Client) GetWithHeaders(ctx context.Context, path string, headers map[string]string, result interface{}) error {
	return c.doWithRetry(ctx, "GET", path, headers, nil, result)
}

// Post performs a POST request with form parameters.
func (c *Client) Post(ctx context.Context, path string, params map[string]string, result interface{}) error {
	return c.doWithRetry(ctx, "POST", path, nil, params, result)
}

// Put performs a PUT request with form parameters.
func (c *Client) Put(ctx context.Context, path string, params map[string]string, result interface{}) error {
	return c.doWithRetry(ctx, "PUT", path, nil, params, result)
}

// KeyValue represents a key-value pair for multi-value form parameters.
type KeyValue struct {
	Key   string
	Value string
}

// PutMulti performs a PUT request with multi-value form parameters (supports duplicate keys).
func (c *Client) PutMulti(ctx context.Context, path string, fields []KeyValue, result interface{}) error {
	return c.doWithRetryMulti(ctx, "PUT", path, fields, result)
}

// Patch performs a PATCH request with form parameters.
func (c *Client) Patch(ctx context.Context, path string, params map[string]string, result interface{}) error {
	return c.doWithRetry(ctx, "PATCH", path, nil, params, result)
}

// PostRaw performs a POST request and returns the raw response body.
func (c *Client) PostRaw(ctx context.Context, path string, params map[string]string) ([]byte, error) {
	var body []byte
	err := c.doWithRetryRaw(ctx, "POST", path, nil, params, &body)
	return body, err
}

// GetRaw performs a GET request and returns the raw response body.
func (c *Client) GetRaw(ctx context.Context, path string) ([]byte, error) {
	var body []byte
	err := c.doWithRetryRaw(ctx, "GET", path, nil, nil, &body)
	return body, err
}

// GetStream performs a GET request and streams the response body to the writer.
func (c *Client) GetStream(ctx context.Context, rawURL string, headers map[string]string, w io.Writer, progressFn func(bytesRead int64)) error {
	req, err := http.NewRequestWithContext(ctx, "GET", rawURL, nil)
	if err != nil {
		return err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	c.debugf("GET %s", rawURL)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return &NetworkError{Err: err, URL: rawURL}
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return &APIError{
			URL:        rawURL,
			StatusCode: resp.StatusCode,
			Status:     resp.Status,
			Body:       string(body),
		}
	}

	if progressFn != nil {
		reader := &progressReader{reader: resp.Body, onProgress: progressFn}
		_, err = io.Copy(w, reader)
	} else {
		_, err = io.Copy(w, resp.Body)
	}
	return err
}

// PutRawBody performs a PUT request with a raw body (for S3 uploads).
// Uses a dedicated HTTP client with no timeout to support large uploads;
// cancellation is handled via the context.
func (c *Client) PutRawBody(ctx context.Context, rawURL string, body io.Reader, contentLength int64, headers map[string]string) error {
	req, err := http.NewRequestWithContext(ctx, "PUT", rawURL, body)
	if err != nil {
		return err
	}
	req.ContentLength = contentLength
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	c.debugf("PUT %s (size: %d)", rawURL, contentLength)

	// Use a client with no timeout for uploads; context handles cancellation.
	uploadClient := &http.Client{}
	resp, err := uploadClient.Do(req)
	if err != nil {
		return &NetworkError{Err: err, URL: rawURL}
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return &APIError{
			URL:        rawURL,
			StatusCode: resp.StatusCode,
			Status:     resp.Status,
			Body:       string(respBody),
		}
	}

	// Drain response body to ensure the connection is properly reused
	io.Copy(io.Discard, resp.Body)

	return nil
}

func (c *Client) doWithRetry(ctx context.Context, method, path string, headers map[string]string, params map[string]string, result interface{}) error {
	var lastErr error

	maxR := c.maxRetries()
	for attempt := 1; attempt <= maxR; attempt++ {
		if err := ctx.Err(); err != nil {
			return err
		}

		lastErr = c.doRequest(ctx, method, path, headers, params, result)
		if lastErr == nil {
			return nil
		}

		if !IsRetryable(lastErr, method) {
			return lastErr
		}

		c.debugf("Retryable error (attempt %d/%d): %v", attempt, maxR, lastErr)

		if attempt < maxR {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(c.retryPause()):
			}
		}
	}

	return lastErr
}

func (c *Client) doWithRetryRaw(ctx context.Context, method, path string, headers map[string]string, params map[string]string, rawBody *[]byte) error {
	var lastErr error

	for attempt := 1; attempt <= defaultMaxRetries; attempt++ {
		if err := ctx.Err(); err != nil {
			return err
		}

		lastErr = c.doRequestRaw(ctx, method, path, headers, params, rawBody)
		if lastErr == nil {
			return nil
		}

		if !IsRetryable(lastErr, method) {
			return lastErr
		}

		c.debugf("Retryable error (attempt %d/%d): %v", attempt, defaultMaxRetries, lastErr)

		if attempt < defaultMaxRetries {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(defaultRetryPause):
			}
		}
	}

	return lastErr
}

func (c *Client) doWithRetryMulti(ctx context.Context, method string, path string, fields []KeyValue, result interface{}) error {
	var lastErr error

	maxR := c.maxRetries()
	for attempt := 1; attempt <= maxR; attempt++ {
		if err := ctx.Err(); err != nil {
			return err
		}

		lastErr = c.doRequestMulti(ctx, method, path, fields, result)
		if lastErr == nil {
			return nil
		}

		if !IsRetryable(lastErr, method) {
			return lastErr
		}

		c.debugf("Retryable error (attempt %d/%d): %v", attempt, maxR, lastErr)

		if attempt < maxR {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(c.retryPause()):
			}
		}
	}

	return lastErr
}

func (c *Client) doRequestMulti(ctx context.Context, method string, path string, fields []KeyValue, result interface{}) error {
	fullURL := c.buildURL(path)

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	for _, f := range fields {
		if err := writer.WriteField(f.Key, f.Value); err != nil {
			return fmt.Errorf("failed to write form field %s: %w", f.Key, err)
		}
	}
	if err := writer.Close(); err != nil {
		return fmt.Errorf("failed to close multipart writer: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, method, fullURL, &buf)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	c.debugf("%s %s", method, fullURL)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return &NetworkError{Err: err, URL: fullURL}
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return &NetworkError{Err: fmt.Errorf("failed to read response body: %w", err), URL: fullURL}
	}

	c.debugf("Response: %d %s", resp.StatusCode, string(respBody))

	if resp.StatusCode >= 400 {
		apiErr := &APIError{
			URL:        fullURL,
			StatusCode: resp.StatusCode,
			Status:     resp.Status,
			Body:       string(respBody),
		}
		var errResp struct {
			Error   string `json:"error"`
			Message string `json:"message"`
		}
		if json.Unmarshal(respBody, &errResp) == nil {
			if errResp.Message != "" {
				apiErr.Message = errResp.Message
			} else if errResp.Error != "" {
				apiErr.Message = errResp.Error
			}
		}
		return apiErr
	}

	if result != nil && len(respBody) > 0 {
		if err := json.Unmarshal(respBody, result); err != nil {
			return fmt.Errorf("failed to decode response: %w (body: %s)", err, string(respBody))
		}
	}

	return nil
}

func (c *Client) doRequest(ctx context.Context, method, path string, headers map[string]string, params map[string]string, result interface{}) error {
	var rawBody []byte
	if err := c.doRequestRaw(ctx, method, path, headers, params, &rawBody); err != nil {
		return err
	}

	if result != nil && len(rawBody) > 0 {
		if err := json.Unmarshal(rawBody, result); err != nil {
			return fmt.Errorf("failed to decode response: %w (body: %s)", err, string(rawBody))
		}
	}

	return nil
}

func (c *Client) doRequestRaw(ctx context.Context, method, path string, headers map[string]string, params map[string]string, rawBody *[]byte) error {
	fullURL := c.buildURL(path)

	var body io.Reader
	var contentType string

	if params != nil && len(params) > 0 {
		var buf bytes.Buffer
		writer := multipart.NewWriter(&buf)
		for k, v := range params {
			if err := writer.WriteField(k, v); err != nil {
				return fmt.Errorf("failed to write form field %s: %w", k, err)
			}
		}
		if err := writer.Close(); err != nil {
			return fmt.Errorf("failed to close multipart writer: %w", err)
		}
		body = &buf
		contentType = writer.FormDataContentType()
	}

	req, err := http.NewRequestWithContext(ctx, method, fullURL, body)
	if err != nil {
		return err
	}

	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}

	for k, v := range headers {
		req.Header.Set(k, v)
	}

	c.debugf("%s %s", method, fullURL)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return &NetworkError{Err: err, URL: fullURL}
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return &NetworkError{Err: fmt.Errorf("failed to read response body: %w", err), URL: fullURL}
	}

	c.debugf("Response: %d %s", resp.StatusCode, string(respBody))

	if resp.StatusCode >= 400 {
		apiErr := &APIError{
			URL:        fullURL,
			StatusCode: resp.StatusCode,
			Status:     resp.Status,
			Body:       string(respBody),
		}
		// Try to parse error message from JSON body
		var errResp struct {
			Error   string `json:"error"`
			Message string `json:"message"`
		}
		if json.Unmarshal(respBody, &errResp) == nil {
			if errResp.Message != "" {
				apiErr.Message = errResp.Message
			} else if errResp.Error != "" {
				apiErr.Message = errResp.Error
			}
		}
		return apiErr
	}

	if rawBody != nil {
		*rawBody = respBody
	}

	return nil
}

// progressReader wraps an io.Reader and reports progress.
type progressReader struct {
	reader     io.Reader
	onProgress func(bytesRead int64)
	total      int64
}

func (r *progressReader) Read(p []byte) (n int, err error) {
	n, err = r.reader.Read(p)
	r.total += int64(n)
	if r.onProgress != nil {
		r.onProgress(r.total)
	}
	return
}
