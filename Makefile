.PHONY: test race vet tidy fmt check-fmt staticcheck vuln integration services services-down bench coverage verify

test:
	./scripts/modules.sh go test ./...
race:
	./scripts/modules.sh go test -race ./...
vet:
	./scripts/modules.sh go vet ./...
tidy:
	./scripts/modules.sh go mod tidy
fmt:
	gofmt -w $$(git ls-files --cached --others --exclude-standard '*.go')
check-fmt:
	@test -z "$$(gofmt -l $$(git ls-files --cached --others --exclude-standard '*.go'))"
staticcheck:
	./scripts/modules.sh staticcheck ./...
vuln:
	./scripts/modules.sh govulncheck ./...
services:
	docker compose -f integration/compose.yaml up -d --wait
services-down:
	docker compose -f integration/compose.yaml down
integration:
	./scripts/integration.sh
bench:
	./scripts/modules.sh go test -run='^$$' -bench=. -benchmem ./...
coverage:
	./scripts/modules.sh go test -cover ./...
verify: check-fmt vet test race staticcheck vuln
