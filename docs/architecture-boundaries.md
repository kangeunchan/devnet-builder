# Architecture Boundaries

This repository enforces architecture boundaries with `depguard` in `golangci-lint`.

## Enforced Rules

1. `internal/application/**` must not import:
   - `internal/infrastructure/**`
   - `internal/daemon/**`
   - `internal/plugin/**`
2. `internal/application/ports/**` must not import:
   - `internal/infrastructure/**`
   - `internal/daemon/**`
   - `internal/plugin/**`

## Why

These rules keep dependency direction aligned with clean architecture:

- Application code depends on abstractions, not infrastructure details.
- Ports remain stable contracts without daemon/plugin coupling.
- Reviewers get automated boundary checks in CI.

## Existing Debt and Rollout

A small set of known legacy files are temporarily excluded from depguard in `.golangci.yml`.
New violations outside that allowlist fail CI.

## Remediation Pattern

When depguard fails:

1. Move concrete dependency usage into infrastructure/daemon adapters.
2. Define or extend a port interface in `internal/application/ports`.
3. Inject the dependency through constructors (DI) instead of direct imports.
