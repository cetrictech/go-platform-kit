module github.com/cetrictech/go-platform-kit/database/postgres

go 1.26.0

toolchain go1.27.3

require (
	github.com/cetrictech/go-platform-kit v0.1.0
	github.com/jackc/pgx/v5 v5.12.0
)

require (
	github.com/google/uuid v1.6.0 // indirect
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/jackc/puddle/v2 v2.2.3 // indirect
	github.com/stretchr/testify v1.12.1 // indirect
	golang.org/x/sync v0.24.0 // indirect
	golang.org/x/text v0.43.0 // indirect
)

replace github.com/cetrictech/go-platform-kit => ../..
