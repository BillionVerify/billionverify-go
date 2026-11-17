package billionverify

import (
	"encoding/json"
	"time"
)

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
	JobStatusPreparing            JobStatus = "preparing"
	JobStatusPending              JobStatus = "pending"
	JobStatusRunning              JobStatus = "running"
	JobStatusProcessing           JobStatus = "processing"
	JobStatusMerging              JobStatus = "merging"
	JobStatusPaused               JobStatus = "paused"
	JobStatusCompleted            JobStatus = "completed"
	JobStatusCompletedWithWarning JobStatus = "completed_with_warning"
	JobStatusFailed               JobStatus = "failed"
	JobStatusCancelled            JobStatus = "cancelled"
)

// IsSettled reports whether a job will not move on by itself: it has ended
// (completed, completed_with_warning, failed, cancelled) or it is paused.
func (s JobStatus) IsSettled() bool {
	switch s {
	case JobStatusCompleted, JobStatusCompletedWithWarning, JobStatusFailed, JobStatusCancelled, JobStatusPaused:
		return true
	}
	return false
}

// WebhookEvent represents a webhook event type.
type WebhookEvent string

const (
	EventFileCompleted WebhookEvent = "file.completed"
	EventFileFailed    WebhookEvent = "file.failed"
)

// VerifyOptions contains options for single email verification.
//
// CheckSMTP is sent whenever options are given (and as true when no options are
// given). Set ServerDefaultSMTP to leave the choice to the server, which makes
// CheckSMTP be ignored.
type VerifyOptions struct {
	CheckSMTP bool `json:"check_smtp"`
	// ServerDefaultSMTP leaves check_smtp out of the request so the server default applies.
	ServerDefaultSMTP bool `json:"-"`
	// ForceRefresh skips every cache and verifies live; a credit is always used.
	ForceRefresh bool `json:"force_refresh,omitempty"`
	// IncludeDomainReputation also fetches the MX IP reputation for cached results.
	IncludeDomainReputation bool `json:"include_domain_reputation,omitempty"`
}

// DomainReputation contains domain reputation information.
type DomainReputation struct {
	MxIP            string   `json:"mx_ip,omitempty"`
	IsListed        bool     `json:"is_listed"`
	Blacklists      []string `json:"blacklists,omitempty"`
	Advisory        []string `json:"advisory,omitempty"`
	Checked         bool     `json:"checked"`
	DropListed      bool     `json:"drop_listed,omitempty"`
	DropCIDR        string   `json:"drop_cidr,omitempty"`
	DropSBLID       string   `json:"drop_sblid,omitempty"`
	DropAttribution string   `json:"drop_attribution,omitempty"`
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
	// SMTPCheck is filled from CheckedSMTP; the API's field is check_smtp.
	SMTPCheck        bool     `json:"smtp_check"`
	CheckedSMTP      bool     `json:"check_smtp"`
	SMTPResult       string   `json:"smtp_result,omitempty"`
	CatchAllEvidence string   `json:"catch_all_evidence,omitempty"`
	Reason           string   `json:"reason,omitempty"`
	Suggestion       string   `json:"suggestion,omitempty"`
	DomainSuggestion string   `json:"domain_suggestion,omitempty"`
	AliasOf          string   `json:"alias_of,omitempty"`
	RiskReasons      []string `json:"risk_reasons,omitempty"`
	MxIP             string   `json:"mx_ip,omitempty"`
	MxASN            int      `json:"mx_asn,omitempty"`
	MxASOrg          string   `json:"mx_as_org,omitempty"`
	SMTPResponse     string   `json:"smtp_response,omitempty"`
	ResponseTime     int      `json:"response_time,omitempty"`
	CreditsUsed      int      `json:"credits_used"`
	VerificationMode string   `json:"verification_mode,omitempty"`
	Retryable        bool     `json:"retryable"`
	SMTPUTF8         bool     `json:"smtputf8,omitempty"`
	// RetryAfterSeconds is a hint for transient unknowns (for example greylisting).
	RetryAfterSeconds *int `json:"retry_after_seconds,omitempty"`
}

// UnmarshalJSON decodes the response and fills the legacy SMTPCheck field.
func (r *VerifyResponse) UnmarshalJSON(data []byte) error {
	type plain VerifyResponse
	var p plain
	if err := json.Unmarshal(data, &p); err != nil {
		return err
	}
	*r = VerifyResponse(p)
	if !r.SMTPCheck {
		r.SMTPCheck = r.CheckedSMTP
	}
	return nil
}

// BatchVerifyOptions contains options for batch verification.
type BatchVerifyOptions struct {
	CheckSMTP bool `json:"check_smtp"`
	// ServerDefaultSMTP leaves check_smtp out of the request so the server default applies.
	ServerDefaultSMTP bool `json:"-"`
	// ForceRefresh skips every cache and verifies live; credits are always used.
	ForceRefresh bool `json:"force_refresh,omitempty"`
	// IncludeDomainReputation also fetches the MX IP reputation for cached results.
	IncludeDomainReputation bool `json:"include_domain_reputation,omitempty"`
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
	Domain        string             `json:"domain,omitempty"`
	Reason        string             `json:"reason,omitempty"`
	CreditsUsed   int                `json:"credits_used"`
	// Fields below are optional in the API and are zero when absent.
	VerificationMode  string   `json:"verification_mode,omitempty"`
	Retryable         bool     `json:"retryable"`
	SMTPResult        string   `json:"smtp_result,omitempty"`
	RiskReasons       []string `json:"risk_reasons,omitempty"`
	RetryAfterSeconds *int     `json:"retry_after_seconds,omitempty"`
}

// BatchVerifyResponse is the response from batch verification (synchronous).
type BatchVerifyResponse struct {
	Results       []BatchResultItem `json:"results"`
	TotalEmails   int               `json:"total_emails"`
	ValidEmails   int               `json:"valid_emails"`
	InvalidEmails int               `json:"invalid_emails"`
	CreditsUsed   int               `json:"credits_used"`
	ProcessTime   int               `json:"process_time"`
	// RetryableEmails can be re-submitted with ForceRefresh.
	RetryableEmails []string `json:"retryable_emails,omitempty"`
	// DeferredEmails were moved to asynchronous processing; retry later.
	DeferredEmails    []string `json:"deferred_emails,omitempty"`
	RetryAfterSeconds *int     `json:"retry_after_seconds,omitempty"`
	// DeliverabilityBuckets is set on every batch response.
	DeliverabilityBuckets *DeliverabilityBuckets `json:"deliverability_buckets,omitempty"`
}

// DeliverabilityBuckets are pre-aggregated business buckets of a batch or file job.
type DeliverabilityBuckets struct {
	StrictDeliverable  int `json:"strict_deliverable"`
	UsableWithCatchall int `json:"usable_with_catchall"`
	NotDeliverable     int `json:"not_deliverable"`
	Unknown            int `json:"unknown"`
	AntiVerified       int `json:"anti_verified"`
	TransientFailure   int `json:"transient_failure"`
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
	// EmailColumnCandidates lists the other columns that held addresses when none clearly dominated.
	EmailColumnCandidates []string `json:"email_column_candidates,omitempty"`
	StatusURL             string   `json:"status_url,omitempty"`
	CreatedAt             string   `json:"created_at"`
}

// FileJobStatusOptions contains options for getting file job status.
type FileJobStatusOptions struct {
	Timeout int `json:"timeout,omitempty"` // Long-polling timeout in seconds (0-300)
}

// FileJobResponse is the status of a file verification job.
//
// The API names the job id task_id and the percentage progress; TaskID and
// Progress hold them. JobID and ProgressPercent are kept for existing callers
// and are filled from the same values.
type FileJobResponse struct {
	TaskID string    `json:"task_id"`
	JobID  string    `json:"job_id"`
	Status JobStatus `json:"status"`
	// Stage is the sub-stage while processing: verifying, retry_tail or generating_report.
	Stage                   string  `json:"stage,omitempty"`
	PendingRetryEmails      *int    `json:"pending_retry_emails,omitempty"`
	FileName                string  `json:"file_name,omitempty"`
	Progress                int     `json:"progress"`
	ProgressPercent         int     `json:"progress_percent"`
	EmailProgress           *int    `json:"email_progress,omitempty"`
	ChunkProgress           *int    `json:"chunk_progress,omitempty"`
	ProgressSource          string  `json:"progress_source,omitempty"`
	TotalEmails             int     `json:"total_emails"`
	ProcessedEmails         int     `json:"processed_emails"`
	ValidEmails             int     `json:"valid_emails"`
	InvalidEmails           int     `json:"invalid_emails"`
	UnknownEmails           int     `json:"unknown_emails"`
	RoleEmails              int     `json:"role_emails"`
	CatchallEmails          int     `json:"catchall_emails"`
	RiskyEmails             int     `json:"risky_emails"`
	DisposableEmails        int     `json:"disposable_emails"`
	DisposableChargedEmails int     `json:"disposable_charged_emails"`
	CreditsUsed             int     `json:"credits_used"`
	CacheHitNotCharged      *int    `json:"cache_hit_not_charged,omitempty"`
	ProcessTimeSeconds      float64 `json:"process_time_seconds,omitempty"`
	ResultFilePath          string  `json:"result_file_path,omitempty"`
	DownloadURL             string  `json:"download_url,omitempty"`
	// DirectDownloadURL is a pre-signed storage URL for the result CSV.
	DirectDownloadURL               string                 `json:"direct_download_url,omitempty"`
	DirectDownloadExpiresAt         string                 `json:"direct_download_expires_at,omitempty"`
	ResultDownloadAvailable         bool                   `json:"result_download_available"`
	ResultDownloadExpiresAt         string                 `json:"result_download_expires_at,omitempty"`
	ResultDownloadUnavailableReason string                 `json:"result_download_unavailable_reason,omitempty"`
	CreatedAt                       string                 `json:"created_at"`
	StartedAt                       string                 `json:"started_at,omitempty"`
	CompletedAt                     string                 `json:"completed_at,omitempty"`
	ErrorMessage                    string                 `json:"error_message,omitempty"`
	PauseReason                     string                 `json:"pause_reason,omitempty"`
	CanPause                        *bool                  `json:"can_pause,omitempty"`
	CanResume                       *bool                  `json:"can_resume,omitempty"`
	CanRestart                      *bool                  `json:"can_restart,omitempty"`
	RestartAttempts                 *int                   `json:"restart_attempts,omitempty"`
	CurrentAttempt                  *int                   `json:"current_attempt,omitempty"`
	TotalChunks                     *int                   `json:"total_chunks,omitempty"`
	CompletedChunks                 *int                   `json:"completed_chunks,omitempty"`
	FailedChunks                    *int                   `json:"failed_chunks,omitempty"`
	UniqueEmails                    *int                   `json:"unique_emails,omitempty"`
	TotalRows                       *int                   `json:"total_rows,omitempty"`
	RuntimeHints                    *FileJobRuntimeHints   `json:"runtime_hints,omitempty"`
	CapacityWait                    *FileJobCapacityWait   `json:"capacity_wait,omitempty"`
	DeliverabilityBuckets           *DeliverabilityBuckets `json:"deliverability_buckets,omitempty"`
}

// FileJobRuntimeHints is per-job runtime telemetry.
type FileJobRuntimeHints struct {
	HasLiveExecution    *bool  `json:"has_live_execution,omitempty"`
	InflightBatches     *int   `json:"inflight_batches,omitempty"`
	LastExecutionFactAt string `json:"last_execution_fact_at,omitempty"`
}

// FileJobCapacityWait is set while the remaining addresses wait for capacity.
// NextEligibleAt is a next-attempt time, not a completion estimate.
type FileJobCapacityWait struct {
	WaitingEmails  *int   `json:"waiting_emails,omitempty"`
	NextEligibleAt string `json:"next_eligible_at,omitempty"`
	WaitingSince   string `json:"waiting_since,omitempty"`
}

// UnmarshalJSON decodes the status and keeps TaskID / JobID and Progress /
// ProgressPercent in step, whichever name the server used.
func (r *FileJobResponse) UnmarshalJSON(data []byte) error {
	type plain FileJobResponse
	var p plain
	if err := json.Unmarshal(data, &p); err != nil {
		return err
	}
	*r = FileJobResponse(p)
	if r.TaskID == "" {
		r.TaskID = r.JobID
	}
	if r.JobID == "" {
		r.JobID = r.TaskID
	}
	if r.Progress == 0 {
		r.Progress = r.ProgressPercent
	}
	if r.ProgressPercent == 0 {
		r.ProgressPercent = r.Progress
	}
	return nil
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
	ID       string         `json:"id"`
	URL      string         `json:"url"`
	Events   []WebhookEvent `json:"events"`
	Secret   string         `json:"secret,omitempty"` // Only returned on creation
	IsActive bool           `json:"is_active"`
	// Delivery health, set on listed webhooks. LastDeliveryStatus is success or failed.
	LastDeliveryStatus string `json:"last_delivery_status,omitempty"`
	LastDeliveryAt     string `json:"last_delivery_at,omitempty"`
	LastError          string `json:"last_error,omitempty"`
	CreatedAt          string `json:"created_at"`
	UpdatedAt          string `json:"updated_at"`
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

// DisposableCheck is the result of the free disposable-domain lookup.
type DisposableCheck struct {
	Email        string `json:"email"`
	Domain       string `json:"domain"`
	IsDisposable bool   `json:"is_disposable"`
	CheckedAt    string `json:"checked_at,omitempty"`
}

// DomainSearchOptions contains optional fields of a domain search.
type DomainSearchOptions struct {
	// IncludeRoleBased keeps info@ / sales@ style addresses (server default: true).
	IncludeRoleBased *bool `json:"include_role_based,omitempty"`
	// MaxPages crawls the sitemap up to this many pages (0 uses the default two passes).
	MaxPages int `json:"max_pages,omitempty"`
}

// DomainSearchCreated is the response to an accepted domain search.
type DomainSearchCreated struct {
	SearchID        string `json:"search_id"`
	Status          string `json:"status"`
	Domain          string `json:"domain"`
	CreditsReserved int    `json:"credits_reserved"`
	StatusURL       string `json:"status_url"`
}

// DomainSearchStatus is the state of a domain search. Status ends as completed,
// completed_with_warning or failed; Result (emails, crawl_report, warnings) is set once it ended.
type DomainSearchStatus struct {
	SearchID       string                 `json:"search_id"`
	Status         string                 `json:"status"`
	Domain         string                 `json:"domain"`
	CreditsCharged int                    `json:"credits_charged"`
	Result         map[string]interface{} `json:"result,omitempty"`
}

// IsFinished reports whether the search has ended.
func (s DomainSearchStatus) IsFinished() bool {
	return s.Status == "completed" || s.Status == "completed_with_warning" || s.Status == "failed"
}

// DomainSearchBatchOptions contains optional fields of a batch domain search.
type DomainSearchBatchOptions struct {
	// MaxEmails per domain, 1-200 (0 leaves the server default of 200).
	MaxEmails        int   `json:"max_emails,omitempty"`
	IncludeRoleBased *bool `json:"include_role_based,omitempty"`
	MaxPages         int   `json:"max_pages,omitempty"`
	// ColumnMap names the CSV column of the domain, e.g. {"domain": "Company Website"}.
	ColumnMap map[string]string `json:"column_map,omitempty"`
}

// DomainSearchBatchCreated is the response to an accepted batch.
type DomainSearchBatchCreated struct {
	BatchID          string `json:"batch_id"`
	Status           string `json:"status"`
	TotalRows        int    `json:"total_rows"`
	UniqueDomains    int    `json:"unique_domains"`
	MaxEmails        int    `json:"max_emails"`
	IncludeRoleBased bool   `json:"include_role_based"`
	MaxPages         int    `json:"max_pages,omitempty"`
	StatusURL        string `json:"status_url"`
	ResultsURL       string `json:"results_url"`
}

// DomainSearchBatchProgress is the progress of a batch.
type DomainSearchBatchProgress struct {
	TotalRows      int `json:"total_rows"`
	Processed      int `json:"processed"`
	Found          int `json:"found"`
	EmailsReturned int `json:"emails_returned"`
	UniqueDomains  int `json:"unique_domains"`
}

// DomainSearchBatchStatus is the state of a batch; CreditsCharged grows as rows settle.
type DomainSearchBatchStatus struct {
	BatchID        string                    `json:"batch_id"`
	Status         string                    `json:"status"`
	MaxEmails      int                       `json:"max_emails,omitempty"`
	MaxPages       int                       `json:"max_pages,omitempty"`
	Progress       DomainSearchBatchProgress `json:"progress"`
	CreditsCharged int                       `json:"credits_charged"`
}

// ListDomainSearchBatchesOptions filters and pages the list of batches.
type ListDomainSearchBatchesOptions struct {
	Limit  int    // up to 100 (0 uses the server default of 20)
	Status string // queued, running, completed, completed_with_warning, failed, cancelled
	Cursor string // the NextCursor of the previous page
}

// DomainSearchBatchPage is one page of batches.
type DomainSearchBatchPage struct {
	Items      []map[string]interface{} `json:"items"`
	HasMore    bool                     `json:"has_more"`
	NextCursor string                   `json:"next_cursor,omitempty"`
}
