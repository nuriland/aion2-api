.DEFAULT_GOAL := help
.PHONY: help check fmt vet test test-v cover e2e probe

help: ## list targets
	@grep -hE '^[a-z-]+:.*## ' $(MAKEFILE_LIST) | awk -F':.*## ' '{printf "  \033[1m%-8s\033[0m %s\n", $$1, $$2}'

check: fmt vet test ## the gate: gofmt, vet, tests under -race

fmt: ## fail if anything is not gofmt-clean
	@out=$$(gofmt -l .); [ -z "$$out" ] || { echo "gofmt needed:"; echo "$$out"; exit 1; }

vet: ## fail if anything is not go vet-clean
	go vet ./...

test: ## offline tests, race detector on, no cache
	go test ./... -race -count=1

test-v: ## same as test, but verbose
	go test ./... -race -count=1 -v

cover: ## coverage summary, per function
	go test . ./internal/... -count=1 -coverprofile=cover.out
	@go tool cover -func=cover.out | tail -1
	@echo "  open it: go tool cover -html=cover.out"

e2e: ## ~15 real requests against NC, both regions
	AION2_E2E=1 go test . -run E2E -count=1 -v -timeout 5m

probe: ## run the example end to end
	go run ./examples/probe $(ARGS)
