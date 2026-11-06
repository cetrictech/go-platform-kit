# Dependency upgrades and vulnerability remediation

Direct and transitive versions are selected in each module's go.mod/go.sum. Optional infrastructure/JWT/telemetry dependencies belong in their own modules; adding them to core requires architectural review. Never use workspace resolution as evidence that a module resolves independently. Development replace directives are not inherited by downstream consumers.

## Upgrade procedure

1. Review stable upstream releases and security advisories at least monthly; monitor Go vulnerability notices and dependency alerts continuously. Verify release notes, minimum Go changes, license and native API changes from primary upstream sources. Do not select prereleases implicitly or replace stable releases with pseudo-versions without an explicit security justification.
2. Update one independently versioned module or a reviewed related group. Reconcile `go mod tidy` with `GOWORK=off`; inspect every direct/transitive change and unexpected native-driver exposure. Update [dependency selections](dependencies.md) and [the adapter matrix](compatibility-matrix.md).
3. Run source API comparison with `scripts/check-api.sh`, wire/error contract tests, independent copied-module/consumer tests, supported Go lines, race detector and static analysis. Test each changed infrastructure adapter against real services, including cancellation, restart/outage recovery, cleanup and credential policy where applicable.
4. Run repeated critical-path benchmarks and the performance-regression gate on the same runner class. Investigate allocation growth, retained buffers and goroutine/connection changes. A changed baseline must explain the cause and tradeoff in review, not merely accept a slowdown.
5. Classify source and behavioral compatibility under [semantic versioning](versioning.md), document deprecations/migration, and run the fail-closed release gate. Every supported module's graph receives its own vulnerability scan; fixing another module's graph is not a remediation.

## Security handling and deadlines

Report exploitable details privately as described in [SECURITY.md](../SECURITY.md). Triage starts within two business days of a report. Classify reachability, attacker control, exposed deployments and affected supported versions; an unreachable transitive advisory is recorded separately from a reachable issue, never hidden by globally disabling scanners.

Target a reviewed fix or concrete mitigation within 48 hours for critical reachable issues, seven calendar days for high severity, 30 days for medium, and the next routine maintenance cycle for low severity. These are remediation targets, not a claim that every upstream fix is immediately available. If no safe fix exists, notify affected users privately, document mitigation and restrict the affected release. Do not ship a known reachable vulnerability merely to meet a release date.

A security exception must be narrow: advisory ID, module/version range, reachability evidence, mitigation, owner/reviewer, reason, and an expiry no later than 30 days. Review it before every release. Any implemented release scanner exception must match that record exactly; an unsupported or expired exception does not bypass a gate. The default release policy permits zero reachable advisories. Preserve raw scanner output and its database/tool versions with release evidence. Go standard-library advisories require a patched supported toolchain as well as module review.

Emergency compatible security fixes use a patch version. If remediation requires a source/wire break, ship an available compatible mitigation first where practical, communicate the security reason and migration path, and follow Go semantic import versioning rather than reusing a published tag. Never rewrite or delete a published module tag to replace a vulnerable artifact.
