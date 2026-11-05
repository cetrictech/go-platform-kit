# PostgreSQL

```go
client, err := postgres.New(ctx, postgres.Config{URL: dsn, MaxConnections:20})
// check err before deferring client.Close()
var rows []struct { ID int64 `db:"id"`; Name string `db:"name"` }
err = client.Select(ctx, &rows, "SELECT id,name FROM accounts WHERE id > $1", 0)
```

New parses the URL, configures pgxpool and pings before returning. Connection errors never echo the supplied configuration. URL sslmode is explicit; production should use verify-full or an appropriately verified TLS config. Zero pool settings retain driver defaults. MinConnections cannot exceed maximum. QueryTimeout covers client query stream lifetime; always Close rows. QueryRow executes on Scan. StatementTimeout is a server connection parameter in milliseconds. AfterConnect is called for each new connection. Observer accepts observability.Hook for query spans/metrics without raw SQL labels.

Get and Select map columns case-insensitively to exported fields or db tags; anonymous value structs flatten, unknown columns are discarded. Scalars require one column; empty Get returns sql.ErrNoRows. Drivers perform typed conversion, including nullable Scanner types, primitive pointers, UUID/time and supported native JSON/array representations. Arbitrary JSON-to-struct conversion and embedded pointer flattening are not promised. Do not pass nil destinations.

WithinTx takes TxOptions and a callback, commits only on nil, and rolls back on failure or panic. Nested Tx.WithinTx uses savepoints. Rollback cleanup has a separate five-second deadline. No arbitrary callback retries occur. Batch uses a protocol batch with an implicit transaction. CopyFrom uses quoted identifier components. Listen holds a dedicated connection until cancellation and cleans LISTEN state before pool release. AdvisoryLock also holds a connection; release exactly once. It is session-scoped and is not a fencing token.

Native exposes pgxpool for advanced tracing, connection hooks, prepared statements and types. SQLDB returns a database/sql bridge suitable for outbox.SQLStore; close that bridge before closing the pool. Primary/replica routing is application-owned in this initial release.

Verified TLS is required by default. `AllowPlaintext: true` is an explicit opt-in for isolated development/test infrastructure; it does not disable validation of a supplied TLS configuration. A custom certificate verifier, including go-spiffe verification, is supported.
