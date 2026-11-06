.PHONY: test race vet tidy fmt check-fmt staticcheck vuln integration services services-down bench coverage verify api api-surface independent gates faults performance release-check

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
	./scripts/extended-services.sh
services-down:
	docker compose -f integration/compose.yaml down
	./scripts/extended-services.sh down
integration:
	./scripts/integration.sh
bench:
	./scripts/modules.sh go test -run='^$$' -bench=. -benchmem ./...
coverage:
	./scripts/modules.sh go test -cover ./...
verify: check-fmt gates api api-surface vet test race staticcheck vuln independent

api:
	./scripts/check-api.sh
api-surface:
	./scripts/check-api-surface.sh
independent:
	./scripts/check-independent.sh
faults:
	./scripts/fault-integration.sh
performance:
	./scripts/check-performance.sh
gates:
	python3 -m unittest discover -s scripts/tests -p 'test_*.py' -v
	python3 -m unittest discover -s scripts -p 'test_check_*.py' -v
release-check:
	./scripts/release-check.sh
