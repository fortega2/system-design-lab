.DEFAULT_GOAL := help
SHELL := /bin/bash

APP_DIR     := app
APP_IMAGE   := sdlab/app:dev
OBS_COMPOSE := docker compose -f observability/compose.yaml

# Phase the up/down/logs targets act on: make up PHASE=01_load_balancing
PHASE         ?= 00_base
PHASE_COMPOSE := docker compose -f $(PHASE)/compose.yaml
LOAD_SCRIPT   ?= constant.js

# Diagrams: D2 installed on the host (sudo pacman -S d2). tala lays these
# out best; the light theme switches to the dark one with the viewer's OS.
D2_FLAGS := --layout=tala --theme=0 --dark-theme=200 --pad=20

# Disposable Postgres for the integration tests (never the lab's database).
TEST_PG       := sdlab-test-postgres
TEST_PG_IMAGE := postgres:18.6-alpine
TEST_PG_PORT  := 55432

# Directories with config files bind-mounted into containers. Containers run
# as other (non-root) users, so these must be world-readable even when the
# shell umask is restrictive (e.g. 077).
CONFIG_DIRS := observability 00_base app/schema

.PHONY: help
help: ## Show available targets
	@grep -hE '^[a-zA-Z0-9_-]+:.*?## ' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2}'

.PHONY: check
check: ## Run vet, tests, lint and vulnerability scan on the app
	cd $(APP_DIR) && go vet ./... && go test -race ./... && \
		golangci-lint run --build-tags=integration ./... && govulncheck ./...

.PHONY: test-integration
test-integration: ## Run the integration tests against a throwaway Postgres
	@trap 'docker rm -f $(TEST_PG) >/dev/null' EXIT; \
	docker run -d --rm --name $(TEST_PG) -e POSTGRES_PASSWORD=test \
		-p 127.0.0.1:$(TEST_PG_PORT):5432 $(TEST_PG_IMAGE) >/dev/null; \
	for _ in {1..30}; do \
		docker exec $(TEST_PG) pg_isready -h 127.0.0.1 -U postgres -q 2>/dev/null && break; sleep 1; \
	done; \
	cd $(APP_DIR) && DATABASE_URL=postgres://postgres:test@127.0.0.1:$(TEST_PG_PORT)/postgres \
		go test -race -count=1 -tags=integration ./...

.PHONY: app-image
app-image: ## Build the app container image
	docker build -t $(APP_IMAGE) $(APP_DIR)

.PHONY: perms
perms: ## Make mounted config files readable by container users
	chmod -R a+rX $(CONFIG_DIRS)

.PHONY: up
up: perms ## Start the phase stack, rebuilding the app (PHASE=00_base; phase knobs e.g. PG_CPUS=2)
	$(PHASE_COMPOSE) up -d --build

.PHONY: down
down: ## Stop the phase stack (keeps its data volumes)
	$(PHASE_COMPOSE) down

.PHONY: logs
logs: ## Follow the phase stack logs
	$(PHASE_COMPOSE) logs -f

.PHONY: load
load: perms ## Run the phase k6 script against the running stack (RATE=200 DURATION=5m MAX_VUS=500)
	$(PHASE_COMPOSE) run --rm --no-deps k6 run /scripts/$(LOAD_SCRIPT)

.PHONY: diagrams
diagrams: ## Render every NN_*/diagrams/*.d2 to SVG (needs d2 installed)
	@command -v d2 >/dev/null || { echo "d2 not found: sudo pacman -S d2" >&2; exit 1; }
	@for f in [0-9][0-9]_*/diagrams/*.d2; do \
		d2 $(D2_FLAGS) "$$f" "$${f%.d2}.svg" || exit 1; \
	done

.PHONY: obs-up
obs-up: perms ## Start only the observability stack (Grafana :3000, Prometheus :9090)
	$(OBS_COMPOSE) up -d

.PHONY: obs-down
obs-down: ## Stop the observability stack (keeps its data volumes)
	$(OBS_COMPOSE) down

.PHONY: obs-reload
obs-reload: perms ## Reload the Prometheus config after editing it (SIGHUP)
	$(OBS_COMPOSE) kill -s HUP prometheus

.PHONY: obs-logs
obs-logs: ## Follow the observability stack logs
	$(OBS_COMPOSE) logs -f
