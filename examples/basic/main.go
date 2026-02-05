// Package main demonstrates basic usage of the EmailVerify Go SDK.
// This example shows single email verification, batch verification,
// getting credits, and health check.
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"time"

	emailverify "github.com/emailverify-ai/go-sdk"
)

func main() {
	// Get API key from environment variable
	apiKey := os.Getenv("EMAILVERIFY_API_KEY")
	if apiKey == "" {
		log.Fatal("EMAILVERIFY_API_KEY environment variable is required")
	}

	// Create a new client
	client, err := emailverify.NewClient(emailverify.Config{
		APIKey:  apiKey,
		Timeout: 30 * time.Second,
		Retries: 3,
	})
	if err != nil {
		log.Fatalf("Failed to create client: %v", err)
	}

	ctx := context.Background()

	// 1. Health Check (no authentication required)
	fmt.Println("=== Health Check ===")
	health, err := client.Health(ctx)
	if err != nil {
		log.Printf("Health check failed: %v", err)
	} else {
		fmt.Printf("API Status: %s\n", health.Status)
		fmt.Printf("Server Time: %d\n", health.Time)
	}
	fmt.Println()

	// 2. Get Credits
	fmt.Println("=== Credits ===")
	credits, err := client.GetCredits(ctx)
	if err != nil {
		handleError(err)
	} else {
		fmt.Printf("Account ID: %s\n", credits.AccountID)
		fmt.Printf("API Key Name: %s\n", credits.APIKeyName)
		fmt.Printf("Credits Balance: %d\n", credits.CreditsBalance)
		fmt.Printf("Credits Consumed: %d\n", credits.CreditsConsumed)
		fmt.Printf("Last Updated: %s\n", credits.LastUpdated)
	}
	fmt.Println()

	// 3. Single Email Verification
	fmt.Println("=== Single Email Verification ===")
	result, err := client.Verify(ctx, "test@example.com", &emailverify.VerifyOptions{
		CheckSMTP: true, // Perform SMTP verification
	})
	if err != nil {
		handleError(err)
	} else {
		fmt.Printf("Email: %s\n", result.Email)
		fmt.Printf("Status: %s\n", result.Status)
		fmt.Printf("Score: %.2f\n", result.Score)
		fmt.Printf("Is Deliverable: %t\n", result.IsDeliverable)
		fmt.Printf("Is Disposable: %t\n", result.IsDisposable)
		fmt.Printf("Is Catchall: %t\n", result.IsCatchall)
		fmt.Printf("Is Role: %t\n", result.IsRole)
		fmt.Printf("Is Free: %t\n", result.IsFree)
		fmt.Printf("Domain: %s\n", result.Domain)
		fmt.Printf("SMTP Check: %t\n", result.SMTPCheck)
		fmt.Printf("Credits Used: %d\n", result.CreditsUsed)
		if result.Reason != "" {
			fmt.Printf("Reason: %s\n", result.Reason)
		}
		if result.Suggestion != "" {
			fmt.Printf("Suggestion: %s\n", result.Suggestion)
		}
		if result.DomainReputation != nil {
			fmt.Printf("Domain Reputation - Listed: %t\n", result.DomainReputation.IsListed)
		}
	}
	fmt.Println()

	// 4. Batch Verification (synchronous, max 50 emails)
	fmt.Println("=== Batch Verification ===")
	emails := []string{
		"user1@example.com",
		"user2@gmail.com",
		"admin@company.org",
		"info@business.net",
		"test@disposable.email",
	}

	batchResult, err := client.VerifyBatch(ctx, emails, &emailverify.BatchVerifyOptions{
		CheckSMTP: true,
	})
	if err != nil {
		handleError(err)
	} else {
		fmt.Printf("Total Emails: %d\n", batchResult.TotalEmails)
		fmt.Printf("Valid Emails: %d\n", batchResult.ValidEmails)
		fmt.Printf("Invalid Emails: %d\n", batchResult.InvalidEmails)
		fmt.Printf("Credits Used: %d\n", batchResult.CreditsUsed)
		fmt.Printf("Process Time: %d ms\n", batchResult.ProcessTime)
		fmt.Println()
		fmt.Println("Individual Results:")
		for _, item := range batchResult.Results {
			fmt.Printf("  %s: status=%s, deliverable=%t, disposable=%t, catchall=%t\n",
				item.Email, item.Status, item.IsDeliverable, item.IsDisposable, item.IsCatchall)
		}
	}
	fmt.Println()

	// 5. Verify with context timeout
	fmt.Println("=== Verification with Context Timeout ===")
	ctxWithTimeout, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	result2, err := client.Verify(ctxWithTimeout, "another@example.com", nil)
	if err != nil {
		handleError(err)
	} else {
		fmt.Printf("Email: %s, Status: %s\n", result2.Email, result2.Status)
	}
	fmt.Println()

	// 6. Demonstrate status checking
	fmt.Println("=== Status Constants ===")
	fmt.Printf("Valid status: %s\n", emailverify.StatusValid)
	fmt.Printf("Invalid status: %s\n", emailverify.StatusInvalid)
	fmt.Printf("Unknown status: %s\n", emailverify.StatusUnknown)
	fmt.Printf("Risky status: %s\n", emailverify.StatusRisky)
	fmt.Printf("Disposable status: %s\n", emailverify.StatusDisposable)
	fmt.Printf("Catchall status: %s\n", emailverify.StatusCatchall)
	fmt.Printf("Role status: %s\n", emailverify.StatusRole)
}

// handleError demonstrates proper error handling for the SDK
func handleError(err error) {
	var authErr *emailverify.AuthenticationError
	var rateLimitErr *emailverify.RateLimitError
	var validationErr *emailverify.ValidationError
	var creditsErr *emailverify.InsufficientCreditsError
	var notFoundErr *emailverify.NotFoundError
	var timeoutErr *emailverify.TimeoutError

	switch {
	case errors.As(err, &authErr):
		log.Printf("Authentication error: Invalid API key")
	case errors.As(err, &rateLimitErr):
		log.Printf("Rate limit exceeded. Retry after %d seconds", rateLimitErr.RetryAfter)
	case errors.As(err, &validationErr):
		log.Printf("Validation error: %s (details: %s)", validationErr.Message, validationErr.Details)
	case errors.As(err, &creditsErr):
		log.Printf("Insufficient credits: %s", creditsErr.Message)
	case errors.As(err, &notFoundErr):
		log.Printf("Resource not found: %s", notFoundErr.Message)
	case errors.As(err, &timeoutErr):
		log.Printf("Request timed out: %s", timeoutErr.Message)
	default:
		log.Printf("Unexpected error: %v", err)
	}
}
