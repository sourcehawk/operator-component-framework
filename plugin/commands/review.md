---
description: Review this operator against the operator-component-framework guidelines
argument-hint: [path or scope, defaults to the whole repo]
---

Review this operator codebase against the operator-component-framework guidelines. Scope (may be empty, meaning the
whole repository): $ARGUMENTS

Dispatch the plugin's `reviewer` agent via the Agent tool with the scope above. When it returns, relay its findings to
the user unchanged, then offer to fix the violations it found, highest severity first.
