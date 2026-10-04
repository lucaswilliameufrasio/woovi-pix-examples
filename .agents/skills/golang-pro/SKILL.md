---
name: golang-pro
description: Use for Go concurrency, workers, cancellation, lifecycle, profiling, and race-sensitive backend changes.
---

# Go concurrency and validation

- Give every goroutine a bounded lifecycle, cancellation, and ownership of cleanup.
- Do not block error reporting forever or leak workers after cancellation.
- Bound concurrency, close channels from their owner, and avoid shared mutable state without synchronization.
- Use transactions and database constraints for cross-process correctness, not only process-local mutexes.
- Test competing callers, idempotency, retries, leases, expiration, and late payments.
- Run mise-managed Go tests with `-race`, build, vet, gofmt, and full golangci-lint. Profile before speculative optimization.
