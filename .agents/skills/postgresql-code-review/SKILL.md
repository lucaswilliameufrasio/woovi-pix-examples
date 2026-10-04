---
name: postgresql-code-review
description: Use when reviewing PostgreSQL schemas, queries, migrations, transactions, indexes, and worker persistence.
---

# PostgreSQL review

- Enforce invariants with constraints and transactions; parameterize queries and use appropriate integer money/timestamp types.
- Review locking order, atomic inventory/pickup operations, deduplication keys, worker leases, rollback, and retry behavior.
- Match indexes to actual predicates and ordering. Verify plans before optimization; do not add generic GIN indexes that do not support the query.
- CHECK constraints need explicit presence requirements when missing values must be rejected.
- Use isolated integration databases and safe fixture cleanup; never reset unrelated data.
- Test migrations, contention, expiration races, late events, and restart recovery with real PostgreSQL.
