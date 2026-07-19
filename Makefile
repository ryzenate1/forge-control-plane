.PHONY: help lint format test build clean

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

lint: ## Run all linters
	./scripts/lint.sh

format: ## Format all code
	./scripts/format.sh

test: ## Run all tests
	cd forge/api && go test ./... && cd ../..
	cd beacon && go test ./... && cd ../..
	cd forge/web && npm test && cd ../..

build: ## Build all components
	cd forge/api && go build ./cmd/api && cd ../..
	cd beacon && go build ./cmd/daemon && cd ../..
	cd forge/web && npm run build && cd ../..

api-test: ## Run only API tests
	cd forge/api && go test -v ./... && cd ../..

beacon-test: ## Run only Beacon tests
	cd beacon && go test -v ./... && cd ../..

web-test: ## Run only Web tests
	cd forge/web && npm test && cd ../..

clean: ## Clean build artifacts
	cd forge/api && go clean && cd ../..
	cd beacon && go clean && cd ../..
	rm -rf forge/web/.next
