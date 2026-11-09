---
name: lab-step
description: Workflow for every change in system-design-lab (code, compose, configs, docs). Use before starting any commit-sized step - picking skills and docs, proposing, writing, verifying, committing, and handling review fixes.
---

# One lab step = one small commit

## 1. Prepare

- Load the skills for the area being touched:
  - Go code: `cc-skills-golang:golang-how-to` plus the specific skills it
    points to (testing, error-handling, database, observability...).
  - Go libraries: godig (`cc-skills-golang:golang-pkg-go-dev`).
  - Non-Go tools (Prometheus, Grafana, Loki, Alloy, cAdvisor, Compose, k6,
    D2): Context7.
  - Charts or dashboards: `dataviz`.
- Check the current phase plan and progress in auto memory.

## 2. Propose

Show the user the proposal before writing: commit subject, files to create or
modify with their content or a diff summary, design decisions and trade-offs,
and how it will be verified. Wait for the user's answer.

## 3. Write and verify

- Run `make check` for anything touching `app/`.
- For containers: `make perms`, bring the stack up, check the behavior
  (targets up, logs in Loki, endpoints answering, failure cases), then bring
  everything down. Leave nothing running.
- Use `superpowers:verification-before-completion` before claiming it works.

## 4. Commit

- Conventional Commit, subject line only, no attribution.
- Report the hash and what was verified, then propose the next step.

## 5. Review fixes

If a review (e.g. the automatic security review) flags a problem in the commit
just made: explain it, propose the fix, and once approved apply it with
`git commit --amend`, without adding code comments about the fix. For older
commits, ask whether to amend or add a `fix(...)` commit.
