---
name: svelte-core-bestpractices
description: Use when creating, editing, or reviewing Svelte 5 components, reactivity, event handling, and SSR state.
---

# Svelte 5 practices

- Use runes for new components: `$props`, `$state`, `$derived`, and modern event handlers.
- Use `$derived` for computed data, not effects that synchronize duplicate state.
- Props can change during navigation; do not capture stale derived values.
- Keep request/user state scoped; no shared mutable module state leaking across SSR requests.
- Use keyed each blocks, semantic controls, explicit loading/error states, and accessible focus behavior.
- Follow local TypeScript standards, including `$app/paths` resolution.
- Verify current framework documentation before relying on experimental or version-sensitive behavior.
