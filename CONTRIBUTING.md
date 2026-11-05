# Contributing

Use Go 1.26+ and run make test, race, vet, staticcheck, vuln and integration before release. Do not add optional drivers to root dependencies. Test each module with GOWORK=off. Add behavioral tests for error, cancellation and cleanup paths. Document resource ownership, delivery semantics, public error fields and compatibility changes. Keep callbacks context-aware. Never publish tags or releases without maintainer authorization. See docs/versioning.md for nested module tags.
