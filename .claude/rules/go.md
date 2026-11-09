---
paths:
  - "app/**"
---

# Go app rules

- Before writing Go, load the matching `cc-skills-golang:*` skills for the area
  touched (`golang-how-to` picks them). Look up Go library APIs with godig
  (`golang-pkg-go-dev`), not from memory.
- Database integration tests use the `//go:build integration` tag and run
  against a real ephemeral Postgres (`postgres:18.6-alpine`); unit tests use
  fakes of the consumer-side interfaces.
- The runtime image is distroless: no shell, no wget. Health checks use the
  binary's own `-healthcheck` flag; ad-hoc network probes need a throwaway
  `busybox` container on the right network.
- `make check` must pass (vet, `-race` tests, golangci-lint with and without
  the `integration` tag, govulncheck).
