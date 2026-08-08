.PHONY: help lint format test build clean api-test beacon-test web-test

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

lint: ## Run all linters
	./scripts/dev/lint.sh

format: ## Format all code
	./scripts/dev/format.sh

test: ## Run all tests
	@set -e; cd forge/api && go test -race -timeout 10m -count=1 ./...
	@set -e; cd beacon && go test -race -timeout 10m -count=1 ./...
	@set -e; cd forge/web && npm test

build: ## Build all components
	cd forge/api && go build ./cmd/api && cd ../..
	cd beacon && go build ./cmd/daemon && cd ../..
	cd forge/web && npm run build && cd ../..

api-test: ## Run only API tests
	cd forge/api && go test -v -race -timeout 10m -count=1 ./... && cd ../..

beacon-test: ## Run only Beacon tests
	cd beacon && go test -v -race -timeout 10m -count=1 ./... && cd ../..

web-test: ## Run only Web tests
	cd forge/web && npm test && cd ../..

clean: ## Clean build artifacts
	cd forge/api && go clean && cd ../..
	cd beacon && go clean && cd ../..
	rm -rf forge/web/.next
