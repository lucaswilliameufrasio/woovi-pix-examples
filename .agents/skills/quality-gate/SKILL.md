---
name: quality-gate
description: Before declaring implementation complete, run fresh full tests, real local integration dependencies, lint, format, and build for every application touched.
---

# Quality gate

Discover existing scripts and run the whole affected application, not only edited files:

1. Format check, including `.svelte` files.
2. Full lint/static analysis.
3. Unit tests and real local integration tests. Skips and HTTP mocks alone do not prove integration.
4. Build supported targets; explicitly report unavailable platforms.
5. Run database suites 2–3 times and Go concurrency tests with `-race`.

Use mise and FVM. For the offers backend, `-p=1` is currently required because packages share a global event queue; keep within-test concurrency enabled. Isolated databases/schemas remain necessary before claiming parallel suite stability.

Report missing lint, E2E, adapters, platforms, or other gates as pending, not “all green.” Re-run fresh immediately before an authorized commit. Preserve unrelated user work; ask before consequential fixes outside scope.
