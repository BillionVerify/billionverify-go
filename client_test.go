package emailverify

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

func TestVerify(t *testing.T) {
	t.Run("verifies email successfully", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/verify" {
				t.Errorf("expected path /verify, got %s", r.URL.Path)
			}
			if r.Method != http.MethodPost {
				t.Errorf("expected POST method, got %s", r.Method)
			}
			if r.Header.Get("EMAILVERIFY-API-KEY") != "test-key" {
				t.Errorf("expected EMAILVERIFY-API-KEY header")
			}

			response := VerifyResponse{
				Email:  "test@example.com",
				Status: StatusValid,
				Result: VerificationResult{
					Deliverable: true,
					ValidFormat: true,
					ValidDomain: true,
					ValidMX:     true,
					Disposable:  false,
					Role:        false,
					Catchall:    false,
					Free:        false,
					SMTPValid:   true,
				},
				Score:       0.95,
				CreditsUsed: 1,
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

	t.Run("handles insufficient credits error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusForbidden)
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

func TestVerifyBulk(t *testing.T) {
	t.Run("submits bulk job successfully", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/verify/bulk" {
				t.Errorf("expected path /verify/bulk, got %s", r.URL.Path)
			}

			response := BulkJobResponse{
				JobID:       "job_123",
				Status:      JobStatusProcessing,
				Total:       3,
				Processed:   0,
				Valid:       0,
				Invalid:     0,
				Unknown:     0,
				CreditsUsed: 3,
				CreatedAt:   "2025-01-15T10:30:00Z",
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(response)
		}))
		defer server.Close()

		client, _ := NewClient(Config{APIKey: "test-key", BaseURL: server.URL})
		result, err := client.VerifyBulk(context.Background(), []string{
			"user1@example.com",
			"user2@example.com",
			"user3@example.com",
		}, nil)

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.JobID != "job_123" {
			t.Errorf("expected job ID job_123, got %s", result.JobID)
		}
		if result.Total != 3 {
			t.Errorf("expected total 3, got %d", result.Total)
		}
	})

	t.Run("rejects too many emails", func(t *testing.T) {
		client, _ := NewClient(Config{APIKey: "test-key"})
		emails := make([]string, 10001)
		for i := range emails {
			emails[i] = "test@example.com"
		}

		_, err := client.VerifyBulk(context.Background(), emails, nil)

		if err == nil {
			t.Fatal("expected error")
		}
		if _, ok := err.(*ValidationError); !ok {
			t.Errorf("expected ValidationError, got %T", err)
		}
	})
}

func TestGetBulkJobStatus(t *testing.T) {
	t.Run("gets job status successfully", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/verify/bulk/job_123" {
				t.Errorf("expected path /verify/bulk/job_123, got %s", r.URL.Path)
			}

			progress := 50
			response := BulkJobResponse{
				JobID:           "job_123",
				Status:          JobStatusProcessing,
				Total:           100,
				Processed:       50,
				Valid:           40,
				Invalid:         5,
				Unknown:         5,
				CreditsUsed:     100,
				CreatedAt:       "2025-01-15T10:30:00Z",
				ProgressPercent: &progress,
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(response)
		}))
		defer server.Close()

		client, _ := NewClient(Config{APIKey: "test-key", BaseURL: server.URL})
		result, err := client.GetBulkJobStatus(context.Background(), "job_123")

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.JobID != "job_123" {
			t.Errorf("expected job ID job_123, got %s", result.JobID)
		}
		if *result.ProgressPercent != 50 {
			t.Errorf("expected progress 50, got %d", *result.ProgressPercent)
		}
	})
}

func TestGetBulkJobResults(t *testing.T) {
	t.Run("gets job results successfully", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/verify/bulk/job_123/results" {
				t.Errorf("expected path /verify/bulk/job_123/results, got %s", r.URL.Path)
			}

			response := BulkResultsResponse{
				JobID:  "job_123",
				Total:  100,
				Limit:  50,
				Offset: 0,
				Results: []BulkResultItem{
					{
						Email:  "test@example.com",
						Status: StatusValid,
						Result: map[string]interface{}{"deliverable": true},
						Score:  0.95,
					},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(response)
		}))
		defer server.Close()

		client, _ := NewClient(Config{APIKey: "test-key", BaseURL: server.URL})
		result, err := client.GetBulkJobResults(context.Background(), "job_123", &BulkResultsOptions{
			Limit:  50,
			Offset: 0,
		})

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result.Results) != 1 {
			t.Errorf("expected 1 result, got %d", len(result.Results))
		}
		if result.Results[0].Email != "test@example.com" {
			t.Errorf("expected email test@example.com, got %s", result.Results[0].Email)
		}
	})
}

func TestGetCredits(t *testing.T) {
	t.Run("gets credits successfully", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/credits" {
				t.Errorf("expected path /credits, got %s", r.URL.Path)
			}

			response := CreditsResponse{
				Available: 9500,
				Used:      500,
				Total:     10000,
				Plan:      "Professional",
				ResetsAt:  "2025-02-01T00:00:00Z",
				RateLimit: RateLimit{
					RequestsPerHour: 10000,
					Remaining:       9850,
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
		if result.Available != 9500 {
			t.Errorf("expected available 9500, got %d", result.Available)
		}
		if result.Plan != "Professional" {
			t.Errorf("expected plan Professional, got %s", result.Plan)
		}
	})
}

func TestWebhooks(t *testing.T) {
	t.Run("creates webhook successfully", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/webhooks" {
				t.Errorf("expected path /webhooks, got %s", r.URL.Path)
			}

			response := Webhook{
				ID:        "webhook_123",
				URL:       "https://example.com/webhook",
				Events:    []WebhookEvent{EventVerificationCompleted},
				CreatedAt: "2025-01-15T10:30:00Z",
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(response)
		}))
		defer server.Close()

		client, _ := NewClient(Config{APIKey: "test-key", BaseURL: server.URL})
		result, err := client.CreateWebhook(context.Background(), WebhookConfig{
			URL:    "https://example.com/webhook",
			Events: []WebhookEvent{EventVerificationCompleted},
		})

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.ID != "webhook_123" {
			t.Errorf("expected ID webhook_123, got %s", result.ID)
		}
	})

	t.Run("lists webhooks successfully", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			response := []Webhook{
				{
					ID:        "webhook_123",
					URL:       "https://example.com/webhook",
					Events:    []WebhookEvent{EventVerificationCompleted},
					CreatedAt: "2025-01-15T10:30:00Z",
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
		if len(result) != 1 {
			t.Errorf("expected 1 webhook, got %d", len(result))
		}
	})

	t.Run("deletes webhook successfully", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/webhooks/webhook_123" {
				t.Errorf("expected path /webhooks/webhook_123, got %s", r.URL.Path)
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
		signature := "sha256=f7bc83f430538424b13298e6aa6fb143ef4d59a14946175997479dbc2d1a3cd8"

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

	t.Run("InsufficientCreditsError", func(t *testing.T) {
		err := NewInsufficientCreditsError("")
		if err.Code != "INSUFFICIENT_CREDITS" {
			t.Errorf("expected code INSUFFICIENT_CREDITS, got %s", err.Code)
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
