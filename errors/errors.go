// Package errors provides safe application errors interoperable with standard Go errors.
package errors

import (
	"context"
	"database/sql"
	stderrors "errors"
	"fmt"
	"net"
	"net/http"
	"time"
)

// Category classifies an application failure.
type Category string

// Severity indicates diagnostic importance.
type Severity string

const (
	Validation     Category = "VALIDATION"
	Authentication Category = "AUTHENTICATION"
	Authorization  Category = "AUTHORIZATION"
	Business       Category = "BUSINESS"
	NotFound       Category = "NOT_FOUND"
	Conflict       Category = "CONFLICT"
	RateLimit      Category = "RATE_LIMIT"
	Database       Category = "DATABASE"
	Cache          Category = "CACHE"
	Messaging      Category = "MESSAGING"
	Network        Category = "NETWORK"
	Infrastructure Category = "INFRASTRUCTURE"
	Internal       Category = "INTERNAL"
	Timeout        Category = "TIMEOUT"
	Unavailable    Category = "UNAVAILABLE"
)
const (
	Debug    Severity = "DEBUG"
	Info     Severity = "INFO"
	Warning  Severity = "WARNING"
	Error    Severity = "ERROR"
	Critical Severity = "CRITICAL"
)

// AppError separates public data from private diagnostics. Details and Metadata must contain only caller-approved public data.
type AppError struct {
	Code          string         `json:"code"`
	Message       string         `json:"message"`
	DebugMessage  string         `json:"-"`
	Category      Category       `json:"category"`
	Severity      Severity       `json:"severity"`
	HTTPStatus    int            `json:"-"`
	Details       map[string]any `json:"details,omitempty"`
	Cause         error          `json:"-"`
	Retryable     bool           `json:"retryable,omitempty"`
	RetryAfter    time.Duration  `json:"-"`
	TraceID       string         `json:"trace_id,omitempty"`
	CorrelationID string         `json:"correlation_id,omitempty"`
	Metadata      map[string]any `json:"metadata,omitempty"`
}

// Error returns a diagnostic error description.
func (e *AppError) Error() string {
	if e == nil {
		return "<nil>"
	}
	message := e.Code + ": " + e.Message
	if e.DebugMessage != "" {
		message += " (" + e.DebugMessage + ")"
	}
	if e.Cause != nil {
		message += ": " + e.Cause.Error()
	}
	return message
}

// Unwrap returns the underlying wrapped resource or error cause.
func (e *AppError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

// Option configures a newly constructed application error.
type Option func(*AppError)

// New constructs and validates an explicitly owned instance. See package documentation for defaults and cleanup.
func New(message string, opts ...Option) *AppError {
	e := &AppError{Code: "INTERNAL_ERROR", Message: message, Category: Internal, Severity: Error, HTTPStatus: 500}
	for _, o := range opts {
		if o != nil {
			o(e)
		}
	}
	return e
}

// Wrap constructs an application error preserving its underlying cause.
func Wrap(cause error, message string, opts ...Option) *AppError {
	e := New(message, opts...)
	e.Cause = cause
	return e
}

// Wrapf constructs a wrapped error with a formatted customer-facing message.
func Wrapf(cause error, format string, args ...any) *AppError {
	return Wrap(cause, fmt.Sprintf(format, args...))
}

// WithCode sets a machine-readable application error code.
func WithCode(v string) Option { return func(e *AppError) { e.Code = v } }

// WithMessage sets an approved customer-facing message.
func WithMessage(v string) Option { return func(e *AppError) { e.Message = v } }

// WithDebug sets internal diagnostic text excluded from default responses.
func WithDebug(v string) Option { return func(e *AppError) { e.DebugMessage = v } }

// WithCategory sets the application error category.
func WithCategory(v Category) Option { return func(e *AppError) { e.Category = v } }

// WithSeverity sets the application error severity.
func WithSeverity(v Severity) Option { return func(e *AppError) { e.Severity = v } }

// WithHTTPStatus sets the intended HTTP status.
func WithHTTPStatus(v int) Option { return func(e *AppError) { e.HTTPStatus = v } }

// WithDetails sets explicitly approved public validation details.
func WithDetails(v map[string]any) Option { return func(e *AppError) { e.Details = v } }

// WithCause sets the underlying diagnostic cause.
func WithCause(v error) Option { return func(e *AppError) { e.Cause = v } }

// WithRetryable sets retryability metadata without performing retries.
func WithRetryable(v bool) Option { return func(e *AppError) { e.Retryable = v } }

// WithRetryAfter sets retry delay metadata.
func WithRetryAfter(v time.Duration) Option { return func(e *AppError) { e.RetryAfter = v } }

// Is delegates standard error identity matching.
func Is(err, target error) bool { return stderrors.Is(err, target) }

// As delegates standard error type matching.
func As(err error, target any) bool { return stderrors.As(err, target) }

// Unwrap returns the underlying wrapped resource or error cause.
func Unwrap(err error) error { return stderrors.Unwrap(err) }

// Classify recognizes core errors without importing optional drivers.
func Classify(err error) *AppError {
	if err == nil {
		return nil
	}
	var a *AppError
	if As(err, &a) && a != nil {
		return a
	}
	e := Wrap(err, "We could not complete your request.")
	if Is(err, sql.ErrNoRows) {
		e.Category = NotFound
		e.Code = "NOT_FOUND"
		e.HTTPStatus = 404
		e.Message = "The requested resource was not found."
	} else if Is(err, context.DeadlineExceeded) {
		e.Category = Timeout
		e.Code = "TIMEOUT"
		e.HTTPStatus = 504
		e.Retryable = true
	} else if Is(err, context.Canceled) {
		e.Category = Network
		e.Code = "CANCELED"
		e.HTTPStatus = 408
	} else {
		var n net.Error
		if As(err, &n) {
			e.Category = Network
			e.Code = "NETWORK_ERROR"
			e.HTTPStatus = http.StatusServiceUnavailable
			e.Retryable = n.Timeout()
		}
	}
	return e
}

// Recover converts a panic into an error and invokes report. Use as defer Recover(&err, report).
func Recover(dst *error, report func(error)) {
	if p := recover(); p != nil {
		e := New("An internal error occurred.", WithDebug(fmt.Sprintf("panic: %v", p)))
		if report != nil {
			report(e)
		}
		if dst == nil {
			panic(p)
		}
		*dst = e
	}
}
