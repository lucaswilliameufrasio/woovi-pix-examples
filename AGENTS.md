# Project instructions

Read the relevant local `.agents/skills/<name>/SKILL.md` before working on that subsystem. These instructions apply to all three independent demos and their backend, web, and mobile applications.

## Required skills

- Any TypeScript, JavaScript, or Svelte change: `typescript-code-standards`.
- Svelte components: `svelte-code-writer`, `svelte-core-bestpractices`.
- API contracts: `rest-api-conventions`.
- Go: `golang-patterns`, `golang-pro`; SQL: `postgresql-code-review`.
- Flutter: `flutter-architecting-apps`, `flutter-handling-http-and-json`; tests: `flutter-testing-apps`.
- Mutually exclusive UI flows: `single-state-enum`.
- All tests: `integration-testing`; web tests: `vitest`.
- Non-trivial design choices: `document-decisions`.
- Credentials/configuration: `secrets-safety`; Git operations: `git-workflow`.
- Before reporting implementation complete: `quality-gate`.

## Local constraints

- Use FVM for Flutter and mise for Go/Node. Preserve pinned versions and native package-manager lockfiles; web currently uses npm.
- Keep each demo independent, including its database. API is `/v1`, JSON/query/path fields are `snake_case`, errors are `{message, error_code, extra?}`.
- Backend owns prices, payment state, reservation state, and pickup authorization. Local history is not authentication or authoritative state.
- Keep payment and fulfillment states separate. UI enums do not merge independent business state machines.
- No real payments, provider calls, sandbox registration, deployment, commit, push, or PR without separate explicit authorization.
- Never invent credentials or a payable Pix payload. Keep local demos loopback-only until proper authorization is implemented.
- Public database demo credentials are allowed only when explicitly didactic, local-only, and authorized; never reuse them elsewhere.
- Preserve existing uncommitted work. Do not modify the SDK dependency checkout.
- Record progress in the existing ai-memory plan when requested. Do not call the whole plan finished while demos, integration, or required validation remain pending.
