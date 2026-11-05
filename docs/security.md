# Security

RandomBytes/Token use crypto/rand and bound allocation to 1 MiB. HMAC signing requires at least 32 key bytes, uses SHA-256 and constant-time verification. SHA256 is a digest, not a password hash. HashPassword uses bcrypt with explicitly configured cost 10..16; empty passwords are rejected and bcrypt rejects inputs exceeding 72 bytes. CheckPassword uses the maintained implementation. Zero clears a supplied byte slice but cannot erase copies or guarantee compiler/runtime memory erasure.

Use verified TLS for all remote production infrastructure. Local integration compose deliberately uses plaintext and test-only credentials on loopback. Never reuse these credentials. Native escape hatches can weaken driver guarantees and require application review.

Structured redaction cannot reliably detect secrets embedded inside arbitrary free text. Keep raw payloads, connection strings and credentials out of log messages, public error fields, cursor values and outbox headers. Cursor payloads are signed but readable. Stored outbox events and Redis JSON are not automatically encrypted; encryption/access control/retention must be configured by the application.

Redis ownership leases are not fencing. Idempotency does not make arbitrary external effects exactly once; retain durable unique business keys at the authoritative system. Report vulnerabilities privately as described in SECURITY.md.
