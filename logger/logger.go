// Package logger configures native slog with recursive structured-field redaction.
package logger

import (
	"context"
	"errors"
	"fmt"
	"github.com/cetrictech/go-platform-kit/internal/redact"
	"gopkg.in/natefinch/lumberjack.v2"
	"io"
	"log/slog"
	"os"
	"sync"
)

// Config contains explicit constructor settings; see package documentation for defaults.
type Config struct {
	JSON                                      bool
	Level                                     slog.Level
	Output                                    io.Writer
	OwnOutput                                 bool
	File                                      string
	MaxSizeMB, MaxBackups, MaxAgeDays         int
	Compress                                  bool
	Service, Version, Environment, TimeFormat string
	SanitizeText                              func(string) string
}

// Logger embeds native slog and owns applicable configured outputs.
type Logger struct {
	*slog.Logger
	level  *slog.LevelVar
	closer io.Closer
	state  *outputState
}

// New constructs and validates an explicitly owned instance. See package documentation for defaults and cleanup.
func New(c Config) *Logger {
	level := new(slog.LevelVar)
	level.Set(c.Level)
	out := c.Output
	if out == nil {
		out = os.Stderr
	}
	var closer io.Closer
	if c.OwnOutput {
		closer, _ = out.(io.Closer)
	}
	if c.File != "" {
		r := &lumberjack.Logger{Filename: c.File, MaxSize: c.MaxSizeMB, MaxBackups: c.MaxBackups, MaxAge: c.MaxAgeDays, Compress: c.Compress}
		out = r
		closer = r
	}
	opts := &slog.HandlerOptions{Level: level, ReplaceAttr: func(_ []string, a slog.Attr) slog.Attr {
		if a.Key == slog.TimeKey && c.TimeFormat != "" {
			return slog.String(a.Key, a.Value.Time().UTC().Format(c.TimeFormat))
		}
		a = clean(a)
		if a.Value.Kind() == slog.KindString && c.SanitizeText != nil {
			a.Value = slog.StringValue(c.SanitizeText(a.Value.String()))
		}
		return a
	}}
	var h slog.Handler
	if c.JSON {
		h = slog.NewJSONHandler(out, opts)
	} else {
		h = slog.NewTextHandler(out, opts)
	}
	l := slog.New(&contextHandler{h})
	if c.Service != "" {
		l = l.With("service", c.Service, "version", c.Version, "environment", c.Environment)
	}
	host, _ := os.Hostname()
	return &Logger{l.With("hostname", host), level, closer, &outputState{out: out, closer: closer}}
}

// From wraps an existing logger. Its handler and lifecycle remain caller-owned.
func From(l *slog.Logger) *Logger {
	if l == nil {
		l = slog.Default()
	}
	return &Logger{Logger: slog.New(&contextHandler{&redactHandler{l.Handler()}})}
}
func clean(a slog.Attr) slog.Attr {
	if redact.Sensitive(a.Key) {
		return slog.String(a.Key, "[REDACTED]")
	}
	a.Value = a.Value.Resolve()
	if a.Value.Kind() == slog.KindGroup {
		g := a.Value.Group()
		out := make([]slog.Attr, len(g))
		for i := range g {
			out[i] = clean(g[i])
		}
		a.Value = slog.GroupValue(out...)
	} else if a.Value.Kind() == slog.KindAny {
		v := a.Value.Any()
		if level, ok := v.(slog.Level); ok {
			a.Value = slog.StringValue(level.String())
		} else if e, ok := v.(error); ok {
			a.Value = slog.StringValue(redact.SafeText(e.Error()))
		} else {
			a.Value = slog.AnyValue(redact.Redact(v))
		}

	}
	if a.Value.Kind() == slog.KindString {
		a.Value = slog.StringValue(redact.SafeText(a.Value.String()))
	}
	return a
}

// SetLevel changes the shared dynamic logging threshold.
func (l *Logger) SetLevel(v slog.Level) {
	if l.level != nil {
		l.level.Set(v)
	}
}

// With returns a logger enriched with structured attributes.
func (l *Logger) With(args ...any) *Logger {
	return &Logger{l.Logger.With(args...), l.level, l.closer, l.state}
}

// WithError returns a logger with an error attribute; error text must be safe for logging.
func (l *Logger) WithError(e error) *Logger { return l.With("error", e) }

// WithContext copies logger correlation attributes from the supplied context.
func (l *Logger) WithContext(ctx context.Context) *Logger {
	v, _ := ctx.Value(contextKey{}).([]slog.Attr)
	args := make([]any, len(v))
	for i := range v {
		args[i] = v[i]
	}
	return l.With(args...)
}

// Close releases owned resources. See package documentation for shutdown ordering.
func (l *Logger) Close() error {
	if l.state == nil {
		return nil
	}
	l.state.once.Do(func() {
		if l.state.closer != nil {
			l.state.err = l.state.closer.Close()
		}
	})
	return l.state.err
}

type outputState struct {
	out    io.Writer
	closer io.Closer
	once   sync.Once
	err    error
}

// Open validates output settings and eagerly checks file access. New remains
// available for callers that deliberately use slog's lazy write error handling.
func Open(c Config) (*Logger, error) {
	if c.File != "" && c.Output != nil {
		return nil, errors.New("logger: choose file or output")
	}
	if c.MaxSizeMB < 0 || c.MaxBackups < 0 || c.MaxAgeDays < 0 {
		return nil, errors.New("logger: negative rotation setting")
	}
	if c.OwnOutput && c.File == "" {
		if _, ok := c.Output.(io.Closer); !ok {
			return nil, errors.New("logger: owned output must implement Close")
		}
	}
	if c.File != "" {
		f, err := os.OpenFile(c.File, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
		if err != nil {
			return nil, fmt.Errorf("logger: open output: %w", err)
		}
		if err := f.Close(); err != nil {
			return nil, err
		}
	}
	return New(c), nil
}

// Flush invokes a configured writer's flush contract. Injected writers remain
// caller-owned unless OwnOutput is explicitly enabled. Call after producers stop.
func (l *Logger) Flush(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if l.state == nil {
		return nil
	}
	switch w := l.state.out.(type) {
	case interface{ Flush(context.Context) error }:
		return w.Flush(ctx)
	case interface{ Flush() error }:
		return w.Flush()
	case interface{ Sync() error }:
		return w.Sync()
	default:
		return nil
	}
}

type contextKey struct{}

// Context adds correlation attributes without retaining the context in a client.
func Context(ctx context.Context, attrs ...slog.Attr) context.Context {
	old, _ := ctx.Value(contextKey{}).([]slog.Attr)
	a := append(append([]slog.Attr(nil), old...), attrs...)
	return context.WithValue(ctx, contextKey{}, a)
}

type contextHandler struct{ slog.Handler }

func (h *contextHandler) Handle(ctx context.Context, r slog.Record) error {
	a, _ := ctx.Value(contextKey{}).([]slog.Attr)
	r.AddAttrs(a...)
	return h.Handler.Handle(ctx, r)
}
func (h *contextHandler) WithAttrs(a []slog.Attr) slog.Handler {
	return &contextHandler{h.Handler.WithAttrs(a)}
}
func (h *contextHandler) WithGroup(g string) slog.Handler {
	return &contextHandler{h.Handler.WithGroup(g)}
}

type redactHandler struct{ slog.Handler }

func (h *redactHandler) Handle(ctx context.Context, r slog.Record) error {
	out := slog.NewRecord(r.Time, r.Level, redact.SafeText(r.Message), r.PC)
	r.Attrs(func(a slog.Attr) bool { out.AddAttrs(clean(a)); return true })
	return h.Handler.Handle(ctx, out)
}
func (h *redactHandler) WithAttrs(a []slog.Attr) slog.Handler {
	out := make([]slog.Attr, len(a))
	for i := range a {
		out[i] = clean(a[i])
	}
	return &redactHandler{h.Handler.WithAttrs(out)}
}
func (h *redactHandler) WithGroup(g string) slog.Handler {
	return &redactHandler{h.Handler.WithGroup(g)}
}
