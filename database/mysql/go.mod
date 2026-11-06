module github.com/cetrictech/go-platform-kit/database/mysql

go 1.26.0

toolchain go1.27.3

require (
	github.com/cetrictech/go-platform-kit v0.1.0
	github.com/go-sql-driver/mysql v1.10.1
)

require (
	filippo.io/edwards25519 v1.2.0 // indirect
	github.com/google/uuid v1.6.0 // indirect
)

replace github.com/cetrictech/go-platform-kit => ../..
