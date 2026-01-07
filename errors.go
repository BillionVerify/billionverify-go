package emailverify

import "fmt"

// EmailVerifyError is the base error type for all SDK errors.
type EmailVerifyError struct {
	Code       string
	Message    string
	StatusCode int
	Details    string
}

func (e *EmailVerifyError) Error() string {
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// AuthenticationError is returned when API key is invalid or missing.
type AuthenticationError struct {
	EmailVerifyError
}

// NewAuthenticationError creates a new AuthenticationError.
func NewAuthenticationError(message string) *AuthenticationError {
	if message == "" {
		message = "Invalid or missing API key"
	}
	return &AuthenticationError{
		EmailVerifyError: EmailVerifyError{
			Code:       "INVALID_API_KEY",
			Message:    message,
			StatusCode: 401,
		},
	}
}

// RateLimitError is returned when rate limit is exceeded.
type RateLimitError struct {
	EmailVerifyError
	RetryAfter int
}

// NewRateLimitError creates a new RateLimitError.
func NewRateLimitError(message string, retryAfter int) *RateLimitError {
	if message == "" {
		message = "Rate limit exceeded"
	}
	return &RateLimitError{
		EmailVerifyError: EmailVerifyError{
			Code:       "RATE_LIMIT_EXCEEDED",
			Message:    message,
			StatusCode: 429,
		},
		RetryAfter: retryAfter,
	}
}

// ValidationError is returned when request validation fails.
type ValidationError struct {
	EmailVerifyError
}

// NewValidationError creates a new ValidationError.
func NewValidationError(message string, details string) *ValidationError {
	return &ValidationError{
		EmailVerifyError: EmailVerifyError{
			Code:       "INVALID_REQUEST",
			Message:    message,
			StatusCode: 400,
			Details:    details,
		},
	}
}

// InsufficientCreditsError is returned when there are not enough credits.
type InsufficientCreditsError struct {
	EmailVerifyError
}

// NewInsufficientCreditsError creates a new InsufficientCreditsError.
func NewInsufficientCreditsError(message string) *InsufficientCreditsError {
	if message == "" {
		message = "Insufficient credits"
	}
	return &InsufficientCreditsError{
		EmailVerifyError: EmailVerifyError{
			Code:       "INSUFFICIENT_CREDITS",
			Message:    message,
			StatusCode: 403,
		},
	}
}

// NotFoundError is returned when a resource is not found.
type NotFoundError struct {
	EmailVerifyError
}

// NewNotFoundError creates a new NotFoundError.
func NewNotFoundError(message string) *NotFoundError {
	if message == "" {
		message = "Resource not found"
	}
	return &NotFoundError{
		EmailVerifyError: EmailVerifyError{
			Code:       "NOT_FOUND",
			Message:    message,
			StatusCode: 404,
		},
	}
}

// TimeoutError is returned when a request times out.
type TimeoutError struct {
	EmailVerifyError
}

// NewTimeoutError creates a new TimeoutError.
func NewTimeoutError(message string) *TimeoutError {
	if message == "" {
		message = "Request timed out"
	}
	return &TimeoutError{
		EmailVerifyError: EmailVerifyError{
			Code:       "TIMEOUT",
			Message:    message,
			StatusCode: 504,
		},
	}
}
