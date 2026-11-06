# Errors and diagnostic policy

```go
err := errors.Wrap(cause, "We could not complete your request.",
    errors.WithCode("DATABASE_ERROR"), errors.WithCategory(errors.Database))
```

AppError carries a public code/message, category, severity, HTTP status, safe Details/Metadata, retryability/delay and correlation IDs. DebugMessage and Cause are excluded from JSON. Standard errors.Is/As/Unwrap traverse causes. Core classification covers context and net errors; database adapters classify driver errors. gRPC translates application categories without adding gRPC to core.

Public Details and Metadata must be approved for external use. Error() is diagnostic and can include causes; do not return it verbatim to clients. response.Error hides technical details by default. Debug exposure requires DevelopmentOnly plus server configuration, or AuthorizedInternal plus a server-side authorizer. Both additionally require an explicit SanitizeDebug callback; no client header grants access. A sanitizer must remove credentials from arbitrary text.

Panic conversion is explicit: defer errors.Recover(&err, report). Without an error destination, the panic is re-raised. HTTP/gRPC recovery reports panics and returns a safe error; Kafka recovery returns an error so failed messages remain uncommitted.
