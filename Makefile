LIST_ALL := $(shell go list ./... | grep -v vendor | grep -v mocks)

all: lint test

.PHONY: lint
lint: ## Lint the files
	@golangci-lint version
	@golangci-lint run --timeout 3m0s

.PHONY: test
test: ## Run unit tests
	@go test -short -count 1 -v ./...

.PHONY: race
race: ## Run data race detector
	@go test -race -count 1 -v ./...

.PHONY: coverage
coverage: ## Generate coverage report
	@go-acc ./...
	@go tool cover -func=coverage.txt

.PHONY: depcheck
depcheck: ## Check dependencies for vulnerabilities
	@go list -json -deps ./... | nancy sleuth

.PHONY: licensecheck
licensecheck: ## Check dependencies for forbidden licenses
	@go-licenses check $$(go list -deps -test ./... | grep -v "\.test")

.PHONY: upgrade
upgrade: ## Upgrade the dependencies
	@go get -u -t ./...
	@go mod tidy
	@go mod vendor

.PHONY: clean
clean: ## Remove outdated file and empty cache
	@rm -rf "$(go env GOCACHE)"
	@rm -f coverage.*
	@rm -f ./bin/*

.PHONY: help
help: ## Display this help screen
	@grep -h -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'
