# Deprecation and migration policy

Public source APIs, error codes, JSON field names/types/omission rules, HTTP/gRPC mappings, pagination cursors and resource lifecycle behavior are compatibility contracts. Keep interfaces small; add optional interfaces instead of adding required methods. A source-compatible change can still be a wire or ownership break and needs the same review.

Add a Go `Deprecated:` comment to the affected exported symbol identifying its replacement. Add an entry to `release/deprecations.json` (`schema_version: 1`, `deprecations` array) containing `module`, `symbol`, `since`, `remove_in`, `replacement`, and `migration_url`. `since` is the first version warning consumers; `remove_in` is the earliest proposed removal, not permission to remove before gates pass. Link concrete before/after examples and any changed cancellation, ownership, error or wire semantics. Record announced support dates when applicable.

Maintain the old API for at least two released minor versions before removal when practical. Stable modules remove it only in a new major import path; v0 removes it only in a new minor with an explicit migration record. The API checker must not silently bless removal merely because a version number was increased. Security emergencies can shorten notice only with a documented advisory/mitigation and maintainer approval; they do not authorize reusing a published tag or bypassing Go's import-path rules.

Before changing a response/error contract, preserve fixtures for every supported series and test the previous consumer assumptions. New optional metadata should use documented omission rules and must not require old consumers to understand it. Do not rename established codes to match a changed underlying driver message. Debug/private causes are never made public as an additive convenience.

The initial `release/deprecations.json` is empty because no module has been released; do not invent historical deprecation versions. The existing pre-release `limit` pagination alias is described in the [migration guide](migration-guide.md); retaining it is part of the initial candidate contract. Any future removal needs a real announced deprecation entry. Existing pre-release changes to statements, QueryRow and collector URL configuration are also documented in that guide.

Run `scripts/check-api.sh` against previous module-specific release tags and `scripts/release-check.sh` before proposing removal. A bootstrap API baseline when no previous tags exist cannot establish that real consuming projects have migrated.
