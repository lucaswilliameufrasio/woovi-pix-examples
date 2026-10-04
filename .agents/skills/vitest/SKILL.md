---
name: vitest
description: Use when writing or configuring web unit/integration tests, mocks, timers, isolation, and coverage with Vitest.
---

# Vitest conventions

- Web unit tests live in `tests/unit/` and use `it('Should ...')`; browser tests live in `tests/browser/`.
- Use typed `vi.fn`, runtime guards, and contextual typing instead of casts.
- Restore environment, timers, mocks, and resources after tests. Avoid shared fixture state.
- Test deadlines with fake timers and rejected requests; verify no unintended retry or mutation.
- Mock unavailable services at the protocol boundary. Real backend/database E2E is still required for critical flows.
- Keep pinned dependencies and run the full suite through mise-managed npm.
