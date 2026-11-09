.DEFAULT_GOAL := help
SHELL := /bin/bash

APP_DIR   := app
APP_IMAGE := sdlab/app:dev

.PHONY: help
help: ## Show available targets
	@grep -hE '^[a-zA-Z0-9_-]+:.*?## ' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2}'

.PHONY: check
check: ## Run vet, tests, lint and vulnerability scan on the app
	cd $(APP_DIR) && go vet ./... && go test -race ./... && \
		golangci-lint run --build-tags=integration ./... && govulncheck ./...

.PHONY: app-image
app-image: ## Build the app container image
	docker build -t $(APP_IMAGE) $(APP_DIR)
