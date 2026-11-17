# BillionVerify Go SDK

Official BillionVerify Go SDK for email verification.

**Documentation:** https://billionverify.com/docs

## Installation

```bash
go get github.com/BillionVerify/billionverify-go
```

## Quick Start

```go
package main

import (
    "context"
    "fmt"
    "log"

    billionverify "github.com/BillionVerify/billionverify-go"
)

func main() {
    client, err := billionverify.NewClient(billionverify.Config{
        APIKey: "your-api-key",
    })
    if err != nil {
        log.Fatal(err)
    }

    result, err := client.Verify(context.Background(), "user@example.com", nil)
    if err != nil {
        log.Fatal(err)
    }

    fmt.Printf("Email: %s\n", result.Email)
    fmt.Printf("Status: %s\n", result.Status)
    fmt.Printf("Deliverable: %t\n", result.IsDeliverable)
}
```

## Configuration

```go
client, err := billionverify.NewClient(billionverify.Config{
    APIKey:  "your-api-key",                      // Required
    BaseURL: "https://api.billionverify.com",        // Optional
    Timeout: 30 * time.Second,                    // Optional (default: 30s)
    Retries: 3,                                   // Optional (default: 3)
})
```

## Single Email Verification

Uses the `/verify/single` endpoint for verifying individual emails.

```go
result, err := client.Verify(context.Background(), "user@example.com", &billionverify.VerifyOptions{
    CheckSMTP: true, // Optional: Perform SMTP verification (default: true)
})
if err != nil {
    log.Fatal(err)
}

// Flat response structure (no nested Result object)
fmt.Printf("Email: %s\n", result.Email)
fmt.Printf("Status: %s\n", result.Status)          // valid, invalid, unknown, risky, disposable, catchall, role
fmt.Printf("Score: %.2f\n", result.Score)
fmt.Printf("Deliverable: %t\n", result.IsDeliverable)
fmt.Printf("Disposable: %t\n", result.IsDisposable)
fmt.Printf("Catchall: %t\n", result.IsCatchall)
fmt.Printf("Role: %t\n", result.IsRole)
fmt.Printf("Free: %t\n", result.IsFree)
fmt.Printf("Domain: %s\n", result.Domain)
fmt.Printf("SMTP Check: %t\n", result.SMTPCheck)
fmt.Printf("Credits Used: %d\n", result.CreditsUsed)
```

## Batch Email Verification (Synchronous)

Use `VerifyBatch()` for synchronous verification of up to 50 emails at once.

```go
// Submit a batch verification (max 50 emails)
response, err := client.VerifyBatch(context.Background(), []string{
    "user1@example.com",
    "user2@example.com",
    "user3@example.com",
}, &billionverify.BatchVerifyOptions{
    CheckSMTP: true, // Optional: Perform SMTP verification (default: true)
})
if err != nil {
    log.Fatal(err)
}

fmt.Printf("Total Emails: %d\n", response.TotalEmails)
fmt.Printf("Valid Emails: %d\n", response.ValidEmails)
fmt.Printf("Invalid Emails: %d\n", response.InvalidEmails)
fmt.Printf("Credits Used: %d\n", response.CreditsUsed)

// Iterate through results
for _, item := range response.Results {
    fmt.Printf("%s: %s (deliverable: %t)\n", item.Email, item.Status, item.IsDeliverable)
}
```

## File Upload Verification (Asynchronous)

Use `UploadFile()` for asynchronous verification of large email lists from CSV/TXT files.

```go
// Upload a file for verification
job, err := client.UploadFile(context.Background(), "emails.csv", &billionverify.FileUploadOptions{
    CheckSMTP:        true,    // Optional: Perform SMTP verification
    EmailColumn:      "email", // Optional: Column name containing emails
    PreserveOriginal: true,    // Optional: Keep original columns in results
})
if err != nil {
    log.Fatal(err)
}

fmt.Printf("Task ID: %s\n", job.TaskID)
fmt.Printf("Status: %s\n", job.Status)
fmt.Printf("File Name: %s\n", job.FileName)

// Get job status with optional long-polling (timeout in seconds, max 300)
status, err := client.GetFileJobStatus(context.Background(), job.TaskID, &billionverify.FileJobStatusOptions{
    Timeout: 60, // Wait up to 60 seconds for status change
})
if err != nil {
    log.Fatal(err)
}
fmt.Printf("Progress: %d%%\n", status.Progress)

// Wait for completion (polling)
completed, err := client.WaitForFileJobCompletion(
    context.Background(),
    job.TaskID,
    5*time.Second,   // poll interval
    10*time.Minute,  // max wait
)
if err != nil {
    log.Fatal(err)
}

fmt.Printf("Status: %s\n", completed.Status)
fmt.Printf("Valid: %d\n", completed.ValidEmails)
fmt.Printf("Invalid: %d\n", completed.InvalidEmails)

// Download results with optional filters
validOnly := true
csvData, err := client.GetFileJobResults(context.Background(), job.TaskID, &billionverify.FileResultsOptions{
    Valid:      &validOnly, // Only get valid emails
    Invalid:    nil,        // Include all
    Catchall:   nil,
    Role:       nil,
    Unknown:    nil,
    Disposable: nil,
    Risky:      nil,
})
if err != nil {
    log.Fatal(err)
}

// Save results to file
os.WriteFile("results.csv", csvData, 0644)
```

## Disposable Check (free)

```go
d, _ := client.CheckDisposable(ctx, "someone@mailinator.com")        // POST
d, _ = client.CheckDisposableViaQuery(ctx, "someone@mailinator.com") // GET
fmt.Println(d.IsDisposable)
```

## Verification Options

```go
// check_smtp is sent as CheckSMTP (true when you pass no options). Set ServerDefaultSMTP to
// leave it out so the server default applies.
res, _ := client.Verify(ctx, "user@example.com", &billionverify.VerifyOptions{
    ServerDefaultSMTP:       true,
    ForceRefresh:            true, // skip every cache; a credit is always used
    IncludeDomainReputation: true,
})

batch, _ := client.VerifyBatch(ctx, emails, &billionverify.BatchVerifyOptions{CheckSMTP: true})
fmt.Println(batch.DeliverabilityBuckets.NotDeliverable, batch.RetryableEmails)
```

## Paused Jobs

```go
status, _ := client.ResumeJob(ctx, job.TaskID)
```

`WaitForFileJobCompletion` returns when a job has ended (`completed`, `completed_with_warning`, `failed`, `cancelled`) or is `paused`.

## Domain Search

Give a domain, get the emails published on its site, each one verified. 5 credits per returned email; `maxEmails` (1-200) caps the bill.

```go
no := false
search, _ := client.CreateDomainSearch(ctx, "example.com", 20, &billionverify.DomainSearchOptions{IncludeRoleBased: &no, MaxPages: 100})
done, _ := client.WaitForDomainSearch(ctx, search.SearchID, 0, 0)
fmt.Println(done.Status, done.CreditsCharged, done.Result["emails"])

// A CSV of domains
batch, _ := client.CreateDomainSearchBatch(ctx, "domains.csv", &billionverify.DomainSearchBatchOptions{
    MaxEmails: 10,
    ColumnMap: map[string]string{"domain": "Company Website"},
})
st, _ := client.GetDomainSearchBatch(ctx, batch.BatchID)
page, _ := client.ListDomainSearchBatches(ctx, &billionverify.ListDomainSearchBatchesOptions{Limit: 20, Status: "completed"})
csv, _ := client.DownloadDomainSearchBatchResults(ctx, batch.BatchID) // 409 until the batch has finished
```

## Result Downloads

`GetFileJobResults` and `DownloadDomainSearchBatchResults` follow the redirect to the pre-signed storage URL themselves and never send your API key to another host.

## Health Check

Use `Health()` to check API availability (no authentication required).

```go
health, err := client.Health(context.Background())
if err != nil {
    log.Fatal(err)
}

fmt.Printf("Status: %s\n", health.Status)
fmt.Printf("Time: %d\n", health.Time)
```

## Credits

```go
credits, err := client.GetCredits(context.Background())
if err != nil {
    log.Fatal(err)
}

fmt.Printf("Credits Balance: %d\n", credits.CreditsBalance)
fmt.Printf("Credits Consumed: %d\n", credits.CreditsConsumed)
fmt.Printf("Account ID: %s\n", credits.AccountID)
```

## Webhooks

Webhook events: `file.completed`, `file.failed`

```go
// Create a webhook
webhook, err := client.CreateWebhook(context.Background(), billionverify.WebhookConfig{
    URL: "https://your-app.com/webhooks/billionverify",
    Events: []billionverify.WebhookEvent{
        billionverify.EventFileCompleted, // "file.completed"
        billionverify.EventFileFailed,    // "file.failed"
    },
})
if err != nil {
    log.Fatal(err)
}

fmt.Printf("Webhook ID: %s\n", webhook.ID)
fmt.Printf("Secret: %s\n", webhook.Secret) // Save this for signature verification!

// List webhooks
webhooks, err := client.ListWebhooks(context.Background())
if err != nil {
    log.Fatal(err)
}

for _, wh := range webhooks.Webhooks {
    fmt.Printf("ID: %s, URL: %s, Active: %t\n", wh.ID, wh.URL, wh.IsActive)
}

// Delete a webhook
err = client.DeleteWebhook(context.Background(), webhook.ID)
if err != nil {
    log.Fatal(err)
}

// Verify webhook signature in your handler
func webhookHandler(w http.ResponseWriter, r *http.Request) {
    body, _ := io.ReadAll(r.Body)
    signature := r.Header.Get("X-Webhook-Signature")
    timestamp := r.Header.Get("X-Webhook-Timestamp")

    // The signature is an HMAC-SHA256 over "<timestamp>.<body>". Deliveries older or
    // newer than 5 minutes are rejected (pass a tolerance to change that; negative turns it off).
    if !billionverify.VerifyWebhookSignatureWithTimestamp(string(body), signature, timestamp, "your-webhook-secret", 0) {
        http.Error(w, "Invalid signature", http.StatusUnauthorized)
        return
    }

    // Process the webhook payload
    var payload billionverify.WebhookPayload
    json.Unmarshal(body, &payload)

    switch payload.Event {
    case billionverify.EventFileCompleted:
        // Handle file.completed event
    case billionverify.EventFileFailed:
        // Handle file.failed event
    }

    w.WriteHeader(http.StatusOK)
}
```

## Error Handling

```go
import (
    "errors"
    billionverify "github.com/BillionVerify/billionverify-go"
)

result, err := client.Verify(context.Background(), "user@example.com", nil)
if err != nil {
    var authErr *billionverify.AuthenticationError
    var rateLimitErr *billionverify.RateLimitError
    var validationErr *billionverify.ValidationError
    var creditsErr *billionverify.InsufficientCreditsError
    var notFoundErr *billionverify.NotFoundError
    var timeoutErr *billionverify.TimeoutError

    switch {
    case errors.As(err, &authErr):
        log.Println("Invalid API key")
    case errors.As(err, &rateLimitErr):
        log.Printf("Rate limited. Retry after %d seconds", rateLimitErr.RetryAfter)
    case errors.As(err, &validationErr):
        log.Printf("Invalid input: %s", validationErr.Message)
    case errors.As(err, &creditsErr):
        log.Println("Not enough credits")
    case errors.As(err, &notFoundErr):
        log.Println("Resource not found")
    case errors.As(err, &timeoutErr):
        log.Println("Request timed out")
    default:
        log.Printf("Error: %v", err)
    }
}
```

## Context Support

All methods accept a `context.Context` for cancellation and timeouts:

```go
ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
defer cancel()

result, err := client.Verify(ctx, "user@example.com", nil)
```

## Status Constants

```go
billionverify.StatusValid      // "valid"
billionverify.StatusInvalid    // "invalid"
billionverify.StatusUnknown    // "unknown"
billionverify.StatusRisky      // "risky"
billionverify.StatusDisposable // "disposable"
billionverify.StatusCatchall   // "catchall"
billionverify.StatusRole       // "role"
```

## Job Status Constants

```go
billionverify.JobStatusPreparing            // "preparing"
billionverify.JobStatusPending              // "pending"
billionverify.JobStatusRunning              // "running"
billionverify.JobStatusProcessing           // "processing"
billionverify.JobStatusMerging              // "merging"
billionverify.JobStatusPaused               // "paused"
billionverify.JobStatusCompleted            // "completed"
billionverify.JobStatusCompletedWithWarning // "completed_with_warning"
billionverify.JobStatusFailed               // "failed"
billionverify.JobStatusCancelled            // "cancelled"

status.Status.IsSettled() // ended (completed, completed_with_warning, failed, cancelled) or paused
```

## Webhook Event Constants

```go
billionverify.EventFileCompleted // "file.completed"
billionverify.EventFileFailed    // "file.failed"
```

## Examples

See the [examples](./examples) directory for complete working examples:

- [Basic Usage](./examples/basic/main.go) - Single verification, batch verification, credits, health check
- [File Upload](./examples/file_upload/main.go) - Async file verification with status polling
- [Webhooks](./examples/webhooks/main.go) - Webhook management and signature verification

## License

MIT
