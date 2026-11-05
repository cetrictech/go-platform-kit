# MySQL

```go
client, err := mysql.New(ctx, mysql.Config{DSN: dsn, MaxOpenConnections:20})
// check err, then defer client.Close()
err = client.WithinTx(ctx, mysql.TxOptions{}, func(tx *mysql.Tx) error {
    _, err := tx.Exec(ctx, "INSERT INTO events (id) VALUES (?)", eventID)
    return err
})
```

New builds a driver connector without global registration, forces parseTime, configures the SQL pool and pings. TLS configs are cloned and certificate verification cannot be disabled through Config.TLS. DSN settings remain explicit; production must enable verified TLS. Pool size zero retains database/sql defaults. Observer instruments wrapper operations. Native escape hatches and prepared statements do not automatically inherit wrapper instrumentation.

Mapping and empty-row behavior follow the shared database scanner; see postgres.md. MySQL uses ? placeholders, exposes LastInsertId through sql.Result and does not emulate PostgreSQL RETURNING. Batch executes sequential commands inside one transaction. WithinTx rolls back on callback errors and panics; commit failures are returned. Transactions do not retry automatically. Prepare returns an owned sql.Stmt that must be closed. Native returns the pool for advanced integrations and outbox.SQLStore. Duplicate key 1062 maps to conflict; deadlock 1213 and lock timeout 1205 indicate retryable failures, subject to operation idempotency.

Verified TLS is required by default. `AllowPlaintext: true` is an explicit opt-in for isolated development/test infrastructure; it does not disable validation of a supplied TLS configuration. A custom certificate verifier, including go-spiffe verification, is supported.
