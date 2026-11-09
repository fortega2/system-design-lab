.DEFAULT_GOAL := help
SHELL := /bin/bash

APP_DIR     := app
APP_IMAGE   := sdlab/app:dev
OBS_COMPOSE := docker compose -f observability/compose.yaml

# Directories with config files bind-mounted into containers. Containers run
# as other (non-root) users, so these must be world-readable even when the
# shell umask is restrictive (e.g. 077).
CONFIG_DIRS := observability

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

.PHONY: perms
perms: ## Make mounted config files readable by container users
	chmod -R a+rX $(CONFIG_DIRS)

.PHONY: obs-up
obs-up: perms ## Start only the observability stack (Grafana :3000, Prometheus :9090)
	$(OBS_COMPOSE) up -d

.PHONY: obs-down
obs-down: ## Stop the observability stack (keeps its data volumes)
	$(OBS_COMPOSE) down

.PHONY: obs-logs
obs-logs: ## Follow the observability stack logs
	$(OBS_COMPOSE) logs -f
