.PHONY: help build fmt check test

help:
	@echo Argus engineering-foundation command surface
	@echo   make build  Compile all Go packages
	@echo   make fmt    Format all Go packages
	@echo   make check  Run Go static analysis
	@echo   make test   Run Go tests

build:
	go build -o bin/ ./cmd/control-plane

fmt:
	go fmt ./...

check:
	go vet ./...

test:
	go test ./...
