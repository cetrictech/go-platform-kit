# SPIFFE Workload API

New requires a TrustDomain and either exact AllowedPeers or an explicit Authorize callback. Socket accepts a Workload API address such as unix:///run/spire/agent/public/api.sock; when omitted, go-spiffe's environment-based endpoint selection applies. ExpectedID optionally pins the acquired workload identity. Peer allowlist entries outside the configured trust domain are rejected.

The maintained go-spiffe X509Source streams SVIDs and trust bundles and handles reconnection/rotation. ClientTLS and ServerTLS reference that live source; do not copy certificates into static TLS configs. Keep Source open until all connections are closed. Identity returns the currently acquired SPIFFE ID. TLS authentication and the configured peer authorization are separate from end-user authentication and business permissions.

Tests run a local gRPC implementation of the standard Workload API, issue real certificates, complete a mutual TLS handshake and verify identity rotation. This validates protocol integration, not a deployed SPIRE agent, node attestation or production issuance policy. Run deployment-specific tests against your SPIRE socket before release.
