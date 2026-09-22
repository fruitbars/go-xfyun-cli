package xfyun

import (
	"fmt"
	"strings"
)

type APIError struct {
	Service      string
	Code         string
	Message      string
	SID          string
	Category     string
	Retryable    bool
	RetryAfterMS int
}

func (e *APIError) Error() string {
	e.normalize()
	detail := fmt.Sprintf("category=%s retryable=%t", e.Category, e.Retryable)
	if e.RetryAfterMS > 0 {
		detail += fmt.Sprintf(" retry_after_ms=%d", e.RetryAfterMS)
	}
	if e.SID != "" {
		return fmt.Sprintf("%s API error %s: %s (sid: %s; %s)", e.Service, e.Code, e.Message, e.SID, detail)
	}
	return fmt.Sprintf("%s API error %s: %s (%s)", e.Service, e.Code, e.Message, detail)
}

// normalize provides conservative, machine-readable retry guidance without
// pretending to know every provider-specific numeric error code. Callers may
// override Category/Retryable when a service response gives stronger data.
func (e *APIError) normalize() {
	if e.Category != "" {
		return
	}
	message := strings.ToLower(e.Message)
	switch {
	case strings.Contains(message, "auth"), strings.Contains(message, "api key"), strings.Contains(message, "secret"), strings.Contains(message, "signature"), strings.Contains(message, "token"):
		e.Category = "authentication"
	case strings.Contains(message, "quota"), strings.Contains(message, "rate limit"), strings.Contains(message, "frequency"), strings.Contains(message, "too many"):
		e.Category = "quota"
	case strings.Contains(message, "permission"), strings.Contains(message, "forbidden"), strings.Contains(message, "not allowed"):
		e.Category = "permission"
	case strings.Contains(message, "invalid"), strings.Contains(message, "parameter"), strings.Contains(message, "format"), strings.Contains(message, "required"):
		e.Category = "invalid_request"
	default:
		e.Category = "upstream"
	}
	if e.Category == "upstream" && (strings.Contains(message, "timeout") || strings.Contains(message, "temporar") || strings.Contains(message, "unavailable") || strings.Contains(message, "busy")) {
		e.Retryable = true
	}
}
