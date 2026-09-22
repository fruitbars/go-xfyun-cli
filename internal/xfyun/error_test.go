package xfyun

import (
	"strings"
	"testing"
)

func TestAPIErrorIncludesConservativeRetryMetadata(t *testing.T) {
	err := &APIError{Service: "OCR", Code: "500", Message: "temporary service unavailable", SID: "sid"}
	message := err.Error()
	if !strings.Contains(message, "category=upstream") || !strings.Contains(message, "retryable=true") {
		t.Fatalf("error = %q", message)
	}
}

func TestAPIErrorDoesNotMarkInvalidInputRetryable(t *testing.T) {
	err := &APIError{Service: "TTS", Code: "400", Message: "invalid parameter"}
	message := err.Error()
	if !strings.Contains(message, "category=invalid_request") || !strings.Contains(message, "retryable=false") {
		t.Fatalf("error = %q", message)
	}
}
