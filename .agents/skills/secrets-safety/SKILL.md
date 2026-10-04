---
name: secrets-safety
description: Use for credentials, environment configuration, payment keys, bearer tokens, database connections, CI secrets, and suspected leaks. Prevent disclosure in public files, logs, output, and commits.
---

# Secrets safety

- Do not dump environment variables, secret stores, or credential files. Inspect names and redacted metadata.
- Do not echo, log, reproduce, or commit resolved credentials.
- Provider keys belong only on the backend, never in web/mobile artifacts.
- Use fictional placeholders; public local database credentials require explicit authorization and must be unmistakably test-only.
- Prefer secret references or approved write-only configuration. Verify with health checks, not resolved values.
- On exposure, report location/scope without quoting the value and recommend rotation. Do not rotate, rewrite history, or change permissions without authorization.
- Audit copied skills/resources for private paths, customer data, and credentials. Preserve public licenses and attribution.
