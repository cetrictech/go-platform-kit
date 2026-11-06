# Security policy

The SDK is unreleased and has no supported production release series yet. Do not treat the development version or a passing unit suite as a production readiness certification. Published-series and Go/infrastructure support commitments are defined in [support policy](docs/support-policy.md).

Report security issues privately through the repository maintainer's GitHub security advisory channel. Do not include production credentials or exploitable details in public issues. If private reporting is unavailable, contact the maintainer privately before disclosure. Include affected module/version, Go/driver/server versions, reachability, impact, a minimal reproduction and any safe mitigation. Keep exploit material and user data private until coordinated disclosure.

Triage starts within two business days. Remediation targets are 48 hours for critical reachable issues, seven days for high severity, 30 days for medium, and routine maintenance for low severity. If no safe upstream fix exists, maintainers must document mitigation and restrict affected releases. These targets do not justify bypassing release gates. [Dependency policy](docs/dependency-policy.md) defines reachability review, narrowly scoped time-limited exceptions, security backports and upgrade validation.

Never commit production keys, tokens or connection strings. Integration credentials and short-lived generated keys are local test-only material; generated TLS files remain ignored by Git. Apply verified TLS and explicit peer/issuer/audience policy. Public response contracts hide diagnostics by default; log/error redaction cannot reliably detect arbitrary unlabeled secrets. See [security boundaries](docs/security.md) and [public API ownership contracts](docs/public-api-contracts.md).

Every releasable module receives an independent vulnerability scan. The release gate rejects reachable vulnerabilities and expired/unreviewed exceptions; preserve scanner/database evidence. An unused advisory in a required module must still be recorded and monitored. Published vulnerable tags are immutable: issue a fixed version or advisory, never replace the original artifact.
