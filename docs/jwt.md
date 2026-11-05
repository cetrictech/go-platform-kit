# Optional JWT and JWKS authentication

Import `github.com/cetrictech/go-platform-kit/security/jwt` independently. The root and gRPC modules do not depend on a JWT implementation.

Construct `jwt.New(ctx, jwt.Config{Issuer: issuer, Audience: []string{audience}, Algorithms: []string{"RS256"}, JWKSURL: "https://identity.example/keys"})`. Construction fetches keys with the caller's context and a finite HTTP timeout. Close the verifier during shutdown. A supplied HTTP client remains caller-owned.

`Verify(ctx, token)` requires a signature, explicit allowed algorithm, expiration, issuer and audience. Present `nbf` and `iat` claims are checked. Static keys require the same policy; HMAC keys have algorithm-dependent minimum lengths. No token-supplied key URLs are fetched. Remote keys must be asymmetric and use supported RSA, NIST EC or Ed25519 encodings. Unknown critical extensions and ambiguous/missing key IDs fail closed by default.

The bounded JWKS cache refreshes on expiration or unknown key ID, throttles repeated unknown-key refreshes, and offers explicit `Refresh(ctx)` for an administrator-controlled rotation. Expired keys are not accepted during a fetch outage. Deploy overlapping old/new keys for the cache lifetime when rotating. Configure custom TLS roots through an injected HTTP transport. Plain HTTP requires explicit development opt-in.

Use `verifier.Middleware(next)` for HTTP, or `grpcx.BearerAuthentication(verifier.Authenticate)` in the gRPC server config. `jwt.FromContext(ctx)` returns a copy of verified claims. Authentication establishes identity; application authorization remains explicit. Token strings and verification diagnostics must not be logged or returned in public errors.
