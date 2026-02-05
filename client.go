package emailverify

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	defaultBaseURL = "https://api.emailverify.ai"
	defaultTimeout = 30 * time.Second
	defaultRetries = 3
	userAgent      = "emailverify-go/1.0.0"
)

// Config contains client configuration options.
type Config struct {
	APIKey  string
	BaseURL string
	Timeout time.Duration
	Retries int
}

// Client is the EmailVerify API client.
type Client struct {
	apiKey     string
	baseURL    string
	timeout    time.Duration
	retries    int
	httpClient *http.Client
}

// NewClient creates a new EmailVerify client.
func NewClient(config Config) (*Client, error) {
	if config.APIKey == "" {
		return nil, NewAuthenticationError("API key is required")
	}

	baseURL := config.BaseURL
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	baseURL = strings.TrimSuffix(baseURL, "/")

	timeout := config.Timeout
	if timeout == 0 {
		timeout = defaultTimeout
	}

	retries := config.Retries
	if retries == 0 {
		retries = defaultRetries
	}

	return &Client{
		apiKey:  config.APIKey,
		baseURL: baseURL,
		timeout: timeout,
		retries: retries,
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}, nil
}

func (c *Client) request(ctx context.Context, method, path string, body interface{}, result interface{}) error {
	return c.requestWithRetry(ctx, method, path, body, result, 1)
}

func (c *Client) requestWithRetry(ctx context.Context, method, path string, body interface{}, result interface{}, attempt int) error {
	reqURL := c.baseURL + path

	var bodyReader io.Reader
	if body != nil {
		jsonBody, err := json.Marshal(body)
		if err != nil {
			return &EmailVerifyError{Code: "MARSHAL_ERROR", Message: err.Error()}
		}
		bodyReader = bytes.NewReader(jsonBody)
	}

	req, err := http.NewRequestWithContext(ctx, method, reqURL, bodyReader)
	if err != nil {
		return &EmailVerifyError{Code: "REQUEST_ERROR", Message: err.Error()}
	}

	req.Header.Set("EV-API-KEY", c.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", userAgent)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return NewTimeoutError(fmt.Sprintf("Request timed out after %v", c.timeout))
		}
		return &EmailVerifyError{Code: "NETWORK_ERROR", Message: err.Error()}
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return &EmailVerifyError{Code: "READ_ERROR", Message: err.Error()}
	}

	if resp.StatusCode == http.StatusNoContent {
		return nil
	}

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		if result != nil && len(respBody) > 0 {
			if err := json.Unmarshal(respBody, result); err != nil {
				return &EmailVerifyError{Code: "UNMARSHAL_ERROR", Message: err.Error()}
			}
		}
		return nil
	}

	return c.handleErrorResponse(ctx, resp, respBody, method, path, body, result, attempt)
}

// requestNoAuth makes a request without authentication (for health check).
func (c *Client) requestNoAuth(ctx context.Context, method, path string, result interface{}) error {
	reqURL := c.baseURL + path

	req, err := http.NewRequestWithContext(ctx, method, reqURL, nil)
	if err != nil {
		return &EmailVerifyError{Code: "REQUEST_ERROR", Message: err.Error()}
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", userAgent)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return NewTimeoutError(fmt.Sprintf("Request timed out after %v", c.timeout))
		}
		return &EmailVerifyError{Code: "NETWORK_ERROR", Message: err.Error()}
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return &EmailVerifyError{Code: "READ_ERROR", Message: err.Error()}
	}

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		if result != nil && len(respBody) > 0 {
			if err := json.Unmarshal(respBody, result); err != nil {
				return &EmailVerifyError{Code: "UNMARSHAL_ERROR", Message: err.Error()}
			}
		}
		return nil
	}

	return &EmailVerifyError{Code: "REQUEST_FAILED", Message: resp.Status, StatusCode: resp.StatusCode}
}

func (c *Client) handleErrorResponse(ctx context.Context, resp *http.Response, body []byte, method, path string, reqBody interface{}, result interface{}, attempt int) error {
	var errResp struct {
		Error APIError `json:"error"`
	}
	json.Unmarshal(body, &errResp)

	message := errResp.Error.Message
	if message == "" {
		message = resp.Status
	}
	code := errResp.Error.Code
	if code == "" {
		code = "UNKNOWN_ERROR"
	}

	switch resp.StatusCode {
	case http.StatusUnauthorized:
		return NewAuthenticationError(message)

	case http.StatusPaymentRequired:
		return NewInsufficientCreditsError(message)

	case http.StatusNotFound:
		return NewNotFoundError(message)

	case http.StatusTooManyRequests:
		retryAfter, _ := strconv.Atoi(resp.Header.Get("Retry-After"))
		if attempt < c.retries {
			waitTime := retryAfter
			if waitTime == 0 {
				waitTime = 1 << attempt
			}
			time.Sleep(time.Duration(waitTime) * time.Second)
			return c.requestWithRetry(ctx, method, path, reqBody, result, attempt+1)
		}
		return NewRateLimitError(message, retryAfter)

	case http.StatusBadRequest:
		return NewValidationError(message, errResp.Error.Details)

	case http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnavailable:
		if attempt < c.retries {
			time.Sleep(time.Duration(1<<attempt) * time.Second)
			return c.requestWithRetry(ctx, method, path, reqBody, result, attempt+1)
		}
		return &EmailVerifyError{Code: code, Message: message, StatusCode: resp.StatusCode}

	default:
		return &EmailVerifyError{Code: code, Message: message, StatusCode: resp.StatusCode}
	}
}

// Health checks the API health status (no authentication required).
func (c *Client) Health(ctx context.Context) (*HealthResponse, error) {
	var result HealthResponse
	if err := c.requestNoAuth(ctx, http.MethodGet, "/health", &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// Verify verifies a single email address.
func (c *Client) Verify(ctx context.Context, email string, opts *VerifyOptions) (*VerifyResponse, error) {
	payload := map[string]interface{}{
		"email":      email,
		"check_smtp": true,
	}

	if opts != nil {
		payload["check_smtp"] = opts.CheckSMTP
	}

	var apiResp struct {
		Success bool           `json:"success"`
		Code    string         `json:"code"`
		Message string         `json:"message"`
		Data    VerifyResponse `json:"data"`
	}
	if err := c.request(ctx, http.MethodPost, "/v1/verify/single", payload, &apiResp); err != nil {
		return nil, err
	}

	return &apiResp.Data, nil
}

// VerifyBatch verifies multiple email addresses synchronously (max 50 emails).
func (c *Client) VerifyBatch(ctx context.Context, emails []string, opts *BatchVerifyOptions) (*BatchVerifyResponse, error) {
	if len(emails) > 50 {
		return nil, NewValidationError("Maximum 50 emails per batch request", "")
	}

	payload := map[string]interface{}{
		"emails":     emails,
		"check_smtp": true,
	}

	if opts != nil {
		payload["check_smtp"] = opts.CheckSMTP
	}

	var apiResp struct {
		Success bool                `json:"success"`
		Code    string              `json:"code"`
		Message string              `json:"message"`
		Data    BatchVerifyResponse `json:"data"`
	}
	if err := c.request(ctx, http.MethodPost, "/v1/verify/bulk", payload, &apiResp); err != nil {
		return nil, err
	}

	return &apiResp.Data, nil
}

// UploadFile uploads a file for asynchronous verification.
func (c *Client) UploadFile(ctx context.Context, filePath string, opts *FileUploadOptions) (*FileUploadResponse, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, &EmailVerifyError{Code: "FILE_ERROR", Message: err.Error()}
	}
	defer file.Close()

	return c.UploadFileReader(ctx, file, filepath.Base(filePath), opts)
}

// UploadFileReader uploads a file from an io.Reader for asynchronous verification.
func (c *Client) UploadFileReader(ctx context.Context, reader io.Reader, filename string, opts *FileUploadOptions) (*FileUploadResponse, error) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		return nil, &EmailVerifyError{Code: "MULTIPART_ERROR", Message: err.Error()}
	}

	if _, err := io.Copy(part, reader); err != nil {
		return nil, &EmailVerifyError{Code: "COPY_ERROR", Message: err.Error()}
	}

	// Add optional form fields
	checkSMTP := true
	preserveOriginal := true
	emailColumn := ""

	if opts != nil {
		checkSMTP = opts.CheckSMTP
		preserveOriginal = opts.PreserveOriginal
		emailColumn = opts.EmailColumn
	}

	writer.WriteField("check_smtp", strconv.FormatBool(checkSMTP))
	writer.WriteField("preserve_original", strconv.FormatBool(preserveOriginal))
	if emailColumn != "" {
		writer.WriteField("email_column", emailColumn)
	}

	if err := writer.Close(); err != nil {
		return nil, &EmailVerifyError{Code: "MULTIPART_ERROR", Message: err.Error()}
	}

	reqURL := c.baseURL + "/v1/verify/file"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, &body)
	if err != nil {
		return nil, &EmailVerifyError{Code: "REQUEST_ERROR", Message: err.Error()}
	}

	req.Header.Set("EV-API-KEY", c.apiKey)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("User-Agent", userAgent)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, NewTimeoutError(fmt.Sprintf("Request timed out after %v", c.timeout))
		}
		return nil, &EmailVerifyError{Code: "NETWORK_ERROR", Message: err.Error()}
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &EmailVerifyError{Code: "READ_ERROR", Message: err.Error()}
	}

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		var apiResp struct {
			Success bool               `json:"success"`
			Code    string             `json:"code"`
			Message string             `json:"message"`
			Data    FileUploadResponse `json:"data"`
		}
		if err := json.Unmarshal(respBody, &apiResp); err != nil {
			return nil, &EmailVerifyError{Code: "UNMARSHAL_ERROR", Message: err.Error()}
		}
		return &apiResp.Data, nil
	}

	// Handle error response
	var errResp struct {
		Error APIError `json:"error"`
	}
	json.Unmarshal(respBody, &errResp)

	message := errResp.Error.Message
	if message == "" {
		message = resp.Status
	}

	switch resp.StatusCode {
	case http.StatusUnauthorized:
		return nil, NewAuthenticationError(message)
	case http.StatusPaymentRequired:
		return nil, NewInsufficientCreditsError(message)
	case http.StatusBadRequest:
		return nil, NewValidationError(message, errResp.Error.Details)
	default:
		return nil, &EmailVerifyError{Code: errResp.Error.Code, Message: message, StatusCode: resp.StatusCode}
	}
}

// GetFileJobStatus gets the status of a file verification job.
func (c *Client) GetFileJobStatus(ctx context.Context, jobID string, opts *FileJobStatusOptions) (*FileJobResponse, error) {
	path := "/v1/verify/file/" + jobID

	if opts != nil && opts.Timeout > 0 {
		timeout := opts.Timeout
		if timeout > 300 {
			timeout = 300
		}
		path += "?timeout=" + strconv.Itoa(timeout)
	}

	var apiResp struct {
		Success bool            `json:"success"`
		Code    string          `json:"code"`
		Message string          `json:"message"`
		Data    FileJobResponse `json:"data"`
	}
	if err := c.request(ctx, http.MethodGet, path, nil, &apiResp); err != nil {
		return nil, err
	}

	return &apiResp.Data, nil
}

// GetFileJobResults downloads the results of a completed file verification job.
// Returns the raw CSV data as bytes. Use FileResultsOptions to filter results.
func (c *Client) GetFileJobResults(ctx context.Context, jobID string, opts *FileResultsOptions) ([]byte, error) {
	path := "/v1/verify/file/" + jobID + "/results"

	if opts != nil {
		params := url.Values{}
		if opts.Valid != nil {
			params.Set("valid", strconv.FormatBool(*opts.Valid))
		}
		if opts.Invalid != nil {
			params.Set("invalid", strconv.FormatBool(*opts.Invalid))
		}
		if opts.Catchall != nil {
			params.Set("catchall", strconv.FormatBool(*opts.Catchall))
		}
		if opts.Role != nil {
			params.Set("role", strconv.FormatBool(*opts.Role))
		}
		if opts.Unknown != nil {
			params.Set("unknown", strconv.FormatBool(*opts.Unknown))
		}
		if opts.Disposable != nil {
			params.Set("disposable", strconv.FormatBool(*opts.Disposable))
		}
		if opts.Risky != nil {
			params.Set("risky", strconv.FormatBool(*opts.Risky))
		}
		if len(params) > 0 {
			path += "?" + params.Encode()
		}
	}

	reqURL := c.baseURL + path
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, &EmailVerifyError{Code: "REQUEST_ERROR", Message: err.Error()}
	}

	req.Header.Set("EV-API-KEY", c.apiKey)
	req.Header.Set("User-Agent", userAgent)

	// Create a client that doesn't follow redirects automatically
	client := &http.Client{
		Timeout: c.timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	resp, err := client.Do(req)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, NewTimeoutError(fmt.Sprintf("Request timed out after %v", c.timeout))
		}
		return nil, &EmailVerifyError{Code: "NETWORK_ERROR", Message: err.Error()}
	}
	defer resp.Body.Close()

	// Handle redirect (307) - follow it to get the actual file
	if resp.StatusCode == http.StatusTemporaryRedirect {
		location := resp.Header.Get("Location")
		if location != "" {
			redirectReq, err := http.NewRequestWithContext(ctx, http.MethodGet, location, nil)
			if err != nil {
				return nil, &EmailVerifyError{Code: "REQUEST_ERROR", Message: err.Error()}
			}
			redirectResp, err := c.httpClient.Do(redirectReq)
			if err != nil {
				return nil, &EmailVerifyError{Code: "NETWORK_ERROR", Message: err.Error()}
			}
			defer redirectResp.Body.Close()
			return io.ReadAll(redirectResp.Body)
		}
	}

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return io.ReadAll(resp.Body)
	}

	// Handle error
	respBody, _ := io.ReadAll(resp.Body)
	var errResp struct {
		Error APIError `json:"error"`
	}
	json.Unmarshal(respBody, &errResp)

	message := errResp.Error.Message
	if message == "" {
		message = resp.Status
	}

	switch resp.StatusCode {
	case http.StatusUnauthorized:
		return nil, NewAuthenticationError(message)
	case http.StatusNotFound:
		return nil, NewNotFoundError(message)
	default:
		return nil, &EmailVerifyError{Code: errResp.Error.Code, Message: message, StatusCode: resp.StatusCode}
	}
}

// WaitForFileJobCompletion polls for file job completion with optional long-polling.
func (c *Client) WaitForFileJobCompletion(ctx context.Context, jobID string, pollInterval, maxWait time.Duration) (*FileJobResponse, error) {
	if pollInterval == 0 {
		pollInterval = 5 * time.Second
	}
	if maxWait == 0 {
		maxWait = 10 * time.Minute
	}

	deadline := time.Now().Add(maxWait)
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		status, err := c.GetFileJobStatus(ctx, jobID, nil)
		if err != nil {
			return nil, err
		}

		if status.Status == JobStatusCompleted || status.Status == JobStatusFailed {
			return status, nil
		}

		if time.Now().After(deadline) {
			return nil, NewTimeoutError(fmt.Sprintf("File job %s did not complete within %v", jobID, maxWait))
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
			continue
		}
	}
}

// GetCredits gets current credit balance.
func (c *Client) GetCredits(ctx context.Context) (*CreditsResponse, error) {
	var apiResp struct {
		Success bool            `json:"success"`
		Code    string          `json:"code"`
		Message string          `json:"message"`
		Data    CreditsResponse `json:"data"`
	}
	if err := c.request(ctx, http.MethodGet, "/v1/credits", nil, &apiResp); err != nil {
		return nil, err
	}

	return &apiResp.Data, nil
}

// CreateWebhook creates a new webhook.
func (c *Client) CreateWebhook(ctx context.Context, config WebhookConfig) (*Webhook, error) {
	var apiResp struct {
		Success bool    `json:"success"`
		Code    string  `json:"code"`
		Message string  `json:"message"`
		Data    Webhook `json:"data"`
	}
	if err := c.request(ctx, http.MethodPost, "/v1/webhooks", config, &apiResp); err != nil {
		return nil, err
	}

	return &apiResp.Data, nil
}

// ListWebhooks lists all webhooks.
func (c *Client) ListWebhooks(ctx context.Context) (*WebhooksListResponse, error) {
	var apiResp struct {
		Success bool                 `json:"success"`
		Code    string               `json:"code"`
		Message string               `json:"message"`
		Data    WebhooksListResponse `json:"data"`
	}
	if err := c.request(ctx, http.MethodGet, "/v1/webhooks", nil, &apiResp); err != nil {
		return nil, err
	}

	return &apiResp.Data, nil
}

// DeleteWebhook deletes a webhook.
func (c *Client) DeleteWebhook(ctx context.Context, webhookID string) error {
	return c.request(ctx, http.MethodDelete, "/v1/webhooks/"+webhookID, nil, nil)
}

// VerifyWebhookSignature verifies a webhook signature.
func VerifyWebhookSignature(payload, signature, secret string) bool {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(payload))
	expectedSig := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(signature), []byte(expectedSig))
}

// Deprecated: VerifyBulk is deprecated. Use VerifyBatch for synchronous batch verification
// or UploadFile for asynchronous file-based verification.
func (c *Client) VerifyBulk(ctx context.Context, emails []string, opts *BatchVerifyOptions) (*BatchVerifyResponse, error) {
	return c.VerifyBatch(ctx, emails, opts)
}
