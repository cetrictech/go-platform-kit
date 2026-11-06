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

Use `logger.Open` when startup must fail on invalid rotation settings or an inaccessible file. `New` preserves slog's lazy error handling. `Open` eagerly opens the configured file with mode 0600 for new files; protect its parent directory and existing file permissions explicitly. Do not configure both `File` and `Output` with `Open`.

Injected outputs stay caller-owned unless `OwnOutput` is true. Loggers derived with `With` share one close operation and dynamic level. `Flush(ctx)` invokes the writer's supported flush/sync contract; stop producers first. A synchronous writer's `Flush()` or `Sync()` must itself be bounded. Lumberjack writes directly and has no separate buffered flush. Default structured redaction bounds traversal, handles typed maps/structs and URL credentials, and masks common labeled free-text credentials. `SanitizeText` can enforce application-specific text policy; unlabeled arbitrary secrets cannot be detected reliably.
