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
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	defaultBaseURL = "https://api.emailverify.ai/v1"
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

	req.Header.Set("EMAILVERIFY-API-KEY", c.apiKey)
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

	case http.StatusForbidden:
		if code == "INSUFFICIENT_CREDITS" {
			return NewInsufficientCreditsError(message)
		}
		return &EmailVerifyError{Code: code, Message: message, StatusCode: 403}

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

// Verify verifies a single email address.
func (c *Client) Verify(ctx context.Context, email string, opts *VerifyOptions) (*VerifyResponse, error) {
	payload := map[string]interface{}{
		"email":      email,
		"smtp_check": true,
	}

	if opts != nil {
		payload["smtp_check"] = opts.SMTPCheck
		if opts.Timeout > 0 {
			payload["timeout"] = opts.Timeout
		}
	}

	var result VerifyResponse
	if err := c.request(ctx, http.MethodPost, "/verify", payload, &result); err != nil {
		return nil, err
	}

	return &result, nil
}

// VerifyBulk submits a bulk verification job.
func (c *Client) VerifyBulk(ctx context.Context, emails []string, opts *BulkVerifyOptions) (*BulkJobResponse, error) {
	if len(emails) > 10000 {
		return nil, NewValidationError("Maximum 10,000 emails per bulk job", "")
	}

	payload := map[string]interface{}{
		"emails":     emails,
		"smtp_check": true,
	}

	if opts != nil {
		payload["smtp_check"] = opts.SMTPCheck
		if opts.WebhookURL != "" {
			payload["webhook_url"] = opts.WebhookURL
		}
	}

	var result BulkJobResponse
	if err := c.request(ctx, http.MethodPost, "/verify/bulk", payload, &result); err != nil {
		return nil, err
	}

	return &result, nil
}

// GetBulkJobStatus gets the status of a bulk verification job.
func (c *Client) GetBulkJobStatus(ctx context.Context, jobID string) (*BulkJobResponse, error) {
	var result BulkJobResponse
	if err := c.request(ctx, http.MethodGet, "/verify/bulk/"+jobID, nil, &result); err != nil {
		return nil, err
	}

	return &result, nil
}

// GetBulkJobResults gets the results of a completed bulk verification job.
func (c *Client) GetBulkJobResults(ctx context.Context, jobID string, opts *BulkResultsOptions) (*BulkResultsResponse, error) {
	path := "/verify/bulk/" + jobID + "/results"

	if opts != nil {
		params := url.Values{}
		if opts.Limit > 0 {
			params.Set("limit", strconv.Itoa(opts.Limit))
		}
		if opts.Offset > 0 {
			params.Set("offset", strconv.Itoa(opts.Offset))
		}
		if opts.Status != "" {
			params.Set("status", string(opts.Status))
		}
		if len(params) > 0 {
			path += "?" + params.Encode()
		}
	}

	var result BulkResultsResponse
	if err := c.request(ctx, http.MethodGet, path, nil, &result); err != nil {
		return nil, err
	}

	return &result, nil
}

// WaitForBulkJobCompletion polls for bulk job completion.
func (c *Client) WaitForBulkJobCompletion(ctx context.Context, jobID string, pollInterval, maxWait time.Duration) (*BulkJobResponse, error) {
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
		status, err := c.GetBulkJobStatus(ctx, jobID)
		if err != nil {
			return nil, err
		}

		if status.Status == JobStatusCompleted || status.Status == JobStatusFailed {
			return status, nil
		}

		if time.Now().After(deadline) {
			return nil, NewTimeoutError(fmt.Sprintf("Bulk job %s did not complete within %v", jobID, maxWait))
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
	var result CreditsResponse
	if err := c.request(ctx, http.MethodGet, "/credits", nil, &result); err != nil {
		return nil, err
	}

	return &result, nil
}

// CreateWebhook creates a new webhook.
func (c *Client) CreateWebhook(ctx context.Context, config WebhookConfig) (*Webhook, error) {
	var result Webhook
	if err := c.request(ctx, http.MethodPost, "/webhooks", config, &result); err != nil {
		return nil, err
	}

	return &result, nil
}

// ListWebhooks lists all webhooks.
func (c *Client) ListWebhooks(ctx context.Context) ([]Webhook, error) {
	var result []Webhook
	if err := c.request(ctx, http.MethodGet, "/webhooks", nil, &result); err != nil {
		return nil, err
	}

	return result, nil
}

// DeleteWebhook deletes a webhook.
func (c *Client) DeleteWebhook(ctx context.Context, webhookID string) error {
	return c.request(ctx, http.MethodDelete, "/webhooks/"+webhookID, nil, nil)
}

// VerifyWebhookSignature verifies a webhook signature.
func VerifyWebhookSignature(payload, signature, secret string) bool {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(payload))
	expectedSig := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(signature), []byte(expectedSig))
}
