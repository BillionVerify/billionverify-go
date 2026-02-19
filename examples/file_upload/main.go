// Package main demonstrates file upload verification using the BillionVerify Go SDK.
// This example shows how to upload a file for async verification, poll for status
// with long-polling, and download results with filters.
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"time"

	billionverify "github.com/BillionVerify/go-sdk"
)

func main() {
	// Get API key from environment variable
	apiKey := os.Getenv("EMAILVERIFY_API_KEY")
	if apiKey == "" {
		log.Fatal("EMAILVERIFY_API_KEY environment variable is required")
	}

	// Create a new client with longer timeout for file operations
	client, err := billionverify.NewClient(billionverify.Config{
		APIKey:  apiKey,
		Timeout: 60 * time.Second, // Longer timeout for file uploads
		Retries: 3,
	})
	if err != nil {
		log.Fatalf("Failed to create client: %v", err)
	}

	ctx := context.Background()

	// Check if a file path was provided
	if len(os.Args) < 2 {
		fmt.Println("Usage: go run main.go <path-to-csv-file>")
		fmt.Println()
		fmt.Println("The CSV file should contain email addresses.")
		fmt.Println("Example CSV format:")
		fmt.Println("  email")
		fmt.Println("  user1@example.com")
		fmt.Println("  user2@gmail.com")
		os.Exit(1)
	}

	filePath := os.Args[1]

	// Verify file exists
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		log.Fatalf("File not found: %s", filePath)
	}

	// 1. Upload File for Verification
	fmt.Println("=== Uploading File ===")
	uploadResponse, err := client.UploadFile(ctx, filePath, &billionverify.FileUploadOptions{
		CheckSMTP:        true,    // Perform SMTP verification
		EmailColumn:      "email", // Column name containing emails (optional)
		PreserveOriginal: true,    // Keep original columns in results
	})
	if err != nil {
		handleError(err)
		os.Exit(1)
	}

	fmt.Printf("Task ID: %s\n", uploadResponse.TaskID)
	fmt.Printf("Status: %s\n", uploadResponse.Status)
	fmt.Printf("File Name: %s\n", uploadResponse.FileName)
	fmt.Printf("File Size: %d bytes\n", uploadResponse.FileSize)
	if uploadResponse.EstimatedCount > 0 {
		fmt.Printf("Estimated Emails: %d\n", uploadResponse.EstimatedCount)
	}
	if uploadResponse.UniqueEmails > 0 {
		fmt.Printf("Unique Emails: %d\n", uploadResponse.UniqueEmails)
	}
	fmt.Printf("Created At: %s\n", uploadResponse.CreatedAt)
	fmt.Println()

	jobID := uploadResponse.TaskID

	// 2. Poll for Status with Long-Polling
	fmt.Println("=== Checking Job Status (with long-polling) ===")

	// Get status with long-polling (wait up to 30 seconds for status change)
	status, err := client.GetFileJobStatus(ctx, jobID, &billionverify.FileJobStatusOptions{
		Timeout: 30, // Wait up to 30 seconds for status change
	})
	if err != nil {
		handleError(err)
		os.Exit(1)
	}

	printJobStatus(status)
	fmt.Println()

	// 3. Wait for Job Completion
	fmt.Println("=== Waiting for Job Completion ===")
	fmt.Println("This may take a while depending on the file size...")

	completed, err := client.WaitForFileJobCompletion(
		ctx,
		jobID,
		10*time.Second, // Poll every 10 seconds
		30*time.Minute, // Wait up to 30 minutes
	)
	if err != nil {
		handleError(err)
		os.Exit(1)
	}

	fmt.Println()
	fmt.Println("Job completed!")
	printJobStatus(completed)
	fmt.Println()

	// Check if job failed
	if completed.Status == billionverify.JobStatusFailed {
		log.Fatal("Job failed. Please check your file and try again.")
	}

	// 4. Download All Results
	fmt.Println("=== Downloading All Results ===")
	allResults, err := client.GetFileJobResults(ctx, jobID, nil)
	if err != nil {
		handleError(err)
	} else {
		outputFile := "all_results.csv"
		if err := os.WriteFile(outputFile, allResults, 0644); err != nil {
			log.Printf("Failed to save results: %v", err)
		} else {
			fmt.Printf("All results saved to: %s (%d bytes)\n", outputFile, len(allResults))
		}
	}
	fmt.Println()

	// 5. Download Only Valid Emails
	fmt.Println("=== Downloading Valid Emails Only ===")
	validOnly := true
	validResults, err := client.GetFileJobResults(ctx, jobID, &billionverify.FileResultsOptions{
		Valid: &validOnly,
	})
	if err != nil {
		handleError(err)
	} else {
		outputFile := "valid_results.csv"
		if err := os.WriteFile(outputFile, validResults, 0644); err != nil {
			log.Printf("Failed to save results: %v", err)
		} else {
			fmt.Printf("Valid emails saved to: %s (%d bytes)\n", outputFile, len(validResults))
		}
	}
	fmt.Println()

	// 6. Download Invalid and Disposable Emails
	fmt.Println("=== Downloading Invalid and Disposable Emails ===")
	invalidOnly := true
	disposableOnly := true
	badResults, err := client.GetFileJobResults(ctx, jobID, &billionverify.FileResultsOptions{
		Invalid:    &invalidOnly,
		Disposable: &disposableOnly,
	})
	if err != nil {
		handleError(err)
	} else {
		outputFile := "invalid_disposable_results.csv"
		if err := os.WriteFile(outputFile, badResults, 0644); err != nil {
			log.Printf("Failed to save results: %v", err)
		} else {
			fmt.Printf("Invalid/disposable emails saved to: %s (%d bytes)\n", outputFile, len(badResults))
		}
	}
	fmt.Println()

	// 7. Download Catchall and Risky Emails
	fmt.Println("=== Downloading Catchall and Risky Emails ===")
	catchallOnly := true
	riskyOnly := true
	riskyResults, err := client.GetFileJobResults(ctx, jobID, &billionverify.FileResultsOptions{
		Catchall: &catchallOnly,
		Risky:    &riskyOnly,
	})
	if err != nil {
		handleError(err)
	} else {
		outputFile := "catchall_risky_results.csv"
		if err := os.WriteFile(outputFile, riskyResults, 0644); err != nil {
			log.Printf("Failed to save results: %v", err)
		} else {
			fmt.Printf("Catchall/risky emails saved to: %s (%d bytes)\n", outputFile, len(riskyResults))
		}
	}

	fmt.Println()
	fmt.Println("=== Summary ===")
	fmt.Printf("Total Emails: %d\n", completed.TotalEmails)
	fmt.Printf("Valid: %d\n", completed.ValidEmails)
	fmt.Printf("Invalid: %d\n", completed.InvalidEmails)
	fmt.Printf("Unknown: %d\n", completed.UnknownEmails)
	fmt.Printf("Catchall: %d\n", completed.CatchallEmails)
	fmt.Printf("Disposable: %d\n", completed.DisposableEmails)
	fmt.Printf("Role: %d\n", completed.RoleEmails)
	fmt.Printf("Credits Used: %d\n", completed.CreditsUsed)
	if completed.ProcessTimeSeconds > 0 {
		fmt.Printf("Process Time: %.2f seconds\n", completed.ProcessTimeSeconds)
	}
}

// printJobStatus prints the current job status
func printJobStatus(status *billionverify.FileJobResponse) {
	fmt.Printf("Job ID: %s\n", status.JobID)
	fmt.Printf("Status: %s\n", status.Status)
	fmt.Printf("Progress: %d%%\n", status.ProgressPercent)
	fmt.Printf("Processed: %d / %d\n", status.ProcessedEmails, status.TotalEmails)

	if status.Status == billionverify.JobStatusCompleted || status.Status == billionverify.JobStatusFailed {
		fmt.Printf("Valid: %d\n", status.ValidEmails)
		fmt.Printf("Invalid: %d\n", status.InvalidEmails)
		fmt.Printf("Unknown: %d\n", status.UnknownEmails)
		fmt.Printf("Credits Used: %d\n", status.CreditsUsed)
	}
}

// handleError demonstrates proper error handling for the SDK
func handleError(err error) {
	var authErr *billionverify.AuthenticationError
	var rateLimitErr *billionverify.RateLimitError
	var validationErr *billionverify.ValidationError
	var creditsErr *billionverify.InsufficientCreditsError
	var notFoundErr *billionverify.NotFoundError
	var timeoutErr *billionverify.TimeoutError

	switch {
	case errors.As(err, &authErr):
		log.Printf("Authentication error: Invalid API key")
	case errors.As(err, &rateLimitErr):
		log.Printf("Rate limit exceeded. Retry after %d seconds", rateLimitErr.RetryAfter)
	case errors.As(err, &validationErr):
		log.Printf("Validation error: %s", validationErr.Message)
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
