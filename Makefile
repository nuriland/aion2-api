.DEFAULT_GOAL := help
.PHONY: help check fmt vet lint test test-v cover e2e probe mcp

STATICCHECK := honnef.co/go/tools/cmd/staticcheck@v0.8.1

help: ## list targets
	@grep -hE '^[a-z0-9-]+:.*## ' $(MAKEFILE_LIST) | awk -F':.*## ' '{printf "  \033[1m%-8s\033[0m %s\n", $$1, $$2}'

check: fmt vet lint test ## the gate: gofmt, vet, staticcheck, tests under -race

fmt: ## fail if anything is not gofmt-clean
	@out=$$(gofmt -l .); [ -z "$$out" ] || { echo "gofmt needed:"; echo "$$out"; exit 1; }

vet: ## fail if anything is not go vet-clean
	go vet ./...

lint: ## staticcheck, pinned
	go run $(STATICCHECK) ./...

test: ## offline tests, race detector on, no cache
	go test ./... -race -count=1

test-v: ## same as test, but verbose
	go test ./... -race -count=1 -v

cover: ## coverage summary
	go test . ./internal/... -count=1 -coverpkg=.,./internal/... -coverprofile=cover.out
	@go tool cover -func=cover.out | tail -1
	@echo "  open it: go tool cover -html=cover.out"

e2e: ## real requests against NC, every region
	AION2_E2E=1 go test . -run E2E -count=1 -v -timeout 5m

probe: ## run the example end to end
	go run ./examples/probe $(ARGS)

mcp: ## run the MCP server on stdio, e.g. make mcp ARGS="-region eu -v"
	go run ./cmd/aion2-mcp $(ARGS)
