---
name: single-state-enum
description: Use when adding or reviewing mutually exclusive UI modes, dialogs, sheets, tabs, steps, and request states in web or Flutter.
---

# Explicit UI state

- Use one discriminated union or Dart sealed hierarchy for mutually exclusive modes, carrying each mode's payload together.
- Represent closed/idle explicitly. Avoid independent selected-item and open flags drifting apart.
- Model request loading/success/error states so impossible combinations cannot be constructed.
- Independent true on/off conditions may remain booleans; do not combine independent state machines just to reduce fields.
- Payment and fulfillment remain separate backend concepts. This skill governs UI consistency, not merging their business states.
