// Package logger configures native slog with recursive structured-field redaction.
package logger

import (
	"context"
	"encoding/json"
	"github.com/cetrictech/go-platform-kit/internal/redact"
	"gopkg.in/natefinch/lumberjack.v2"
	"io"
	"log/slog"
	"os"
)

// Config contains explicit constructor settings; see package documentation for defaults.
type Config struct {
	JSON                                      bool
	Level                                     slog.Level
	Output                                    io.Writer
	File                                      string
	MaxSizeMB, MaxBackups, MaxAgeDays         int
	Compress                                  bool
	Service, Version, Environment, TimeFormat string
}

// Logger embeds native slog and owns applicable configured outputs.
type Logger struct {
	*slog.Logger
	level  *slog.LevelVar
	closer io.Closer
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
	if c.File != "" {
		r := &lumberjack.Logger{Filename: c.File, MaxSize: c.MaxSizeMB, MaxBackups: c.MaxBackups, MaxAge: c.MaxAgeDays, Compress: c.Compress}
		out = r
		closer = r
	}
	opts := &slog.HandlerOptions{Level: level, ReplaceAttr: func(_ []string, a slog.Attr) slog.Attr {
		if a.Key == slog.TimeKey && c.TimeFormat != "" {
			return slog.String(a.Key, a.Value.Time().UTC().Format(c.TimeFormat))
		}
		return clean(a)
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
	return &Logger{l.With("hostname", host), level, closer}
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
		if e, ok := v.(error); ok {
			a.Value = slog.StringValue(e.Error())
		} else if b, e := json.Marshal(v); e == nil {
			var x any
			if json.Unmarshal(b, &x) == nil {
				a.Value = slog.AnyValue(redact.Redact(x))
			}
		}
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
func (l *Logger) With(args ...any) *Logger { return &Logger{l.Logger.With(args...), l.level, l.closer} }

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
	if l.closer != nil {
		return l.closer.Close()
	}
	return nil
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
	out := slog.NewRecord(r.Time, r.Level, r.Message, r.PC)
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
