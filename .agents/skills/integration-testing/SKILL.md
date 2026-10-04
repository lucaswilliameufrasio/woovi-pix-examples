---
name: integration-testing
description: Use when writing or reviewing tests, handlers, repositories, HTTP adapters, workers, and test configuration. Require real local infrastructure, protocol fixtures, regression coverage, and isolation.
---

# Integration testing

- Unit tests cover isolated invariants and error branches with deterministic clocks.
- Integration tests exercise real routers, authorization, migrations, SQL, transactions, serialization, and concurrency against PostgreSQL.
- Run locally available infrastructure for real. Fake unavailable external providers at the HTTP protocol boundary using the actual adapter; never call remote environments.
- Every changed handler and repository needs real integration coverage. Assert persisted state, rollback, deduplication, ownership, and rejected authorization.
- Every bug fix needs a regression test that fails for the original reason before the fix.
- Generate unique fixture IDs, register cleanup, and isolate database/schema per suite. Unique IDs alone do not isolate workers consuming global queues.
- Run full suites repeatedly. Missing test database configuration is a blocked gate, not a pass.
- Widget tests and mocked BFF tests complement, not replace, mobile/browser E2E against backend and database.
- Do not exclude business logic, auth, handlers, repositories, or adapters from coverage to inflate numbers.
