// Package response writes bounded JSON envelopes using native net/http.
package response

import (
	"bytes"
	"encoding/json"
	"errors"
	app "github.com/cetrictech/go-platform-kit/errors"
	"github.com/cetrictech/go-platform-kit/security"
	"io"
	"net/http"
	"strconv"
	"time"
)

var ErrWritten = errors.New("response: response already committed")

// DebugPolicy controls explicit server-authorized diagnostic exposure.
type DebugPolicy int

const (
	Never DebugPolicy = iota
	DevelopmentOnly
	AuthorizedInternal
)

// Options controls explicit loading or response policy.
type Options struct {
	MaxBytes       int
	Debug          DebugPolicy
	Development    bool
	AuthorizeDebug func(*http.Request) bool
	SanitizeDebug  func(string) string
	EscapeHTML     *bool
}

// Envelope contains public JSON response data and correlation metadata.
type Envelope struct {
	Success       bool      `json:"success"`
	Code          string    `json:"code"`
	Message       string    `json:"message"`
	Data          any       `json:"data,omitempty"`
	Error         any       `json:"error,omitempty"`
	Meta          any       `json:"meta,omitempty"`
	TraceID       string    `json:"trace_id,omitempty"`
	CorrelationID string    `json:"correlation_id,omitempty"`
	Timestamp     time.Time `json:"timestamp"`
}

// Writer tracks commitment when shared by every component writing a response.
type Writer struct {
	http.ResponseWriter
	Written bool
	Status  int
}

// WriteHeader records and writes the HTTP status once.
func (w *Writer) WriteHeader(status int) {
	if !w.Written {
		w.Written = true
		w.Status = status
		w.ResponseWriter.WriteHeader(status)
	}
}

// Write implements the wrapped HTTP writer with commitment tracking.
func (w *Writer) Write(b []byte) (int, error) {
	if !w.Written {
		w.WriteHeader(200)
	}
	return w.ResponseWriter.Write(b)
}

// Unwrap returns the underlying wrapped resource or error cause.
func (w *Writer) Unwrap() http.ResponseWriter { return w.ResponseWriter }

type bounded struct {
	bytes.Buffer
	max int
}

// Write implements the wrapped HTTP writer with commitment tracking.
func (b *bounded) Write(p []byte) (int, error) {
	if len(p) > b.max-b.Len() {
		return 0, errors.New("response: size limit exceeded")
	}
	return b.Buffer.Write(p)
}

// JSON writes or processes a bounded JSON representation; see package documentation for ownership.
func JSON(w http.ResponseWriter, status int, v any, options ...Options) error {
	if x, ok := w.(*Writer); ok && x.Written {
		return ErrWritten
	}
	if status < 200 || status > 599 {
		return errors.New("response: invalid status")
	}
	if status == 204 || status == 304 {
		w.WriteHeader(status)
		return nil
	}
	o := Options{}
	if len(options) > 0 {
		o = options[0]
	}
	max := o.MaxBytes
	if max <= 0 {
		max = 1 << 20
	}
	b := &bounded{max: max}
	enc := json.NewEncoder(b)
	if o.EscapeHTML != nil {
		enc.SetEscapeHTML(*o.EscapeHTML)
	}
	if e := enc.Encode(v); e != nil {
		return e
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_, e := io.Copy(w, &b.Buffer)
	return e
}
func success(w http.ResponseWriter, status int, data, meta any) error {
	return JSON(w, status, Envelope{Success: true, Code: "SUCCESS", Message: http.StatusText(status), Data: data, Meta: meta, Timestamp: time.Now().UTC()})
}

// OK writes an HTTP 200 success envelope.
func OK(w http.ResponseWriter, data any) error { return success(w, 200, data, nil) }

// Created writes an HTTP 201 success envelope.
func Created(w http.ResponseWriter, data any) error { return success(w, 201, data, nil) }

// Accepted writes an HTTP 202 success envelope.
func Accepted(w http.ResponseWriter, data any) error { return success(w, 202, data, nil) }

// NoContent writes HTTP 204 without a response body.
func NoContent(w http.ResponseWriter) error { return JSON(w, 204, nil) }

// Paginated writes a success envelope with pagination metadata.
func Paginated(w http.ResponseWriter, data, meta any) error { return success(w, 200, data, meta) }

// Error maps an error to a safe public HTTP envelope under explicit diagnostic policy.
func Error(w http.ResponseWriter, r *http.Request, err error, options ...Options) error {
	e := app.Classify(err)
	if e == nil {
		return errors.New("response: nil error")
	}
	o := Options{}
	if len(options) > 0 {
		o = options[0]
	}
	detail := map[string]any{"category": e.Category, "severity": e.Severity, "details": security.Redact(e.Details)}
	allow := o.Debug == DevelopmentOnly && o.Development || o.Debug == AuthorizedInternal && o.AuthorizeDebug != nil && r != nil && o.AuthorizeDebug(r)
	if allow && o.SanitizeDebug != nil {
		detail["debug_error"] = o.SanitizeDebug(e.DebugMessage)
	}
	status := e.HTTPStatus
	if status < 400 || status > 599 {
		status = 500
	}
	if e.RetryAfter > 0 {
		seconds := e.RetryAfter / time.Second
		if e.RetryAfter%time.Second != 0 {
			seconds++
		}
		w.Header().Set("Retry-After", strconv.FormatInt(int64(seconds), 10))
	}
	return JSON(w, status, Envelope{Code: e.Code, Message: e.Message, Error: detail, TraceID: e.TraceID, CorrelationID: e.CorrelationID, Timestamp: time.Now().UTC()}, o)
}

// BadRequest writes the corresponding HTTP error envelope with an approved public message.
func BadRequest(w http.ResponseWriter, message string) error {
	if message == "" {
		message = http.StatusText(http.StatusBadRequest)
	}
	return JSON(w, 400, Envelope{Code: "BADREQUEST", Message: message, Timestamp: time.Now().UTC()})
}

// Unauthorized writes the corresponding HTTP error envelope with an approved public message.
func Unauthorized(w http.ResponseWriter, message string) error {
	if message == "" {
		message = http.StatusText(http.StatusUnauthorized)
	}
	return JSON(w, 401, Envelope{Code: "UNAUTHORIZED", Message: message, Timestamp: time.Now().UTC()})
}

// Forbidden writes the corresponding HTTP error envelope with an approved public message.
func Forbidden(w http.ResponseWriter, message string) error {
	if message == "" {
		message = http.StatusText(http.StatusForbidden)
	}
	return JSON(w, 403, Envelope{Code: "FORBIDDEN", Message: message, Timestamp: time.Now().UTC()})
}

// NotFound writes the corresponding HTTP error envelope with an approved public message.
func NotFound(w http.ResponseWriter, message string) error {
	if message == "" {
		message = http.StatusText(http.StatusNotFound)
	}
	return JSON(w, 404, Envelope{Code: "NOTFOUND", Message: message, Timestamp: time.Now().UTC()})
}

// MethodNotAllowed writes the corresponding HTTP error envelope with an approved public message.
func MethodNotAllowed(w http.ResponseWriter, message string) error {
	if message == "" {
		message = http.StatusText(http.StatusMethodNotAllowed)
	}
	return JSON(w, 405, Envelope{Code: "METHODNOTALLOWED", Message: message, Timestamp: time.Now().UTC()})
}

// Conflict writes the corresponding HTTP error envelope with an approved public message.
func Conflict(w http.ResponseWriter, message string) error {
	if message == "" {
		message = http.StatusText(http.StatusConflict)
	}
	return JSON(w, 409, Envelope{Code: "CONFLICT", Message: message, Timestamp: time.Now().UTC()})
}

// Gone writes the corresponding HTTP error envelope with an approved public message.
func Gone(w http.ResponseWriter, message string) error {
	if message == "" {
		message = http.StatusText(http.StatusGone)
	}
	return JSON(w, 410, Envelope{Code: "GONE", Message: message, Timestamp: time.Now().UTC()})
}

// UnprocessableEntity writes the corresponding HTTP error envelope with an approved public message.
func UnprocessableEntity(w http.ResponseWriter, message string) error {
	if message == "" {
		message = http.StatusText(http.StatusUnprocessableEntity)
	}
	return JSON(w, 422, Envelope{Code: "UNPROCESSABLEENTITY", Message: message, Timestamp: time.Now().UTC()})
}

// TooManyRequests writes the corresponding HTTP error envelope with an approved public message.
func TooManyRequests(w http.ResponseWriter, message string) error {
	if message == "" {
		message = http.StatusText(http.StatusTooManyRequests)
	}
	return JSON(w, 429, Envelope{Code: "TOOMANYREQUESTS", Message: message, Timestamp: time.Now().UTC()})
}

// InternalServerError writes the corresponding HTTP error envelope with an approved public message.
func InternalServerError(w http.ResponseWriter, message string) error {
	if message == "" {
		message = http.StatusText(http.StatusInternalServerError)
	}
	return JSON(w, 500, Envelope{Code: "INTERNALSERVERERROR", Message: message, Timestamp: time.Now().UTC()})
}

// BadGateway writes the corresponding HTTP error envelope with an approved public message.
func BadGateway(w http.ResponseWriter, message string) error {
	if message == "" {
		message = http.StatusText(http.StatusBadGateway)
	}
	return JSON(w, 502, Envelope{Code: "BADGATEWAY", Message: message, Timestamp: time.Now().UTC()})
}

// ServiceUnavailable writes the corresponding HTTP error envelope with an approved public message.
func ServiceUnavailable(w http.ResponseWriter, message string) error {
	if message == "" {
		message = http.StatusText(http.StatusServiceUnavailable)
	}
	return JSON(w, 503, Envelope{Code: "SERVICEUNAVAILABLE", Message: message, Timestamp: time.Now().UTC()})
}

// GatewayTimeout writes the corresponding HTTP error envelope with an approved public message.
func GatewayTimeout(w http.ResponseWriter, message string) error {
	if message == "" {
		message = http.StatusText(http.StatusGatewayTimeout)
	}
	return JSON(w, 504, Envelope{Code: "GATEWAYTIMEOUT", Message: message, Timestamp: time.Now().UTC()})
}
