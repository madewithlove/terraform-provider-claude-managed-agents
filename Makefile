BINARY  := terraform-provider-claude-managed-agents
VERSION ?= dev
# Local install target for terraform CLI dev_overrides.
GOBIN   := $(shell go env GOBIN)
ifeq ($(GOBIN),)
GOBIN := $(shell go env GOPATH)/bin
endif

.PHONY: build install fmt vet tidy test validate clean

build: ## Compile the provider binary
	go build -ldflags="-X main.version=$(VERSION)" -o $(BINARY) .

install: ## Install the provider into GOBIN for dev_overrides
	go install -ldflags="-X main.version=$(VERSION)" .

fmt: ## Format Go sources
	gofmt -w .

vet: ## Run go vet
	go vet ./...

tidy: ## Tidy modules
	go mod tidy

test: ## Run unit tests
	go test ./... $(TESTARGS)

validate: install ## Validate example config against the schema (no API calls)
	./scripts/validate.sh

clean:
	rm -f $(BINARY)
	rm -rf dist/
