# Pagination

Offset uses page >= 1 and limits 1..1000, defaulting to page 1/limit 20 when parsed from a request. Validate before Skip. NewOffsetMeta computes page counts without addition overflow.

Codec signs versioned JSON with HMAC-SHA256. Set ActiveKey and Keys with at least 32 random bytes per key. Keep retired verification keys until all old cursors expire. Default maximum decoded payload is 4096 bytes. Bind Query to a stable query identity and Filter to a canonical filter hash. TTL is optional; configure it to restrict replay duration. Cursors are authenticated, not encrypted: clients can read values.

Keyset accepts only allowlisted identifiers, binds every value, supports NULL-first/last, mixed directions and composite keys, and requires a unique final tie-breaker. Supply decoded cursor key values in sort order. PostgreSQL start offsets placeholders for earlier parameters; MySQL uses ?. `previous=true` reverses sorting and predicates; reverse fetched rows before presenting them. Fetch limit+1 and trim the extra row before creating navigation cursors. Never expose cursors derived from rows excluded from the response.

Keyset does not provide snapshot isolation across requests. Concurrent updates to sort keys can move rows across pages; inserts/deletes change the visible set. Use immutable keys or a database snapshot when stricter consistency is needed. Codec authenticates cursor integrity; developers must still validate key types/arity against their query.

`FetchCursor[T]` composes a codec with explicit trusted `Query` templates containing `{{where}}`, `{{order}}` and `{{limit}}`. Columns and sort fields use an allowlist; `KeyType` describes exact string, int64, float64, timestamp, UUID, boolean or byte cursor values. Int64 keys never pass through floating point. The helper fetches limit+1, reverses backward presentation, and optionally counts totals. Final unique keys cannot be NULL. SQL remains application-owned; parameters remain bound.

Offset HTTP parameters use `page` and `page_size`; `limit` remains a deprecated alias and conflicting values fail. Offset metadata includes type, total_records, total_pages, page and page_size. `OffsetConfig` can cap page size and maximum offset. `OffsetSQL` binds explicit template limits/offsets. `KeyRing.Rotate` atomically changes active signing keys while retaining explicitly supplied old verification keys; context-aware codec methods support external key providers.
