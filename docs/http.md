# Native HTTP

Use response.OK(w,data), Created, Accepted, NoContent, Paginated or Error(w,r,err). JSON(w,status,value,options) returns encoding/write errors. The default atomic response limit is 1 MiB. Encoding failures occur before headers are committed; Go's JSON encoder can still allocate intermediate memory proportional to the input. Stream large responses through net/http directly. 204 and 304 have no body.

Share a response.Writer across all writers to detect repeat writes. Writes made directly to its underlying ResponseWriter cannot be detected. Timestamp values use UTC. Supply trace/correlation fields through Envelope or AppError. JSON accepts arbitrary envelope values and EscapeHTML controls encoding.

`httpx/middleware.Chain` applies left-to-right on requests. Place recovery outside handlers, security headers and request IDs before logging; authenticate before authorization and idempotency. BodyLimit bounds request reading. Timeout uses the standard buffered TimeoutHandler and is unsuitable for streaming. ContentType validates parsed MIME types. CORS rejects wildcard origins with credentials. ClientIP peels forwarded chains only from configured trusted proxy CIDRs. Forwarded request IDs and user identities are never trusted automatically.

`httpx/client.New` defaults to a 30 s request timeout, 5 s dial timeout, 90 s idle timeout, 100 idle connections and a 4 MiB JSON response limit. JSON closes response bodies. Do returns a streaming response whose body the caller must close. Close only closes owned idle transport connections. No automatic retry of non-idempotent requests is performed.

Compression negotiates gzip (including q=0), skips upgrades and avoids bodies on 204/304. validation.DecodeJSON consumes/closes a bounded request body, rejects unknown fields and trailing documents, then validates the destination. validation.ApplicationError maps safe field failures into an AppError suitable for response.Error.
