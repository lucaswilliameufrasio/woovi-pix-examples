---
name: flutter-architecting-apps
description: Use when structuring or refactoring Flutter applications and state. Separate presentation and data access without unnecessary layers.
---

# Flutter architecture

- Keep widgets focused on rendering and user interactions. Move HTTP, persistence, polling, and lifecycle orchestration into testable collaborators.
- Use immutable models and unidirectional data flow; add a domain layer only for actual complexity.
- Model mutually exclusive flows with explicit sealed states rather than disconnected flags.
- Backend is authoritative for prices, payment, expiry, and fulfillment. Local order IDs/cache are not ownership or confirmed current state.
- Scope state and dependency lifetime to the app/feature. Dispose timers, streams, and clients owned by that scope.
- Use FVM; do not modify the sibling SDK checkout as part of consuming its public API.
