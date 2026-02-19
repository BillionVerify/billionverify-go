package billionverify

import "time"

// VerificationStatus represents the status of email verification.
type VerificationStatus string

const (
	StatusValid      VerificationStatus = "valid"
	StatusInvalid    VerificationStatus = "invalid"
	StatusUnknown    VerificationStatus = "unknown"
	StatusRisky      VerificationStatus = "risky"
	StatusDisposable VerificationStatus = "disposable"
	StatusCatchall   VerificationStatus = "catchall"
	StatusRole       VerificationStatus = "role"
)

// JobStatus represents the status of a file verification job.
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
	EventFileCompleted WebhookEvent = "file.completed"
	EventFileFailed    WebhookEvent = "file.failed"
)

// VerifyOptions contains options for single email verification.
type VerifyOptions struct {
	CheckSMTP bool `json:"check_smtp"`
}

// DomainReputation contains domain reputation information.
type DomainReputation struct {
	MxIP       string   `json:"mx_ip,omitempty"`
	IsListed   bool     `json:"is_listed"`
	Blacklists []string `json:"blacklists,omitempty"`
	Checked    bool     `json:"checked"`
}

// VerificationResult contains detailed verification results.
type VerificationResult struct {
	IsDeliverable    bool              `json:"is_deliverable"`
	IsDisposable     bool              `json:"is_disposable"`
	IsCatchall       bool              `json:"is_catchall"`
	IsRole           bool              `json:"is_role"`
	IsFree           bool              `json:"is_free"`
	SMTPCheck        bool              `json:"smtp_check"`
	Domain           string            `json:"domain,omitempty"`
	DomainAge        int               `json:"domain_age,omitempty"`
	MXRecords        []string          `json:"mx_records,omitempty"`
	DomainReputation *DomainReputation `json:"domain_reputation,omitempty"`
	Suggestion       string            `json:"suggestion,omitempty"`
	ResponseTime     int               `json:"response_time,omitempty"`
}

// VerifyResponse is the response from single email verification.
type VerifyResponse struct {
	Email            string             `json:"email"`
	Status           VerificationStatus `json:"status"`
	Score            float64            `json:"score"`
	IsDeliverable    bool               `json:"is_deliverable"`
	IsDisposable     bool               `json:"is_disposable"`
	IsCatchall       bool               `json:"is_catchall"`
	IsRole           bool               `json:"is_role"`
	IsFree           bool               `json:"is_free"`
	Domain           string             `json:"domain,omitempty"`
	DomainAge        int                `json:"domain_age,omitempty"`
	MXRecords        []string           `json:"mx_records,omitempty"`
	DomainReputation *DomainReputation  `json:"domain_reputation,omitempty"`
	SMTPCheck        bool               `json:"smtp_check"`
	Reason           string             `json:"reason,omitempty"`
	Suggestion       string             `json:"suggestion,omitempty"`
	ResponseTime     int                `json:"response_time,omitempty"`
	CreditsUsed      int                `json:"credits_used"`
}

// BatchVerifyOptions contains options for batch verification.
type BatchVerifyOptions struct {
	CheckSMTP bool `json:"check_smtp"`
}

// BatchResultItem represents a single result from batch verification.
type BatchResultItem struct {
	Email         string             `json:"email"`
	Status        VerificationStatus `json:"status"`
	Score         float64            `json:"score"`
	IsDeliverable bool               `json:"is_deliverable"`
	IsDisposable  bool               `json:"is_disposable"`
	IsCatchall    bool               `json:"is_catchall"`
	IsRole        bool               `json:"is_role"`
	IsFree        bool               `json:"is_free"`
	CreditsUsed   int                `json:"credits_used"`
}

// BatchVerifyResponse is the response from batch verification (synchronous).
type BatchVerifyResponse struct {
	Results       []BatchResultItem `json:"results"`
	TotalEmails   int               `json:"total_emails"`
	ValidEmails   int               `json:"valid_emails"`
	InvalidEmails int               `json:"invalid_emails"`
	CreditsUsed   int               `json:"credits_used"`
	ProcessTime   int               `json:"process_time"`
}

// FileUploadOptions contains options for file upload verification.
type FileUploadOptions struct {
	CheckSMTP        bool   `json:"check_smtp"`
	EmailColumn      string `json:"email_column,omitempty"`
	PreserveOriginal bool   `json:"preserve_original"`
}

// FileUploadResponse is the response from file upload.
type FileUploadResponse struct {
	TaskID         string `json:"task_id"`
	Status         string `json:"status"`
	Message        string `json:"message,omitempty"`
	FileName       string `json:"file_name"`
	FileSize       int64  `json:"file_size"`
	EstimatedCount int    `json:"estimated_count,omitempty"`
	UniqueEmails   int    `json:"unique_emails,omitempty"`
	TotalRows      int    `json:"total_rows,omitempty"`
	EmailColumn    string `json:"email_column,omitempty"`
	StatusURL      string `json:"status_url,omitempty"`
	CreatedAt      string `json:"created_at"`
}

// FileJobStatusOptions contains options for getting file job status.
type FileJobStatusOptions struct {
	Timeout int `json:"timeout,omitempty"` // Long-polling timeout in seconds (0-300)
}

// FileJobResponse is the response from file verification job status.
type FileJobResponse struct {
	JobID              string  `json:"job_id"`
	Status             JobStatus `json:"status"`
	FileName           string  `json:"file_name,omitempty"`
	TotalEmails        int     `json:"total_emails"`
	ProcessedEmails    int     `json:"processed_emails"`
	ProgressPercent    int     `json:"progress_percent"`
	ValidEmails        int     `json:"valid_emails"`
	InvalidEmails      int     `json:"invalid_emails"`
	UnknownEmails      int     `json:"unknown_emails"`
	RoleEmails         int     `json:"role_emails"`
	CatchallEmails     int     `json:"catchall_emails"`
	DisposableEmails   int     `json:"disposable_emails"`
	CreditsUsed        int     `json:"credits_used"`
	ProcessTimeSeconds float64 `json:"process_time_seconds,omitempty"`
	ResultFilePath     string  `json:"result_file_path,omitempty"`
	DownloadURL        string  `json:"download_url,omitempty"`
	CreatedAt          string  `json:"created_at"`
	CompletedAt        string  `json:"completed_at,omitempty"`
}

// FileResultsOptions contains options for downloading file results.
type FileResultsOptions struct {
	Valid      *bool `json:"valid,omitempty"`
	Invalid    *bool `json:"invalid,omitempty"`
	Catchall   *bool `json:"catchall,omitempty"`
	Role       *bool `json:"role,omitempty"`
	Unknown    *bool `json:"unknown,omitempty"`
	Disposable *bool `json:"disposable,omitempty"`
	Risky      *bool `json:"risky,omitempty"`
}

// RateLimit contains rate limit information.
type RateLimit struct {
	RequestsPerHour int `json:"requests_per_hour"`
	Remaining       int `json:"remaining"`
}

// CreditsResponse is the response from credits endpoint.
type CreditsResponse struct {
	AccountID       string `json:"account_id"`
	APIKeyID        string `json:"api_key_id"`
	APIKeyName      string `json:"api_key_name"`
	CreditsBalance  int    `json:"credits_balance"`
	CreditsConsumed int    `json:"credits_consumed"`
	CreditsAdded    int    `json:"credits_added"`
	LastUpdated     string `json:"last_updated"`
}

// WebhookConfig contains webhook configuration for creating webhooks.
type WebhookConfig struct {
	URL    string         `json:"url"`
	Events []WebhookEvent `json:"events"`
}

// Webhook represents a webhook.
type Webhook struct {
	ID        string         `json:"id"`
	URL       string         `json:"url"`
	Events    []WebhookEvent `json:"events"`
	Secret    string         `json:"secret,omitempty"` // Only returned on creation
	IsActive  bool           `json:"is_active"`
	CreatedAt string         `json:"created_at"`
	UpdatedAt string         `json:"updated_at"`
}

// WebhooksListResponse is the response from listing webhooks.
type WebhooksListResponse struct {
	Webhooks []Webhook `json:"webhooks"`
	Total    int       `json:"total"`
}

// WebhookPayload is the payload sent to webhook endpoints.
type WebhookPayload struct {
	Event     WebhookEvent           `json:"event"`
	Timestamp time.Time              `json:"timestamp"`
	Data      map[string]interface{} `json:"data"`
}

// HealthResponse is the response from health check endpoint.
type HealthResponse struct {
	Status string `json:"status"`
	Time   int64  `json:"time"`
}

// APIError represents an API error.
type APIError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details string `json:"details,omitempty"`
}

// APIResponse is the generic API response wrapper.
type APIResponse struct {
	Success bool        `json:"success"`
	Code    string      `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
	Error   *APIError   `json:"error,omitempty"`
}
