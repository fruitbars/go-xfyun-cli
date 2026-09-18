package xfyun

import "fmt"

type APIError struct {
	Service string
	Code    string
	Message string
	SID     string
}

func (e *APIError) Error() string {
	if e.SID != "" {
		return fmt.Sprintf("%s API error %s: %s (sid: %s)", e.Service, e.Code, e.Message, e.SID)
	}
	return fmt.Sprintf("%s API error %s: %s", e.Service, e.Code, e.Message)
}
