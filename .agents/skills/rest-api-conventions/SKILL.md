---
name: rest-api-conventions
description: Use when designing or reviewing HTTP endpoints, DTOs, API clients, authentication, errors, and caching.
---

# REST API conventions

- API prefix `/v1`; JSON, query, and path fields use `snake_case` recursively.
- Errors use `{message, error_code, extra?}`: PT-BR human text, stable SCREAMING_SNAKE_CASE machine code, and optional nonempty metadata. Never leak stack traces or secrets.
- Use 400 for malformed requests, 422 for semantic validation with `extra.validation_errors`, 401/403 for auth, named 404 codes, 409 for conflict, 412 for preconditions, and 502 for dependency failures.
- Centralize error translation. Keep provider codes in safe metadata rather than replacing public error codes.
- Return resources directly; cursor-paginate unbounded lists. Small bounded demo catalogs can remain arrays.
- Use UTC ISO timestamps and real database timestamps. Prefer UUID IDs; document deliberate catalog slug exceptions.
- Omit absent optional API values. Validate ownership; knowing an ID is not production authorization.
- Treat payment retries and uncertain outcomes explicitly. Prices come from the backend.
- Keep caching outside business logic and invalidate centrally on relevant writes.
