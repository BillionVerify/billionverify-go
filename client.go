package billionverify

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
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
	defaultBaseURL = "https://api.billionverify.com"
	defaultTimeout = 30 * time.Second
	defaultRetries = 3
	userAgent      = "billionverify-go/1.1.0"

	// DefaultWebhookTolerance is the largest accepted age of a webhook delivery.
	DefaultWebhookTolerance = 300 * time.Second

	maxRedirects = 5
)

// Config contains client configuration options.
type Config struct {
	APIKey  string
	BaseURL string // defaults to https://api.billionverify.com; a trailing /v1 is accepted
	Timeout time.Duration
	// Retries is the total number of attempts for 429, 500, 502 and 503 responses (default 3).
	Retries int
}

// Client is the BillionVerify API client.
type Client struct {
	apiKey     string
	baseURL    string
	timeout    time.Duration
	retries    int
	httpClient *http.Client
	// sleep waits between attempts; replaced in tests.
	sleep func(ctx context.Context, d time.Duration) error
}

// NewClient creates a new BillionVerify client.
func NewClient(config Config) (*Client, error) {
	if config.APIKey == "" {
		return nil, NewAuthenticationError("API key is required")
	}

	baseURL := config.BaseURL
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	baseURL = strings.TrimRight(baseURL, "/")
	baseURL = strings.TrimSuffix(baseURL, "/v1")

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
			// Redirects are followed by the client itself, so the API key never
			// goes to another host.
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
		sleep: sleepContext,
	}, nil
}

func sleepContext(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// ---------------------------------------------------------------- plumbing

type requestSpec struct {
	method string
	url    string
	auth   bool
	// body builds the request body and its content type for each attempt.
	body    func() (io.Reader, string, error)
	timeout time.Duration
}

type rawResponse struct {
	status int
	header http.Header
	body   []byte
}

func (c *Client) endpoint(path string, query url.Values) string {
	u := c.baseURL + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	return u
}

func (c *Client) jsonBody(v interface{}) func() (io.Reader, string, error) {
	return func() (io.Reader, string, error) {
		b, err := json.Marshal(v)
		if err != nil {
			return nil, "", &BillionVerifyError{Code: "MARSHAL_ERROR", Message: err.Error()}
		}
		return bytes.NewReader(b), "application/json", nil
	}
}

// send runs a request, retrying 429, 500, 502 and 503 up to c.retries attempts in total.
func (c *Client) send(ctx context.Context, s requestSpec) (*rawResponse, error) {
	for attempt := 1; ; attempt++ {
		raw, err := c.once(ctx, s)
		if err != nil {
			return nil, err
		}
		if (raw.status == 429 || raw.status == 500 || raw.status == 502 || raw.status == 503) && attempt < c.retries {
			wait := time.Duration(1<<attempt) * time.Second
			if ra, _ := strconv.Atoi(raw.header.Get("Retry-After")); raw.status == 429 && ra > 0 {
				wait = time.Duration(ra) * time.Second
			}
			if err := c.sleep(ctx, wait); err != nil {
				return nil, err
			}
			continue
		}
		return raw, nil
	}
}

func (c *Client) once(ctx context.Context, s requestSpec) (*rawResponse, error) {
	timeout := s.timeout
	if timeout == 0 {
		timeout = c.timeout
	}
	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var body io.Reader
	contentType := ""
	if s.body != nil {
		var err error
		if body, contentType, err = s.body(); err != nil {
			return nil, err
		}
	}
	req, err := http.NewRequestWithContext(reqCtx, s.method, s.url, body)
	if err != nil {
		return nil, &BillionVerifyError{Code: "REQUEST_ERROR", Message: sanitizeError(err)}
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/json")
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if s.auth {
		req.Header.Set("BV-API-KEY", c.apiKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, c.transportError(ctx, reqCtx, err, timeout)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, c.transportError(ctx, reqCtx, err, timeout)
	}
	return &rawResponse{status: resp.StatusCode, header: resp.Header, body: data}, nil
}

func (c *Client) transportError(parent, reqCtx context.Context, err error, timeout time.Duration) error {
	if parent.Err() != nil {
		return parent.Err()
	}
	if errors.Is(reqCtx.Err(), context.DeadlineExceeded) {
		return NewTimeoutError(fmt.Sprintf("Request timed out after %v", timeout))
	}
	return &BillionVerifyError{Code: "NETWORK_ERROR", Message: "Network error: " + sanitizeError(err)}
}

// sanitizeError renders an error without the request URL, whose query may carry a
// pre-signed signature.
func sanitizeError(err error) string {
	var ue *url.Error
	for errors.As(err, &ue) {
		err = ue.Err
	}
	return err.Error()
}

// apiError maps an error response to the matching error type.
func apiError(raw *rawResponse) error {
	var env struct {
		Message string `json:"message"`
		Error   struct {
			Code        string          `json:"code"`
			Message     string          `json:"message"`
			Description string          `json:"description"`
			Details     json.RawMessage `json:"details"`
		} `json:"error"`
	}
	_ = json.Unmarshal(raw.body, &env)

	message := env.Error.Message
	if message == "" {
		message = env.Message
	}
	if message == "" {
		message = http.StatusText(raw.status)
		if message == "" {
			message = "HTTP " + strconv.Itoa(raw.status)
		}
	}
	code := env.Error.Code
	if code == "" {
		code = "UNKNOWN_ERROR"
	}
	details := env.Error.Description
	if len(env.Error.Details) > 0 && string(env.Error.Details) != "null" {
		var s string
		if json.Unmarshal(env.Error.Details, &s) == nil {
			details = s
		} else {
			details = string(env.Error.Details)
		}
	}

	switch raw.status {
	case http.StatusBadRequest:
		return NewValidationError(message, details)
	case http.StatusUnauthorized:
		return NewAuthenticationError(message)
	case http.StatusPaymentRequired:
		return NewInsufficientCreditsError(message)
	case http.StatusNotFound:
		return NewNotFoundError(message)
	case http.StatusTooManyRequests:
		retryAfter, _ := strconv.Atoi(raw.header.Get("Retry-After"))
		return NewRateLimitError(message, retryAfter)
	default:
		return &BillionVerifyError{Code: code, Message: message, StatusCode: raw.status, Details: details}
	}
}

// call runs a request and decodes the envelope's data (or the body itself when
// the endpoint has no envelope) into out.
func (c *Client) call(ctx context.Context, s requestSpec, out interface{}) error {
	raw, err := c.send(ctx, s)
	if err != nil {
		return err
	}
	if raw.status < 200 || raw.status >= 300 {
		return apiError(raw)
	}
	if out == nil || raw.status == http.StatusNoContent || len(bytes.TrimSpace(raw.body)) == 0 {
		return nil
	}
	var env struct {
		Success *bool           `json:"success"`
		Data    json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw.body, &env); err != nil {
		return &BillionVerifyError{Code: "UNMARSHAL_ERROR", Message: err.Error(), StatusCode: raw.status}
	}
	payload := raw.body
	if env.Success != nil || len(env.Data) > 0 {
		payload = env.Data
	}
	if len(payload) == 0 || string(payload) == "null" {
		return nil
	}
	if err := json.Unmarshal(payload, out); err != nil {
		return &BillionVerifyError{Code: "UNMARSHAL_ERROR", Message: err.Error(), StatusCode: raw.status}
	}
	return nil
}

func (c *Client) get(ctx context.Context, path string, query url.Values, out interface{}) error {
	return c.call(ctx, requestSpec{method: http.MethodGet, url: c.endpoint(path, query), auth: true}, out)
}

func (c *Client) postJSON(ctx context.Context, path string, body interface{}, out interface{}) error {
	return c.call(ctx, requestSpec{method: http.MethodPost, url: c.endpoint(path, nil), auth: true, body: c.jsonBody(body)}, out)
}

// download fetches a body, following redirects itself and sending the API key
// only to the API's own origin.
func (c *Client) download(ctx context.Context, path string, query url.Values) ([]byte, error) {
	first, err := url.Parse(c.endpoint(path, query))
	if err != nil {
		return nil, &BillionVerifyError{Code: "REQUEST_ERROR", Message: sanitizeError(err)}
	}
	target := first
	auth := true
	for hop := 0; hop <= maxRedirects; hop++ {
		raw, err := c.send(ctx, requestSpec{method: http.MethodGet, url: target.String(), auth: auth})
		if err != nil {
			return nil, err
		}
		if raw.status >= 200 && raw.status < 300 {
			return raw.body, nil
		}
		switch raw.status {
		case 301, 302, 303, 307, 308:
			if loc := raw.header.Get("Location"); loc != "" {
				next, err := target.Parse(loc)
				if err != nil {
					return nil, &BillionVerifyError{Code: "REQUEST_ERROR", Message: "Invalid redirect location"}
				}
				auth = auth && next.Scheme == first.Scheme && next.Host == first.Host
				target = next
				continue
			}
		}
		return nil, apiError(raw)
	}
	return nil, &BillionVerifyError{Code: "TOO_MANY_REDIRECTS", Message: "Too many redirects"}
}

func (c *Client) uploadMultipart(ctx context.Context, path string, reader io.Reader, filename string, fields [][2]string, out interface{}) error {
	// Read once so every retry can send the same content.
	content, err := io.ReadAll(reader)
	if err != nil {
		return &BillionVerifyError{Code: "COPY_ERROR", Message: err.Error()}
	}
	body := func() (io.Reader, string, error) {
		var buf bytes.Buffer
		w := multipart.NewWriter(&buf)
		part, err := w.CreateFormFile("file", filename)
		if err != nil {
			return nil, "", &BillionVerifyError{Code: "MULTIPART_ERROR", Message: err.Error()}
		}
		if _, err := part.Write(content); err != nil {
			return nil, "", &BillionVerifyError{Code: "COPY_ERROR", Message: err.Error()}
		}
		for _, f := range fields {
			if err := w.WriteField(f[0], f[1]); err != nil {
				return nil, "", &BillionVerifyError{Code: "MULTIPART_ERROR", Message: err.Error()}
			}
		}
		if err := w.Close(); err != nil {
			return nil, "", &BillionVerifyError{Code: "MULTIPART_ERROR", Message: err.Error()}
		}
		return &buf, w.FormDataContentType(), nil
	}
	return c.call(ctx, requestSpec{method: http.MethodPost, url: c.endpoint(path, nil), auth: true, body: body}, out)
}

func pathID(id string) string { return url.PathEscape(id) }

// ------------------------------------------------------------------ health

// Health checks the API health status (no authentication required).
func (c *Client) Health(ctx context.Context) (*HealthResponse, error) {
	var result HealthResponse
	if err := c.call(ctx, requestSpec{method: http.MethodGet, url: c.endpoint("/health", nil)}, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// ------------------------------------------------------------ verification

// Verify verifies a single email address.
func (c *Client) Verify(ctx context.Context, email string, opts *VerifyOptions) (*VerifyResponse, error) {
	payload := map[string]interface{}{"email": email, "check_smtp": true}
	if opts != nil {
		payload["check_smtp"] = opts.CheckSMTP
		if opts.ServerDefaultSMTP {
			delete(payload, "check_smtp")
		}
		if opts.ForceRefresh {
			payload["force_refresh"] = true
		}
		if opts.IncludeDomainReputation {
			payload["include_domain_reputation"] = true
		}
	}

	var result VerifyResponse
	if err := c.postJSON(ctx, "/v1/verify/single", payload, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// VerifyBatch verifies multiple email addresses synchronously (max 50 emails).
func (c *Client) VerifyBatch(ctx context.Context, emails []string, opts *BatchVerifyOptions) (*BatchVerifyResponse, error) {
	if len(emails) > 50 {
		return nil, NewValidationError("Maximum 50 emails per batch request", "")
	}

	payload := map[string]interface{}{"emails": emails, "check_smtp": true}
	if opts != nil {
		payload["check_smtp"] = opts.CheckSMTP
		if opts.ServerDefaultSMTP {
			delete(payload, "check_smtp")
		}
		if opts.ForceRefresh {
			payload["force_refresh"] = true
		}
		if opts.IncludeDomainReputation {
			payload["include_domain_reputation"] = true
		}
	}

	var result BatchVerifyResponse
	if err := c.postJSON(ctx, "/v1/verify/bulk", payload, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// CheckDisposable looks an address up in the free disposable-domain table (POST).
func (c *Client) CheckDisposable(ctx context.Context, email string) (*DisposableCheck, error) {
	var result DisposableCheck
	if err := c.postJSON(ctx, "/v1/verify/disposable", map[string]string{"email": email}, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// CheckDisposableViaQuery is CheckDisposable with the address in the query string (GET).
func (c *Client) CheckDisposableViaQuery(ctx context.Context, email string) (*DisposableCheck, error) {
	var result DisposableCheck
	if err := c.get(ctx, "/v1/verify/disposable", url.Values{"email": {email}}, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// ------------------------------------------------------ file verification

// UploadFile uploads a file for asynchronous verification.
func (c *Client) UploadFile(ctx context.Context, filePath string, opts *FileUploadOptions) (*FileUploadResponse, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, &BillionVerifyError{Code: "FILE_ERROR", Message: err.Error()}
	}
	defer file.Close()

	return c.UploadFileReader(ctx, file, filepath.Base(filePath), opts)
}

// UploadFileReader uploads a file from an io.Reader for asynchronous verification.
func (c *Client) UploadFileReader(ctx context.Context, reader io.Reader, filename string, opts *FileUploadOptions) (*FileUploadResponse, error) {
	checkSMTP := true
	preserveOriginal := true
	emailColumn := ""
	if opts != nil {
		checkSMTP = opts.CheckSMTP
		preserveOriginal = opts.PreserveOriginal
		emailColumn = opts.EmailColumn
	}
	fields := [][2]string{
		{"check_smtp", strconv.FormatBool(checkSMTP)},
		{"preserve_original", strconv.FormatBool(preserveOriginal)},
	}
	if emailColumn != "" {
		fields = append(fields, [2]string{"email_column", emailColumn})
	}

	var result FileUploadResponse
	if err := c.uploadMultipart(ctx, "/v1/verify/file", reader, filename, fields, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// GetFileJobStatus gets the status of a file verification job. With opts.Timeout
// (1-300 seconds) the server holds the request until the job ends or the time is up.
func (c *Client) GetFileJobStatus(ctx context.Context, jobID string, opts *FileJobStatusOptions) (*FileJobResponse, error) {
	query := url.Values{}
	spec := requestSpec{method: http.MethodGet, auth: true}
	if opts != nil {
		if opts.Timeout < 0 {
			return nil, NewValidationError("Timeout must be between 0 and 300 seconds", "")
		}
		if opts.Timeout > 0 {
			timeout := opts.Timeout
			if timeout > 300 {
				timeout = 300
			}
			query.Set("timeout", strconv.Itoa(timeout))
			// the server may hold the request for the whole time
			spec.timeout = time.Duration(timeout)*time.Second + c.timeout
		}
	}
	spec.url = c.endpoint("/v1/verify/file/"+pathID(jobID), query)

	var result FileJobResponse
	if err := c.call(ctx, spec, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// GetFileJobResults downloads the results of a completed file verification job.
// Returns the raw CSV data as bytes. Use FileResultsOptions to filter results.
//
// The result is served from a pre-signed storage URL. The redirect is followed
// here and the API key is not sent to another host.
func (c *Client) GetFileJobResults(ctx context.Context, jobID string, opts *FileResultsOptions) ([]byte, error) {
	query := url.Values{}
	if opts != nil {
		set := func(name string, v *bool) {
			if v != nil {
				query.Set(name, strconv.FormatBool(*v))
			}
		}
		set("valid", opts.Valid)
		set("invalid", opts.Invalid)
		set("catchall", opts.Catchall)
		set("role", opts.Role)
		set("unknown", opts.Unknown)
		set("disposable", opts.Disposable)
		set("risky", opts.Risky)
	}
	return c.download(ctx, "/v1/verify/file/"+pathID(jobID)+"/results", query)
}

// ResumeJob resumes a paused file verification job and returns its status.
func (c *Client) ResumeJob(ctx context.Context, jobID string) (*FileJobResponse, error) {
	var result FileJobResponse
	spec := requestSpec{method: http.MethodPost, url: c.endpoint("/v1/jobs/"+pathID(jobID)+"/resume", nil), auth: true}
	if err := c.call(ctx, spec, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// WaitForFileJobCompletion polls until the job has ended (completed,
// completed_with_warning, failed, cancelled) or is paused. Zero values give a
// 5 second interval and a 10 minute limit.
func (c *Client) WaitForFileJobCompletion(ctx context.Context, jobID string, pollInterval, maxWait time.Duration) (*FileJobResponse, error) {
	if pollInterval == 0 {
		pollInterval = 5 * time.Second
	}
	if maxWait == 0 {
		maxWait = 10 * time.Minute
	}

	deadline := time.Now().Add(maxWait)
	for {
		status, err := c.GetFileJobStatus(ctx, jobID, nil)
		if err != nil {
			return nil, err
		}
		if status.Status.IsSettled() {
			return status, nil
		}
		if time.Now().After(deadline) {
			return nil, NewTimeoutError(fmt.Sprintf("File job %s did not complete within %v", jobID, maxWait))
		}
		if err := c.sleep(ctx, pollInterval); err != nil {
			return nil, err
		}
	}
}

// ----------------------------------------------------------------- credits

// GetCredits gets current credit balance.
func (c *Client) GetCredits(ctx context.Context) (*CreditsResponse, error) {
	var result CreditsResponse
	if err := c.get(ctx, "/v1/credits", nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// ---------------------------------------------------------------- webhooks

// CreateWebhook creates a new webhook. The returned Secret is shown only once.
func (c *Client) CreateWebhook(ctx context.Context, config WebhookConfig) (*Webhook, error) {
	var result Webhook
	if err := c.postJSON(ctx, "/v1/webhooks", config, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// ListWebhooks lists all webhooks.
func (c *Client) ListWebhooks(ctx context.Context) (*WebhooksListResponse, error) {
	var result WebhooksListResponse
	if err := c.get(ctx, "/v1/webhooks", nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// DeleteWebhook deletes a webhook.
func (c *Client) DeleteWebhook(ctx context.Context, webhookID string) error {
	return c.call(ctx, requestSpec{method: http.MethodDelete, url: c.endpoint("/v1/webhooks/"+pathID(webhookID), nil), auth: true}, nil)
}

// VerifyWebhookSignature checks an HMAC of the body alone.
//
// Deliveries are signed over "<timestamp>.<body>" (X-Webhook-Timestamp), so this
// function does not accept them; use VerifyWebhookSignatureWithTimestamp.
func VerifyWebhookSignature(payload, signature, secret string) bool {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(payload))
	expectedSig := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(signature), []byte(expectedSig))
}

// VerifyWebhookSignatureWithTimestamp verifies a webhook delivery. payload is the
// raw request body, signature the X-Webhook-Signature header ("sha256=<hex>"),
// timestamp the X-Webhook-Timestamp header (Unix seconds). The signature is an
// HMAC-SHA256 over "<timestamp>.<payload>" keyed with the webhook secret.
//
// A delivery older or newer than tolerance is rejected; zero means
// DefaultWebhookTolerance (300 s) and a negative value turns the time check off.
func VerifyWebhookSignatureWithTimestamp(payload, signature, timestamp, secret string, tolerance time.Duration) bool {
	return verifyWebhookSignatureAt(payload, signature, timestamp, secret, tolerance, time.Now())
}

func verifyWebhookSignatureAt(payload, signature, timestamp, secret string, tolerance time.Duration, now time.Time) bool {
	if strings.TrimSpace(secret) == "" || signature == "" || timestamp == "" {
		return false
	}
	ts, err := strconv.ParseInt(strings.TrimSpace(timestamp), 10, 64)
	if err != nil || ts < 0 {
		return false
	}
	if tolerance == 0 {
		tolerance = DefaultWebhookTolerance
	}
	if tolerance > 0 {
		// ts >= 0 and now > 0, so neither difference can overflow
		var diff int64
		if n := now.Unix(); n >= ts {
			diff = n - ts
		} else {
			diff = ts - n
		}
		if diff > int64(tolerance/time.Second) {
			return false
		}
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(timestamp + "." + payload))
	expected := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	given := strings.TrimSpace(signature)
	if !strings.HasPrefix(given, "sha256=") {
		given = "sha256=" + given
	}
	return hmac.Equal([]byte(given), []byte(expected))
}

// ----------------------------------------------------------- domain search

func checkMaxEmails(n int) error {
	if n < 1 || n > 200 {
		return NewValidationError("max_emails must be between 1 and 200", "")
	}
	return nil
}

// CreateDomainSearch starts a search for the emails published on a domain's
// site, each one verified. It costs 5 credits per returned email; maxEmails
// (1-200) caps the bill.
func (c *Client) CreateDomainSearch(ctx context.Context, domain string, maxEmails int, opts *DomainSearchOptions) (*DomainSearchCreated, error) {
	if err := checkMaxEmails(maxEmails); err != nil {
		return nil, err
	}
	payload := map[string]interface{}{"domain": domain, "max_emails": maxEmails}
	if opts != nil {
		if opts.IncludeRoleBased != nil {
			payload["include_role_based"] = *opts.IncludeRoleBased
		}
		if opts.MaxPages > 0 {
			payload["max_pages"] = opts.MaxPages
		}
	}
	var result DomainSearchCreated
	if err := c.postJSON(ctx, "/v1/domain/search", payload, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// GetDomainSearch gets the state and, once ended, the result of a domain search.
func (c *Client) GetDomainSearch(ctx context.Context, searchID string) (*DomainSearchStatus, error) {
	var result DomainSearchStatus
	if err := c.get(ctx, "/v1/domain/search/"+pathID(searchID), nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// WaitForDomainSearch polls until the search is completed, completed_with_warning
// or failed. Zero values give a 5 second interval and a 10 minute limit.
func (c *Client) WaitForDomainSearch(ctx context.Context, searchID string, pollInterval, maxWait time.Duration) (*DomainSearchStatus, error) {
	if pollInterval == 0 {
		pollInterval = 5 * time.Second
	}
	if maxWait == 0 {
		maxWait = 10 * time.Minute
	}
	deadline := time.Now().Add(maxWait)
	for {
		status, err := c.GetDomainSearch(ctx, searchID)
		if err != nil {
			return nil, err
		}
		if status.IsFinished() {
			return status, nil
		}
		if time.Now().After(deadline) {
			return nil, NewTimeoutError(fmt.Sprintf("Domain search %s did not finish within %v", searchID, maxWait))
		}
		if err := c.sleep(ctx, pollInterval); err != nil {
			return nil, err
		}
	}
}

// CreateDomainSearchBatch uploads a CSV of domains for a batch domain search.
func (c *Client) CreateDomainSearchBatch(ctx context.Context, filePath string, opts *DomainSearchBatchOptions) (*DomainSearchBatchCreated, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, &BillionVerifyError{Code: "FILE_ERROR", Message: err.Error()}
	}
	defer file.Close()
	return c.CreateDomainSearchBatchReader(ctx, file, filepath.Base(filePath), opts)
}

// CreateDomainSearchBatchReader uploads a CSV of domains read from reader.
func (c *Client) CreateDomainSearchBatchReader(ctx context.Context, reader io.Reader, filename string, opts *DomainSearchBatchOptions) (*DomainSearchBatchCreated, error) {
	var fields [][2]string
	if opts != nil {
		if opts.MaxEmails != 0 {
			if err := checkMaxEmails(opts.MaxEmails); err != nil {
				return nil, err
			}
			fields = append(fields, [2]string{"max_emails", strconv.Itoa(opts.MaxEmails)})
		}
		if opts.IncludeRoleBased != nil {
			fields = append(fields, [2]string{"include_role_based", strconv.FormatBool(*opts.IncludeRoleBased)})
		}
		if opts.MaxPages > 0 {
			fields = append(fields, [2]string{"max_pages", strconv.Itoa(opts.MaxPages)})
		}
		if len(opts.ColumnMap) > 0 {
			b, err := json.Marshal(opts.ColumnMap)
			if err != nil {
				return nil, NewValidationError("Invalid column map", err.Error())
			}
			fields = append(fields, [2]string{"column_map", string(b)})
		}
	}
	var result DomainSearchBatchCreated
	if err := c.uploadMultipart(ctx, "/v1/domain/search/batch", reader, filename, fields, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// ListDomainSearchBatches returns one page of batch domain searches.
func (c *Client) ListDomainSearchBatches(ctx context.Context, opts *ListDomainSearchBatchesOptions) (*DomainSearchBatchPage, error) {
	query := url.Values{}
	if opts != nil {
		if opts.Limit > 0 {
			query.Set("limit", strconv.Itoa(opts.Limit))
		}
		if opts.Status != "" {
			query.Set("status", opts.Status)
		}
		if opts.Cursor != "" {
			query.Set("cursor", opts.Cursor)
		}
	}
	var result DomainSearchBatchPage
	if err := c.get(ctx, "/v1/domain/search/batch", query, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// GetDomainSearchBatch gets the status and progress of a batch domain search.
func (c *Client) GetDomainSearchBatch(ctx context.Context, batchID string) (*DomainSearchBatchStatus, error) {
	var result DomainSearchBatchStatus
	if err := c.get(ctx, "/v1/domain/search/batch/"+pathID(batchID), nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// DownloadDomainSearchBatchResults downloads the results CSV (one row per returned
// email). Until the batch has ended the API answers 409.
func (c *Client) DownloadDomainSearchBatchResults(ctx context.Context, batchID string) ([]byte, error) {
	return c.download(ctx, "/v1/domain/search/batch/"+pathID(batchID)+"/results", nil)
}

// Deprecated: VerifyBulk is deprecated. Use VerifyBatch for synchronous batch verification
// or UploadFile for asynchronous file-based verification.
func (c *Client) VerifyBulk(ctx context.Context, emails []string, opts *BatchVerifyOptions) (*BatchVerifyResponse, error) {
	return c.VerifyBatch(ctx, emails, opts)
}
