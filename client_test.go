package billionverify

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNewClient(t *testing.T) {
	t.Run("requires API key", func(t *testing.T) {
		_, err := NewClient(Config{APIKey: ""})
		if err == nil {
			t.Error("expected error for empty API key")
		}
		if _, ok := err.(*AuthenticationError); !ok {
			t.Errorf("expected AuthenticationError, got %T", err)
		}
	})

	t.Run("creates client with default options", func(t *testing.T) {
		client, err := NewClient(Config{APIKey: "test-key"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if client.apiKey != "test-key" {
			t.Errorf("expected API key 'test-key', got %s", client.apiKey)
		}
		if client.baseURL != defaultBaseURL {
			t.Errorf("expected base URL %s, got %s", defaultBaseURL, client.baseURL)
		}
	})

	t.Run("creates client with custom options", func(t *testing.T) {
		client, err := NewClient(Config{
			APIKey:  "test-key",
			BaseURL: "https://custom.api.com",
			Timeout: 60 * time.Second,
			Retries: 5,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if client.baseURL != "https://custom.api.com" {
			t.Errorf("expected custom base URL, got %s", client.baseURL)
		}
		if client.retries != 5 {
			t.Errorf("expected 5 retries, got %d", client.retries)
		}
	})
}

func TestHealth(t *testing.T) {
	t.Run("checks health successfully", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/health" {
				t.Errorf("expected path /health, got %s", r.URL.Path)
			}
			if r.Method != http.MethodGet {
				t.Errorf("expected GET method, got %s", r.Method)
			}
			// Health endpoint should not require API key
			if r.Header.Get("EV-API-KEY") != "" {
				t.Error("health endpoint should not send API key")
			}

			response := HealthResponse{
				Status: "ok",
				Time:   1705319400,
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(response)
		}))
		defer server.Close()

		client, _ := NewClient(Config{APIKey: "test-key", BaseURL: server.URL})
		result, err := client.Health(context.Background())

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Status != "ok" {
			t.Errorf("expected status ok, got %s", result.Status)
		}
	})
}

func TestVerify(t *testing.T) {
	t.Run("verifies email successfully", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/v1/verify/single" {
				t.Errorf("expected path /v1/verify/single, got %s", r.URL.Path)
			}
			if r.Method != http.MethodPost {
				t.Errorf("expected POST method, got %s", r.Method)
			}
			if r.Header.Get("EV-API-KEY") != "test-key" {
				t.Errorf("expected EV-API-KEY header")
			}

			// Verify request body uses check_smtp not smtp_check
			var reqBody map[string]interface{}
			json.NewDecoder(r.Body).Decode(&reqBody)
			if _, ok := reqBody["check_smtp"]; !ok {
				t.Error("expected check_smtp in request body")
			}

			response := map[string]interface{}{
				"success": true,
				"code":    "0",
				"message": "Success",
				"data": map[string]interface{}{
					"email":          "test@example.com",
					"status":         "valid",
					"score":          0.95,
					"is_deliverable": true,
					"is_disposable":  false,
					"is_catchall":    false,
					"is_role":        false,
					"is_free":        false,
					"domain":         "example.com",
					"domain_age":     10,
					"mx_records":     []string{"mail.example.com"},
					"smtp_check":     true,
					"reason":         "accepted",
					"response_time":  250,
					"credits_used":   1,
				},
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(response)
		}))
		defer server.Close()

		client, _ := NewClient(Config{APIKey: "test-key", BaseURL: server.URL})
		result, err := client.Verify(context.Background(), "test@example.com", nil)

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Email != "test@example.com" {
			t.Errorf("expected email test@example.com, got %s", result.Email)
		}
		if result.Status != StatusValid {
			t.Errorf("expected status valid, got %s", result.Status)
		}
		if result.Score != 0.95 {
			t.Errorf("expected score 0.95, got %f", result.Score)
		}
		if !result.IsDeliverable {
			t.Error("expected is_deliverable to be true")
		}
		if result.Domain != "example.com" {
			t.Errorf("expected domain example.com, got %s", result.Domain)
		}
	})

	t.Run("handles authentication error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"error": map[string]string{
					"code":    "INVALID_API_KEY",
					"message": "Invalid API key",
				},
			})
		}))
		defer server.Close()

		client, _ := NewClient(Config{APIKey: "invalid-key", BaseURL: server.URL})
		_, err := client.Verify(context.Background(), "test@example.com", nil)

		if err == nil {
			t.Fatal("expected error")
		}
		if _, ok := err.(*AuthenticationError); !ok {
			t.Errorf("expected AuthenticationError, got %T", err)
		}
	})

	t.Run("handles validation error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"error": map[string]string{
					"code":    "INVALID_EMAIL",
					"message": "Invalid email format",
				},
			})
		}))
		defer server.Close()

		client, _ := NewClient(Config{APIKey: "test-key", BaseURL: server.URL})
		_, err := client.Verify(context.Background(), "invalid", nil)

		if err == nil {
			t.Fatal("expected error")
		}
		if _, ok := err.(*ValidationError); !ok {
			t.Errorf("expected ValidationError, got %T", err)
		}
	})

	t.Run("handles insufficient credits error with HTTP 402", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusPaymentRequired)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"error": map[string]string{
					"code":    "INSUFFICIENT_CREDITS",
					"message": "Not enough credits",
				},
			})
		}))
		defer server.Close()

		client, _ := NewClient(Config{APIKey: "test-key", BaseURL: server.URL})
		_, err := client.Verify(context.Background(), "test@example.com", nil)

		if err == nil {
			t.Fatal("expected error")
		}
		if _, ok := err.(*InsufficientCreditsError); !ok {
			t.Errorf("expected InsufficientCreditsError, got %T", err)
		}
	})
}

func TestVerifyBatch(t *testing.T) {
	t.Run("verifies batch successfully", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/v1/verify/bulk" {
				t.Errorf("expected path /v1/verify/bulk, got %s", r.URL.Path)
			}

			response := map[string]interface{}{
				"success": true,
				"code":    "0",
				"message": "Success",
				"data": map[string]interface{}{
					"results": []map[string]interface{}{
						{
							"email":          "user1@example.com",
							"status":         "valid",
							"score":          0.95,
							"is_deliverable": true,
							"credits_used":   1,
						},
						{
							"email":          "user2@example.com",
							"status":         "invalid",
							"score":          0.0,
							"is_deliverable": false,
							"credits_used":   0,
						},
					},
					"total_emails":   2,
					"valid_emails":   1,
					"invalid_emails": 1,
					"credits_used":   1,
					"process_time":   1500,
				},
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(response)
		}))
		defer server.Close()

		client, _ := NewClient(Config{APIKey: "test-key", BaseURL: server.URL})
		result, err := client.VerifyBatch(context.Background(), []string{
			"user1@example.com",
			"user2@example.com",
		}, nil)

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result.Results) != 2 {
			t.Errorf("expected 2 results, got %d", len(result.Results))
		}
		if result.TotalEmails != 2 {
			t.Errorf("expected total 2, got %d", result.TotalEmails)
		}
		if result.ValidEmails != 1 {
			t.Errorf("expected 1 valid email, got %d", result.ValidEmails)
		}
	})

	t.Run("rejects more than 50 emails", func(t *testing.T) {
		client, _ := NewClient(Config{APIKey: "test-key"})
		emails := make([]string, 51)
		for i := range emails {
			emails[i] = "test@example.com"
		}

		_, err := client.VerifyBatch(context.Background(), emails, nil)

		if err == nil {
			t.Fatal("expected error")
		}
		if _, ok := err.(*ValidationError); !ok {
			t.Errorf("expected ValidationError, got %T", err)
		}
	})
}

func TestGetFileJobStatus(t *testing.T) {
	t.Run("gets job status successfully", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/v1/verify/file/job_123" {
				t.Errorf("expected path /v1/verify/file/job_123, got %s", r.URL.Path)
			}

			response := map[string]interface{}{
				"success": true,
				"code":    "0",
				"message": "Success",
				"data": map[string]interface{}{
					"job_id":           "job_123",
					"status":           "processing",
					"file_name":        "emails.csv",
					"total_emails":     100,
					"processed_emails": 50,
					"progress_percent": 50,
					"valid_emails":     40,
					"invalid_emails":   5,
					"unknown_emails":   5,
					"role_emails":      0,
					"catchall_emails":  0,
					"credits_used":     50,
					"created_at":       "2026-02-04T10:30:00Z",
				},
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(response)
		}))
		defer server.Close()

		client, _ := NewClient(Config{APIKey: "test-key", BaseURL: server.URL})
		result, err := client.GetFileJobStatus(context.Background(), "job_123", nil)

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.JobID != "job_123" {
			t.Errorf("expected job ID job_123, got %s", result.JobID)
		}
		if result.ProgressPercent != 50 {
			t.Errorf("expected progress 50, got %d", result.ProgressPercent)
		}
	})

	t.Run("supports long-polling timeout parameter", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			timeout := r.URL.Query().Get("timeout")
			if timeout != "30" {
				t.Errorf("expected timeout=30, got %s", timeout)
			}

			response := map[string]interface{}{
				"success": true,
				"code":    "0",
				"message": "Success",
				"data": map[string]interface{}{
					"job_id":           "job_123",
					"status":           "completed",
					"total_emails":     100,
					"processed_emails": 100,
					"progress_percent": 100,
					"created_at":       "2026-02-04T10:30:00Z",
					"completed_at":     "2026-02-04T10:32:00Z",
				},
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(response)
		}))
		defer server.Close()

		client, _ := NewClient(Config{APIKey: "test-key", BaseURL: server.URL})
		result, err := client.GetFileJobStatus(context.Background(), "job_123", &FileJobStatusOptions{Timeout: 30})

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Status != JobStatusCompleted {
			t.Errorf("expected completed status, got %s", result.Status)
		}
	})
}

func TestGetCredits(t *testing.T) {
	t.Run("gets credits successfully", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/v1/credits" {
				t.Errorf("expected path /v1/credits, got %s", r.URL.Path)
			}

			response := map[string]interface{}{
				"success": true,
				"code":    "0",
				"message": "Success",
				"data": map[string]interface{}{
					"account_id":       "abc123",
					"api_key_id":       "key_xyz",
					"api_key_name":     "Default API Key",
					"credits_balance":  9500,
					"credits_consumed": 500,
					"credits_added":    10000,
					"last_updated":     "2026-02-04T10:30:00Z",
				},
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(response)
		}))
		defer server.Close()

		client, _ := NewClient(Config{APIKey: "test-key", BaseURL: server.URL})
		result, err := client.GetCredits(context.Background())

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.CreditsBalance != 9500 {
			t.Errorf("expected credits_balance 9500, got %d", result.CreditsBalance)
		}
		if result.AccountID != "abc123" {
			t.Errorf("expected account_id abc123, got %s", result.AccountID)
		}
		if result.APIKeyName != "Default API Key" {
			t.Errorf("expected api_key_name 'Default API Key', got %s", result.APIKeyName)
		}
	})
}

func TestWebhooks(t *testing.T) {
	t.Run("creates webhook successfully", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/v1/webhooks" {
				t.Errorf("expected path /v1/webhooks, got %s", r.URL.Path)
			}

			response := map[string]interface{}{
				"success": true,
				"code":    "0",
				"message": "Success",
				"data": map[string]interface{}{
					"id":         "webhook_123",
					"url":        "https://example.com/webhook",
					"events":     []string{"file.completed", "file.failed"},
					"secret":     "test-secret",
					"is_active":  true,
					"created_at": "2026-02-04T10:30:00Z",
					"updated_at": "2026-02-04T10:30:00Z",
				},
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(response)
		}))
		defer server.Close()

		client, _ := NewClient(Config{APIKey: "test-key", BaseURL: server.URL})
		result, err := client.CreateWebhook(context.Background(), WebhookConfig{
			URL:    "https://example.com/webhook",
			Events: []WebhookEvent{EventFileCompleted, EventFileFailed},
		})

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.ID != "webhook_123" {
			t.Errorf("expected ID webhook_123, got %s", result.ID)
		}
		if result.Secret != "test-secret" {
			t.Errorf("expected secret test-secret, got %s", result.Secret)
		}
		if !result.IsActive {
			t.Error("expected is_active to be true")
		}
	})

	t.Run("lists webhooks successfully", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			response := map[string]interface{}{
				"success": true,
				"code":    "0",
				"message": "Success",
				"data": map[string]interface{}{
					"webhooks": []map[string]interface{}{
						{
							"id":         "webhook_123",
							"url":        "https://example.com/webhook",
							"events":     []string{"file.completed"},
							"is_active":  true,
							"created_at": "2026-02-04T10:30:00Z",
							"updated_at": "2026-02-04T10:30:00Z",
						},
					},
					"total": 1,
				},
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(response)
		}))
		defer server.Close()

		client, _ := NewClient(Config{APIKey: "test-key", BaseURL: server.URL})
		result, err := client.ListWebhooks(context.Background())

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result.Webhooks) != 1 {
			t.Errorf("expected 1 webhook, got %d", len(result.Webhooks))
		}
		if result.Total != 1 {
			t.Errorf("expected total 1, got %d", result.Total)
		}
	})

	t.Run("deletes webhook successfully", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/v1/webhooks/webhook_123" {
				t.Errorf("expected path /v1/webhooks/webhook_123, got %s", r.URL.Path)
			}
			w.WriteHeader(http.StatusNoContent)
		}))
		defer server.Close()

		client, _ := NewClient(Config{APIKey: "test-key", BaseURL: server.URL})
		err := client.DeleteWebhook(context.Background(), "webhook_123")

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestVerifyWebhookSignature(t *testing.T) {
	t.Run("verifies valid signature", func(t *testing.T) {
		payload := `{"event":"test"}`
		secret := "test-secret"
		// Pre-computed valid signature
		signature := "sha256=ad386d9a61a0540a089d2955a07280771439f9f8c41a4b94cd404a740061c3d9"

		result := VerifyWebhookSignature(payload, signature, secret)

		if !result {
			t.Error("expected signature to be valid")
		}
	})

	t.Run("rejects invalid signature", func(t *testing.T) {
		payload := `{"event":"test"}`
		secret := "test-secret"
		signature := "sha256=invalid"

		result := VerifyWebhookSignature(payload, signature, secret)

		if result {
			t.Error("expected signature to be invalid")
		}
	})
}

func TestErrors(t *testing.T) {
	t.Run("AuthenticationError", func(t *testing.T) {
		err := NewAuthenticationError("")
		if err.Code != "INVALID_API_KEY" {
			t.Errorf("expected code INVALID_API_KEY, got %s", err.Code)
		}
		if err.StatusCode != 401 {
			t.Errorf("expected status 401, got %d", err.StatusCode)
		}
	})

	t.Run("RateLimitError", func(t *testing.T) {
		err := NewRateLimitError("Rate limited", 60)
		if err.RetryAfter != 60 {
			t.Errorf("expected retry after 60, got %d", err.RetryAfter)
		}
	})

	t.Run("ValidationError", func(t *testing.T) {
		err := NewValidationError("Invalid input", "details here")
		if err.Details != "details here" {
			t.Errorf("expected details 'details here', got %s", err.Details)
		}
	})

	t.Run("InsufficientCreditsError with HTTP 402", func(t *testing.T) {
		err := NewInsufficientCreditsError("")
		if err.Code != "INSUFFICIENT_CREDITS" {
			t.Errorf("expected code INSUFFICIENT_CREDITS, got %s", err.Code)
		}
		if err.StatusCode != 402 {
			t.Errorf("expected status 402, got %d", err.StatusCode)
		}
	})

	t.Run("NotFoundError", func(t *testing.T) {
		err := NewNotFoundError("")
		if err.Code != "NOT_FOUND" {
			t.Errorf("expected code NOT_FOUND, got %s", err.Code)
		}
	})

	t.Run("TimeoutError", func(t *testing.T) {
		err := NewTimeoutError("Request timed out")
		if err.Message != "Request timed out" {
			t.Errorf("expected message 'Request timed out', got %s", err.Message)
		}
	})
}

func TestStatusEnums(t *testing.T) {
	t.Run("verification status enums", func(t *testing.T) {
		statuses := []VerificationStatus{
			StatusValid,
			StatusInvalid,
			StatusUnknown,
			StatusRisky,
			StatusDisposable,
			StatusCatchall,
			StatusRole,
		}

		expected := []string{"valid", "invalid", "unknown", "risky", "disposable", "catchall", "role"}

		for i, status := range statuses {
			if string(status) != expected[i] {
				t.Errorf("expected status %s, got %s", expected[i], status)
			}
		}
	})

	t.Run("webhook event enums", func(t *testing.T) {
		events := []WebhookEvent{
			EventFileCompleted,
			EventFileFailed,
		}

		expected := []string{"file.completed", "file.failed"}

		for i, event := range events {
			if string(event) != expected[i] {
				t.Errorf("expected event %s, got %s", expected[i], event)
			}
		}
	})
}
