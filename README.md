# EmailVerify Go SDK

Official EmailVerify Go SDK for email verification.

**Documentation:** https://emailverify.ai/docs

## Installation

```bash
go get github.com/emailverify-ai/go-sdk
```

## Quick Start

```go
package main

import (
    "context"
    "fmt"
    "log"

    emailverify "github.com/emailverify-ai/go-sdk"
)

func main() {
    client, err := emailverify.NewClient(emailverify.Config{
        APIKey: "your-api-key",
    })
    if err != nil {
        log.Fatal(err)
    }

    result, err := client.Verify(context.Background(), "user@example.com", nil)
    if err != nil {
        log.Fatal(err)
    }

    fmt.Printf("Status: %s\n", result.Status)
}
```

## Configuration

```go
client, err := emailverify.NewClient(emailverify.Config{
    APIKey:  "your-api-key",                           // Required
    BaseURL: "https://api.emailverify.ai/v1",       // Optional
    Timeout: 30 * time.Second,                         // Optional (default: 30s)
    Retries: 3,                                        // Optional (default: 3)
})
```

## Single Email Verification

```go
result, err := client.Verify(context.Background(), "user@example.com", &emailverify.VerifyOptions{
    SMTPCheck: true,  // Optional: Perform SMTP verification (default: true)
    Timeout:   5000,  // Optional: Verification timeout in ms
})
if err != nil {
    log.Fatal(err)
}

fmt.Printf("Email: %s\n", result.Email)
fmt.Printf("Status: %s\n", result.Status)  // valid, invalid, unknown, accept_all
fmt.Printf("Score: %.2f\n", result.Score)
fmt.Printf("Deliverable: %t\n", result.Result.Deliverable)
fmt.Printf("Disposable: %t\n", result.Result.Disposable)
```

## Bulk Email Verification

```go
// Submit a bulk verification job
job, err := client.VerifyBulk(context.Background(), []string{
    "user1@example.com",
    "user2@example.com",
    "user3@example.com",
}, &emailverify.BulkVerifyOptions{
    SMTPCheck:  true,
    WebhookURL: "https://your-app.com/webhooks/emailverify",
})
if err != nil {
    log.Fatal(err)
}

fmt.Printf("Job ID: %s\n", job.JobID)

// Check job status
status, err := client.GetBulkJobStatus(context.Background(), job.JobID)
if err != nil {
    log.Fatal(err)
}
fmt.Printf("Progress: %d%%\n", *status.ProgressPercent)

// Wait for completion (polling)
completed, err := client.WaitForBulkJobCompletion(
    context.Background(),
    job.JobID,
    5*time.Second,   // poll interval
    10*time.Minute,  // max wait
)
if err != nil {
    log.Fatal(err)
}

// Get results
results, err := client.GetBulkJobResults(context.Background(), job.JobID, &emailverify.BulkResultsOptions{
    Limit:  100,
    Offset: 0,
    Status: emailverify.StatusValid, // Optional: filter by status
})
if err != nil {
    log.Fatal(err)
}

for _, item := range results.Results {
    fmt.Printf("%s: %s\n", item.Email, item.Status)
}
```

## Credits

```go
credits, err := client.GetCredits(context.Background())
if err != nil {
    log.Fatal(err)
}

fmt.Printf("Available: %d\n", credits.Available)
fmt.Printf("Plan: %s\n", credits.Plan)
fmt.Printf("Rate Limit Remaining: %d\n", credits.RateLimit.Remaining)
```

## Webhooks

```go
// Create a webhook
webhook, err := client.CreateWebhook(context.Background(), emailverify.WebhookConfig{
    URL:    "https://your-app.com/webhooks/emailverify",
    Events: []emailverify.WebhookEvent{
        emailverify.EventVerificationCompleted,
        emailverify.EventBulkCompleted,
    },
    Secret: "your-webhook-secret",
})
if err != nil {
    log.Fatal(err)
}

// List webhooks
webhooks, err := client.ListWebhooks(context.Background())
if err != nil {
    log.Fatal(err)
}

// Delete a webhook
err = client.DeleteWebhook(context.Background(), webhook.ID)
if err != nil {
    log.Fatal(err)
}

// Verify webhook signature
isValid := emailverify.VerifyWebhookSignature(rawBody, signature, "your-webhook-secret")
```

## Error Handling

```go
import (
    "errors"
    emailverify "github.com/emailverify-ai/go-sdk"
)

result, err := client.Verify(context.Background(), "user@example.com", nil)
if err != nil {
    var authErr *emailverify.AuthenticationError
    var rateLimitErr *emailverify.RateLimitError
    var validationErr *emailverify.ValidationError
    var creditsErr *emailverify.InsufficientCreditsError
    var notFoundErr *emailverify.NotFoundError
    var timeoutErr *emailverify.TimeoutError

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
emailverify.StatusValid     // "valid"
emailverify.StatusInvalid   // "invalid"
emailverify.StatusUnknown   // "unknown"
emailverify.StatusAcceptAll // "accept_all"
```

## License

MIT
