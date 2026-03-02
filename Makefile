.PHONY: run test install-tools

GOTESTSUM := $(shell go env GOPATH)/bin/gotestsum

# Packages to measure coverage — excludes testutil (test helper) and cmd/cli (entrypoint)
COVERPKGS := $(shell go list ./internal/... | grep -v '/testutil' | tr '\n' ',' | sed 's/,$$//')

run:
	go run ./cmd/cli/

test:
	$(GOTESTSUM) --format testdox -- \
		-coverprofile=coverage.out \
		-covermode=atomic \
		-coverpkg=$(COVERPKGS) \
		./...
	@echo ""
	@go tool cover -func=coverage.out | grep -v '/testutil'

install-tools:
	go install gotest.tools/gotestsum@latest
