# MySQL

```go
client, err := mysql.New(ctx, mysql.Config{DSN: dsn, MaxOpenConnections:20})
// check err, then defer client.Close()
err = client.WithinTx(ctx, mysql.TxOptions{}, func(tx *mysql.Tx) error {
    _, err := tx.Exec(ctx, "INSERT INTO events (id) VALUES (?)", eventID)
    return err
})
```

New builds a driver connector without global registration, forces parseTime, configures the SQL pool and pings. TLS configs are cloned and certificate verification cannot be disabled through Config.TLS. DSN settings remain explicit; production must enable verified TLS. A zero MaxOpenConnections selects 32 rather than database/sql's unlimited default. MaxIdleConnections must not exceed that open bound. Observer instruments wrapper operations. Native escape hatches and prepared statements do not automatically inherit wrapper instrumentation.

Mapping and empty-row behavior follow the shared database scanner; see postgres.md. MySQL uses ? placeholders, exposes LastInsertId through sql.Result and does not emulate PostgreSQL RETURNING. Batch executes sequential commands inside one transaction. WithinTx rolls back on callback errors and panics; commit failures are returned. Transactions do not retry automatically. Prepare returns an owned sql.Stmt that must be closed. Native returns the pool for advanced integrations and outbox.SQLStore. Duplicate key 1062 maps to conflict; deadlock 1213 and lock timeout 1205 indicate retryable failures, subject to operation idempotency.

Verified TLS is required by default. `AllowPlaintext: true` is an explicit opt-in for isolated development/test infrastructure; it does not disable validation of a supplied TLS configuration. A custom certificate verifier, including go-spiffe verification, is supported.

Per-client `Hooks`, safe `QueryLogger`, and `QueryTimeout` match the PostgreSQL adapter. `Prepare` returns a library-owned Statement with Exec/Query/QueryRow/Get/Select/Native/Close; this replaces the original pre-release raw statement return. `CommitContext` and `RollbackContext` check the caller context; the driver's transaction lifetime is still governed by its BeginTx context. `NewCluster` offers explicit primary/replica selection with Strong/Eventual consistency and partial-start cleanup. The SDK does not promote replicas or parse SQL to route queries.
