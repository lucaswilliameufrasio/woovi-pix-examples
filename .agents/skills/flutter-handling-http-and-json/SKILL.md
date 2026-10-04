---
name: flutter-handling-http-and-json
description: Use when writing Flutter HTTP clients, JSON parsing, API errors, retries, timeouts, or backend integration.
---

# Flutter HTTP and JSON

- Inject the HTTP client, construct URIs safely, and validate status and payload fields at runtime.
- Decode `/v1` snake_case models and house errors; branch on `error_code`, not human messages.
- Keep backend tokens/keys out of client source and artifacts.
- Use HTTPS outside explicitly permitted local debugging. Any cleartext exception stays debug-only and narrowly scoped.
- Bound requests and polling; do not retry order creation automatically after an uncertain result.
- Treat POST timeout as unknown outcome, not proof no order exists.
- Parse large payloads off the UI isolate only when measured cost warrants it.
- Test invalid JSON, values, unknown states, network failures, timeout, and disposal via FVM.
