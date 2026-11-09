---
paths:
  - "**/compose.yaml"
  - "observability/**"
  - "Makefile"
---

# Compose, observability and Makefile rules

Write configs for Prometheus, Grafana, Loki, Alloy, cAdvisor, Compose, k6 and
D2 from current docs (Context7), never from memory. A hook validates every
`compose.yaml` after it is edited.

## Every phase compose

- Name the project `lab-...` (e.g. `name: lab-00-base`). Prometheus drops
  cAdvisor series from projects other than `observability` / `lab-*`; this also
  keeps the user's unrelated containers out of Loki and Prometheus.
- Start with `include: [../observability/compose.yaml]`.
- No `restart:` policy. The stack starts only through `make`, never on boot,
  and a crashed container must stay down so the failure is visible.
- Publish ports on `127.0.0.1` only.
- Scraping is label-based: `lab.scrape=true` plus `lab.metrics_port` (default
  8080). Never edit `prometheus.yml` per phase.
- Services that expose metrics or logs join the `lab` network (Alloy only sees
  containers on `lab`).

## Networks in observability/compose.yaml

- `lab`: phase services, Prometheus, Loki, Alloy, cAdvisor.
- `obs`: Grafana, Prometheus, Loki. Grafana is not on `lab`.
- `docker-api` (internal): socket-proxy, Prometheus and Alloy only. The
  container list it serves includes every container's environment (secrets),
  so nothing else may join it.
- Grafana: anonymous Admin, bound to 127.0.0.1, with
  `GF_SERVER_DOMAIN=localhost` and `GF_SERVER_ENFORCE_DOMAIN=true` against DNS
  rebinding.

## Makefile and mounted configs

The user's shell runs `umask 077` on purpose, so new files are 600 and
non-root containers can't read bind-mounted configs. Every directory with
mounted config files goes in `CONFIG_DIRS`, and every `*-up` target depends on
`perms` (`chmod -R a+rX $(CONFIG_DIRS)`). Run `make perms` before verifying by
hand.
