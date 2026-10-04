---
name: typescript-code-standards
description: Follow these standards whenever writing or reviewing TypeScript, JavaScript, Svelte, or their tests. Prohibit intentional null, casts, non-null assertions, forEach, unbraced if statements, and unused-variable prefix hacks; require route resolution and organized tests.
---

# TypeScript code standards

- Do not write, assign, or return `null` for intentional absence. Use `undefined` and optional fields. Normalize external empty values at the boundary; database NULL is not application state.
- Use `for...of` or ordinary loops, never `.forEach`.
- Every JavaScript/TypeScript `if` uses braces and a multiline block.
- Do not use type assertions (`as Type`, `as const`, double casts, angle-bracket casts) or non-null assertions. Use explicit annotations, contextual typing, `satisfies`, runtime type guards, or schema validation.
- Remove unused variables and parameters; do not hide them behind `_` prefixes.
- Use `resolve()` from `$app/paths` for internal SvelteKit `href` and `goto()` destinations. Asset URLs are not route destinations.
- Key every Svelte `{#each}` block by a stable, unique identifier.
- Name test cases `it('Should ...')`, including parameterized cases.
- Put web tests under `tests/unit/` or `tests/browser/`, never `src/`.
- Validate untrusted HTTP payloads rather than asserting their types. Keep API wire fields `snake_case`.
- Enforce these conventions with lint, not just review. An existing violation is a defect, not an exception.
