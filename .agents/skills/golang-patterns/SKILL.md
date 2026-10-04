---
name: golang-patterns
description: Use when implementing Go handlers, services, repositories, interfaces, errors, and package structure.
---

# Go patterns

- Keep packages focused under `cmd/` and `internal/`; avoid unnecessary abstraction.
- Define small interfaces at the consumer boundary. Inject dependencies instead of mutable global state.
- Pass `context.Context` through blocking operations and propagate cancellation.
- Handle errors explicitly, wrap with `%w`, and use `errors.Is`/`errors.As`; no panic for expected failures.
- Keep HTTP translation at the API boundary and persistence invariants in transactional operations.
- Use table-driven tests and real PostgreSQL integration coverage for repositories and handlers.
