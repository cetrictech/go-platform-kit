# PostgreSQL

```go
client, err := postgres.New(ctx, postgres.Config{URL: dsn, MaxConnections:20})
// check err before deferring client.Close()
var rows []struct { ID int64 `db:"id"`; Name string `db:"name"` }
err = client.Select(ctx, &rows, "SELECT id,name FROM accounts WHERE id > $1", 0)
```

New parses the URL, configures pgxpool and pings before returning. Connection errors never echo the supplied configuration. URL sslmode is explicit; production should use verify-full or an appropriately verified TLS config. Zero pool settings retain driver defaults. MinConnections cannot exceed maximum. QueryTimeout covers client query stream lifetime; always Close rows. QueryRow executes on Scan. StatementTimeout is a server connection parameter in milliseconds. AfterConnect is called for each new connection. Observer accepts observability.Hook for query spans/metrics without raw SQL labels.

Get and Select map columns case-insensitively to exported fields or db tags; anonymous value structs flatten, unknown columns are discarded. Scalars require one column; empty Get returns sql.ErrNoRows. Drivers perform typed conversion, including nullable Scanner types, primitive pointers, UUID/time and supported native JSON/array representations. The shared scanner also accepts timestamp text in RFC3339 or zone-less UTC form (`2006-01-02 15:04:05`, `2006-01-02`) and `id.UUID` text or 16-byte values. NULL is an error for a `time.Time` value and becomes a zero `id.UUID`; pointer fields of either type become nil. Arbitrary JSON-to-struct conversion and embedded pointer flattening are not promised. Do not pass nil destinations.

WithinTx takes TxOptions and a callback, commits only on nil, and rolls back on failure or panic. Nested Tx.WithinTx uses savepoints. Rollback cleanup has a separate five-second deadline. No arbitrary callback retries occur. Batch uses a protocol batch with an implicit transaction. CopyFrom uses quoted identifier components. Listen holds a dedicated connection until cancellation and cleans LISTEN state before pool release. AdvisoryLock also holds a connection; release exactly once. It is session-scoped and is not a fencing token.

Native exposes pgxpool for advanced tracing, connection hooks, prepared statements and types. SQLDB returns a database/sql bridge suitable for outbox.SQLStore; close that bridge before closing the pool. Primary/replica routing is application-owned in this initial release.

Verified TLS is required by default. `AllowPlaintext: true` is an explicit opt-in for isolated development/test infrastructure; it does not disable validation of a supplied TLS configuration. A custom certificate verifier, including go-spiffe verification, is supported.

`NamedArgs` supplies library-owned strict named arguments: `client.Exec(ctx, "UPDATE accounts SET name=@name WHERE id=@id", postgres.NamedArgs{"name": name, "id": id})`. Missing or unused names fail. `Hooks.BeforeBegin`, `AfterQuery` and `AfterTransaction` are explicit per-client callbacks. QueryLogger emits operation, duration and outcome without SQL or parameters. Hooks receive trusted SQL text for application-controlled inspection; keep callbacks prompt and safe.

The package exports `Row`, `Rows`, and `Executor` aliases/interfaces for application-owned repository boundaries. Use `postgres.IsNoRows(err)` instead of importing pgx error values, and `postgres.ConstraintError(err)` to classify PostgreSQL unique, foreign-key, and check-constraint failures without exposing driver details in the service layer.

`Prepare(ctx, sql)` returns an owned statement with Exec/Query/QueryRow/Get/Select and Native. Close statements and `SQLDB()` bridges before the pool. Deferred QueryRow handles support one Scan and never execute twice. The cached scanner flattens exported embedded pointer structs, supports sql.Scanner and `db:"field,json"`, and rejects ambiguous mappings. `database.JSON[T]` distinguishes SQL NULL from valid JSON null.

`NewCluster(ctx, ClusterConfig{Primary: ..., Replicas: ...})` opens explicitly configured pools and cleans up partial startup. Writer always selects the primary; Reader(Strong) selects primary and Reader(Eventual) round-robins replicas configured read-only. It never infers consistency by parsing SQL. Replica topology, replication and promotion remain operator-owned.
