// Package contract defines bounded, non-secret errors shared by the Host APIs.
package contract

import "time"

type Severity string

const (
	SeverityInfo     Severity = "info"
	SeverityWarning  Severity = "warning"
	SeverityError    Severity = "error"
	SeverityCritical Severity = "critical"
)

type Error struct {
	Code      string            `json:"code"`
	Component string            `json:"component"`
	Severity  Severity          `json:"severity"`
	Message   string            `json:"message"`
	Retryable bool              `json:"retryable"`
	Timestamp time.Time         `json:"timestamp"`
	Context   map[string]string `json:"context,omitempty"`
}

func New(code, component string, severity Severity, message string, retryable bool) Error {
	return Error{
		Code: code, Component: component, Severity: severity, Message: message,
		Retryable: retryable, Timestamp: time.Now().UTC(),
	}
}
