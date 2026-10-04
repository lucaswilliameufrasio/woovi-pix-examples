---
name: svelte-code-writer
description: Use whenever creating, editing, or analyzing Svelte components or modules. Consult current documentation and analyze components before completion.
---

# Svelte documentation and analysis

- Consult current official Svelte/SvelteKit documentation for relevant routing, runes, SSR, actions, and generated types.
- The public `@sveltejs/mcp` CLI provides `list-sections`, `get-documentation`, and `svelte-autofixer`.
- Run component analysis before completion when tooling is available; report unavailable checks rather than pretending they ran.
- Use mise-managed Node. Pin tools before adding dependencies and preserve lockfiles; do not silently run unpinned install commands.
- Also run full Svelte check, Prettier with the Svelte plugin, lint, tests, and build.
- Avoid shell interpolation of rune names when passing code to tools.
