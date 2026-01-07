package emailverify

import "time"

// VerificationStatus represents the status of email verification.
type VerificationStatus string

const (
	StatusValid     VerificationStatus = "valid"
	StatusInvalid   VerificationStatus = "invalid"
	StatusUnknown   VerificationStatus = "unknown"
	StatusAcceptAll VerificationStatus = "accept_all"
)

// JobStatus represents the status of a bulk job.
type JobStatus string

const (
	JobStatusPending    JobStatus = "pending"
	JobStatusProcessing JobStatus = "processing"
	JobStatusCompleted  JobStatus = "completed"
	JobStatusFailed     JobStatus = "failed"
)

// WebhookEvent represents a webhook event type.
type WebhookEvent string

const (
	EventVerificationCompleted WebhookEvent = "verification.completed"
	EventBulkCompleted         WebhookEvent = "bulk.completed"
	EventBulkFailed            WebhookEvent = "bulk.failed"
	EventCreditsLow            WebhookEvent = "credits.low"
)

// VerifyOptions contains options for single email verification.
type VerifyOptions struct {
	SMTPCheck bool `json:"smtp_check"`
	Timeout   int  `json:"timeout,omitempty"`
}

// VerificationResult contains detailed verification results.
type VerificationResult struct {
	Deliverable bool `json:"deliverable"`
	ValidFormat bool `json:"valid_format"`
	ValidDomain bool `json:"valid_domain"`
	ValidMX     bool `json:"valid_mx"`
	Disposable  bool `json:"disposable"`
	Role        bool `json:"role"`
	Catchall    bool `json:"catchall"`
	Free        bool `json:"free"`
	SMTPValid   bool `json:"smtp_valid"`
}

// VerifyResponse is the response from single email verification.
type VerifyResponse struct {
	Email       string             `json:"email"`
	Status      VerificationStatus `json:"status"`
	Result      VerificationResult `json:"result"`
	Score       float64            `json:"score"`
	Reason      *string            `json:"reason"`
	CreditsUsed int                `json:"credits_used"`
}

// BulkVerifyOptions contains options for bulk verification.
type BulkVerifyOptions struct {
	SMTPCheck  bool   `json:"smtp_check"`
	WebhookURL string `json:"webhook_url,omitempty"`
}

// BulkJobResponse is the response from bulk verification operations.
type BulkJobResponse struct {
	JobID           string    `json:"job_id"`
	Status          JobStatus `json:"status"`
	Total           int       `json:"total"`
	Processed       int       `json:"processed"`
	Valid           int       `json:"valid"`
	Invalid         int       `json:"invalid"`
	Unknown         int       `json:"unknown"`
	CreditsUsed     int       `json:"credits_used"`
	CreatedAt       string    `json:"created_at"`
	CompletedAt     *string   `json:"completed_at,omitempty"`
	ProgressPercent *int      `json:"progress_percent,omitempty"`
}

// BulkResultItem represents a single result from bulk verification.
type BulkResultItem struct {
	Email  string                 `json:"email"`
	Status VerificationStatus     `json:"status"`
	Result map[string]interface{} `json:"result"`
	Score  float64                `json:"score"`
}

// BulkResultsOptions contains options for getting bulk results.
type BulkResultsOptions struct {
	Limit  int                `json:"limit,omitempty"`
	Offset int                `json:"offset,omitempty"`
	Status VerificationStatus `json:"status,omitempty"`
}

// BulkResultsResponse is the response from getting bulk job results.
type BulkResultsResponse struct {
	JobID   string           `json:"job_id"`
	Total   int              `json:"total"`
	Limit   int              `json:"limit"`
	Offset  int              `json:"offset"`
	Results []BulkResultItem `json:"results"`
}

// RateLimit contains rate limit information.
type RateLimit struct {
	RequestsPerHour int `json:"requests_per_hour"`
	Remaining       int `json:"remaining"`
}

// CreditsResponse is the response from credits endpoint.
type CreditsResponse struct {
	Available int       `json:"available"`
	Used      int       `json:"used"`
	Total     int       `json:"total"`
	Plan      string    `json:"plan"`
	ResetsAt  string    `json:"resets_at"`
	RateLimit RateLimit `json:"rate_limit"`
}

// WebhookConfig contains webhook configuration.
type WebhookConfig struct {
	URL    string         `json:"url"`
	Events []WebhookEvent `json:"events"`
	Secret string         `json:"secret,omitempty"`
}

// Webhook represents a webhook.
type Webhook struct {
	ID        string         `json:"id"`
	URL       string         `json:"url"`
	Events    []WebhookEvent `json:"events"`
	CreatedAt string         `json:"created_at"`
}

// WebhookPayload is the payload sent to webhook endpoints.
type WebhookPayload struct {
	Event     WebhookEvent           `json:"event"`
	Timestamp time.Time              `json:"timestamp"`
	Data      map[string]interface{} `json:"data"`
	Signature string                 `json:"signature"`
}

// APIError represents an API error.
type APIError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details string `json:"details,omitempty"`
}
