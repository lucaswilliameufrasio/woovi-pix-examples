---
name: flutter-testing-apps
description: Use when implementing or reviewing Flutter unit, widget, or device integration tests.
---

# Flutter tests

- Unit tests cover models, clients, state transitions, and lifecycle logic.
- Widget tests cover loading, empty, success, error, retry, accessibility, callbacks, and disposal.
- Use deterministic clocks/completers rather than wall-clock sleeps. Exercise overlapping requests and closing views while responses are pending.
- Device integration tests cover critical flows using real local backend/database and simulator; no remote provider calls.
- Verify public SDK behavior, not only that symbols are exported.
- Run FVM format, analyze, tests, and supported builds; state clearly when iOS/device integration was not validated.
