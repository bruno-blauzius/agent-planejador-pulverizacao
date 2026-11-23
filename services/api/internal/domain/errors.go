// Package domain holds the business entities and invariants of the API for
// users, companies and invitations (ADR 0013 and 0015). It depends only on the
// standard library.
package domain

import (
	"errors"
	"fmt"
	"strings"
)

// Sentinel errors of the domain. Adapters translate them to HTTP codes.
var (
	// ErrNoAccessToProduct means the identity has no product role at all.
	ErrNoAccessToProduct = errors.New("identity has no role in this product")
	// ErrAmbiguousRole means the identity has more than one product role.
	ErrAmbiguousRole = errors.New("identity has more than one role in this product")
	// ErrInvitationClosed means the invitation was already accepted or cancelled.
	ErrInvitationClosed = errors.New("invitation is closed")
	// ErrInvalidTransition means the status change is not allowed from the current status.
	ErrInvalidTransition = errors.New("invalid status transition")
)

// FieldError describes one invalid input field.
type FieldError struct {
	Field   string
	Message string
}

// ValidationError aggregates field failures so the API can answer 422 with
// one entry per field.
type ValidationError struct {
	Fields []FieldError
}

// Error implements error.
func (e *ValidationError) Error() string {
	parts := make([]string, 0, len(e.Fields))
	for _, f := range e.Fields {
		parts = append(parts, f.Field+": "+f.Message)
	}
	return "validation failed: " + strings.Join(parts, "; ")
}

// Violations collects field failures while a value is being validated.
type Violations []FieldError

// Add records a failure for a field.
func (v *Violations) Add(field, message string) {
	*v = append(*v, FieldError{Field: field, Message: message})
}

// Merge appends the field failures carried by err when it is a ValidationError.
// Any other error is returned unchanged so callers can stop on it.
func (v *Violations) Merge(err error) error {
	if err == nil {
		return nil
	}
	var verr *ValidationError
	if !errors.As(err, &verr) {
		return err
	}
	*v = append(*v, verr.Fields...)
	return nil
}

// Err returns a *ValidationError when something was recorded, otherwise nil.
func (v Violations) Err() error {
	if len(v) == 0 {
		return nil
	}
	return &ValidationError{Fields: append([]FieldError(nil), v...)}
}

// Invalid builds a ValidationError for a single field.
func Invalid(field, format string, args ...any) error {
	return &ValidationError{Fields: []FieldError{{Field: field, Message: fmt.Sprintf(format, args...)}}}
}
