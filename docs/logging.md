# Logging

```go
log := logger.New(logger.Config{JSON:true, Service:"billing", Environment:"production"})
defer log.Close()
log.InfoContext(ctx, "operation complete", "operation", "settlement")
log.SetLevel(slog.LevelDebug)
```

Use native slog methods, including InfoContext, DebugContext, WarnContext and ErrorContext. With, WithError and WithContext preserve configured output ownership. `logger.Context` adds request/correlation fields to a context. `observability.Correlate` adds active trace/span IDs.

Output defaults to stderr. File selects a rotating output; MaxSizeMB, MaxBackups, MaxAgeDays and Compress use lumberjack rotation semantics (zero size means 100 MB; zero retention/count means unlimited). Close the owned output once; wrappers from From retain caller ownership. Caller-supplied writers must be safe for concurrent use through slog.

Redaction covers sensitive structured keys, groups and JSON-serializable nested values. Messages and error strings are free text and cannot be reliably scrubbed automatically. Never place credentials, tokens, DSNs, private keys or raw sensitive request bodies in free text. Provide sanitized error descriptions. Secrets in caller-defined custom text representations remain caller responsibility.
