# system-design-lab

Hands-on system design lab: each phase builds a small architecture with Docker
Compose, puts load on it, breaks it on purpose and observes the failure. The
roadmap lives in `system_design_lab.md` (local only, excluded from git).

## Layout

- `app/`: the single reusable Go service (stdlib `net/http`, pgx, Prometheus
  metrics, slog JSON logs). One binary, behavior toggled by env vars; phases
  change compose files and env, not code.
- `observability/`: shared Prometheus, Grafana, Loki, Alloy and cAdvisor stack,
  included by every phase compose with `include:`.
- `NN_<phase>/`: one directory per phase (`00_base/`, `01_load_balancing/`, ...)
  with its compose, load script, README and diagrams.

## Commands

- `make help`: list targets.
- `make check`: go vet, tests with `-race`, golangci-lint (with the
  `integration` tag) and govulncheck. Must pass before every commit.
- `make test-integration`: integration tests against a throwaway Postgres
  container (removed afterwards; never the lab's database).
- `make up` / `make down` / `make logs`: the current phase stack, observability
  included (`PHASE=00_base` by default; `make up PHASE=01_load_balancing`).
- `make app-image`: build the app image.
- `make obs-up` / `make obs-down` / `make obs-logs`: observability stack only.

## Git

- Remote `git@github.com:fortega2/system-design-lab.git`; Go module
  `github.com/fortega2/system-design-lab/app`.
- Conventional branches (`feat/`, `fix/`, `docs/`, `chore/`); never commit to
  `main` directly.
- Small Conventional Commits, subject line only, each one green (builds, tests
  pass, nothing that worked before breaks).

## Workflow

Every change goes through the `lab-step` skill: pick skills and docs, propose,
write, verify, commit.
