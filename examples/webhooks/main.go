// Package main demonstrates webhook management using the EmailVerify Go SDK.
// This example shows how to create, list, and delete webhooks, as well as
// how to verify webhook signatures in an HTTP handler.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	emailverify "github.com/emailverify-ai/go-sdk"
)

// Store webhook secret globally (in production, use secure storage)
var webhookSecret string

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

	// 1. List Existing Webhooks
	fmt.Println("=== Listing Existing Webhooks ===")
	webhooks, err := client.ListWebhooks(ctx)
	if err != nil {
		handleError(err)
	} else {
		fmt.Printf("Total webhooks: %d\n", webhooks.Total)
		for _, wh := range webhooks.Webhooks {
			fmt.Printf("  ID: %s\n", wh.ID)
			fmt.Printf("  URL: %s\n", wh.URL)
			fmt.Printf("  Events: %v\n", wh.Events)
			fmt.Printf("  Active: %t\n", wh.IsActive)
			fmt.Printf("  Created: %s\n", wh.CreatedAt)
			fmt.Println()
		}
	}
	fmt.Println()

	// 2. Create a New Webhook
	fmt.Println("=== Creating New Webhook ===")

	// Get webhook URL from environment or use a placeholder
	webhookURL := os.Getenv("WEBHOOK_URL")
	if webhookURL == "" {
		webhookURL = "https://your-app.com/webhooks/emailverify"
		fmt.Printf("Note: Using placeholder URL. Set WEBHOOK_URL environment variable for real usage.\n")
	}

	webhook, err := client.CreateWebhook(ctx, emailverify.WebhookConfig{
		URL: webhookURL,
		Events: []emailverify.WebhookEvent{
			emailverify.EventFileCompleted, // "file.completed"
			emailverify.EventFileFailed,    // "file.failed"
		},
	})
	if err != nil {
		handleError(err)
	} else {
		fmt.Printf("Webhook created successfully!\n")
		fmt.Printf("ID: %s\n", webhook.ID)
		fmt.Printf("URL: %s\n", webhook.URL)
		fmt.Printf("Events: %v\n", webhook.Events)
		fmt.Printf("Active: %t\n", webhook.IsActive)
		fmt.Printf("Created: %s\n", webhook.CreatedAt)

		// IMPORTANT: Save this secret! It's only returned once during creation
		if webhook.Secret != "" {
			webhookSecret = webhook.Secret
			fmt.Printf("Secret: %s\n", webhook.Secret)
			fmt.Println()
			fmt.Println("IMPORTANT: Save this secret! It won't be shown again.")
			fmt.Println("Use it to verify webhook signatures in your handler.")
		}
	}
	fmt.Println()

	// 3. List Webhooks Again (verify creation)
	fmt.Println("=== Verifying Webhook Creation ===")
	webhooksAfter, err := client.ListWebhooks(ctx)
	if err != nil {
		handleError(err)
	} else {
		fmt.Printf("Total webhooks after creation: %d\n", webhooksAfter.Total)
	}
	fmt.Println()

	// 4. Delete the Webhook (cleanup)
	if webhook != nil && webhook.ID != "" {
		fmt.Println("=== Deleting Webhook ===")
		err = client.DeleteWebhook(ctx, webhook.ID)
		if err != nil {
			handleError(err)
		} else {
			fmt.Printf("Webhook %s deleted successfully\n", webhook.ID)
		}
		fmt.Println()
	}

	// 5. Demonstrate Webhook Event Constants
	fmt.Println("=== Webhook Event Constants ===")
	fmt.Printf("File Completed: %s\n", emailverify.EventFileCompleted)
	fmt.Printf("File Failed: %s\n", emailverify.EventFileFailed)
	fmt.Println()

	// 6. Start Webhook Server (optional demo)
	fmt.Println("=== Webhook Handler Example ===")
	fmt.Println("To test webhook handling, run this server and configure your webhook URL.")
	fmt.Println()
	fmt.Println("Example webhook handler code:")
	fmt.Print(`
http.HandleFunc("/webhooks/emailverify", webhookHandler)
http.ListenAndServe(":8080", nil)
`)
	fmt.Println()

	// Optionally start the server
	if os.Getenv("START_SERVER") == "true" {
		fmt.Println("Starting webhook server on :8080...")
		http.HandleFunc("/webhooks/emailverify", webhookHandler)
		log.Fatal(http.ListenAndServe(":8080", nil))
	}
}

// webhookHandler handles incoming webhook requests from EmailVerify
func webhookHandler(w http.ResponseWriter, r *http.Request) {
	// Only accept POST requests
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Read the request body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		log.Printf("Failed to read request body: %v", err)
		http.Error(w, "Failed to read body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	// Get the signature from headers
	signature := r.Header.Get("X-EmailVerify-Signature")
	if signature == "" {
		log.Println("Missing X-EmailVerify-Signature header")
		http.Error(w, "Missing signature", http.StatusUnauthorized)
		return
	}

	// Verify the webhook signature
	if !emailverify.VerifyWebhookSignature(string(body), signature, webhookSecret) {
		log.Println("Invalid webhook signature")
		http.Error(w, "Invalid signature", http.StatusUnauthorized)
		return
	}

	// Parse the webhook payload
	var payload emailverify.WebhookPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		log.Printf("Failed to parse webhook payload: %v", err)
		http.Error(w, "Invalid payload", http.StatusBadRequest)
		return
	}

	// Handle the event
	log.Printf("Received webhook event: %s at %s", payload.Event, payload.Timestamp)

	switch payload.Event {
	case emailverify.EventFileCompleted:
		handleFileCompleted(payload.Data)

	case emailverify.EventFileFailed:
		handleFileFailed(payload.Data)

	default:
		log.Printf("Unknown event type: %s", payload.Event)
	}

	// Respond with 200 OK to acknowledge receipt
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status": "received"}`))
}

// handleFileCompleted processes file.completed events
func handleFileCompleted(data map[string]interface{}) {
	log.Println("File verification completed!")

	// Extract job information from the data
	if jobID, ok := data["job_id"].(string); ok {
		log.Printf("Job ID: %s", jobID)
	}

	if totalEmails, ok := data["total_emails"].(float64); ok {
		log.Printf("Total emails processed: %.0f", totalEmails)
	}

	if validEmails, ok := data["valid_emails"].(float64); ok {
		log.Printf("Valid emails: %.0f", validEmails)
	}

	if invalidEmails, ok := data["invalid_emails"].(float64); ok {
		log.Printf("Invalid emails: %.0f", invalidEmails)
	}

	if downloadURL, ok := data["download_url"].(string); ok {
		log.Printf("Download URL: %s", downloadURL)
	}

	// Here you would typically:
	// 1. Download the results using client.GetFileJobResults()
	// 2. Process the verified emails
	// 3. Update your database
	// 4. Send notifications to users
}

// handleFileFailed processes file.failed events
func handleFileFailed(data map[string]interface{}) {
	log.Println("File verification failed!")

	// Extract error information from the data
	if jobID, ok := data["job_id"].(string); ok {
		log.Printf("Job ID: %s", jobID)
	}

	if errorMsg, ok := data["error"].(string); ok {
		log.Printf("Error: %s", errorMsg)
	}

	// Here you would typically:
	// 1. Log the failure
	// 2. Notify the user
	// 3. Retry the job or take corrective action
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
