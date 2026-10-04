---
name: git-workflow
description: Use before branching, staging, committing, pushing, or preparing a PR. Preserve dirty worktrees and require explicit authorization.
---

# Git workflow

- Inspect status, branch, diff, and remote before acting. Do not assume a branch model or remote name.
- Preserve existing work; never reset, restore, overwrite, or discard unrelated changes.
- Use worktrees when another branch is necessary; do not switch a shared dirty checkout.
- Commit, push, amend, rebase, force-push, PR, merge, and deployment require explicit authorization for the operation.
- Stage explicit paths only and inspect staged diffs. Do not use broad staging to absorb unrelated work.
- Use English Conventional Commits and real issue references when available.
- Run the full quality gate freshly before an authorized commit. Report the hash and any excluded changes.
