---
feature: version-matrix-goldens
spec: docs/superpowers/specs/2026-06-01-version-matrix-goldens-design.md
plan: docs/superpowers/plans/2026-06-01-version-matrix-goldens-plan.md
tracking_issue: #131
feature_branch: feature/version-matrix-goldens
feature_worktree: .claude/worktrees/version-matrix-goldens
sub_pr_approval: autonomous
sub_pr_review_loop: off
integration_pr: #<pr>
status: planning
---

# Version-matrix golden generation — orchestration state

## Phases

- **Phase 1 (foundational)** — `#132` (introspection + serializer export; produces both contracts)
- **Phase 2 (core)** — `#133` (goldengen core; consumes Phase 1, produces the goldengen public API)
- **Phase 3 (consumers, parallel)** — `#134` (accounting + example + docs), `#135` (YAML matrix loader)

Phase 3's two issues are independent of each other and may run in parallel once `#133` has merged into the feature branch.

## PRs / worktrees

| Issue | Branch                         | Worktree path                                       | PR (→ base)                       | Status      |
| ----- | ------------------------------ | --------------------------------------------------- | --------------------------------- | ----------- |
| #132  | pr/132-mutation-introspection  | .claude/worktrees/version-matrix-goldens--132       | #136 → feature/version-matrix-goldens | ready |
| #133  | pr/133-goldengen-core          | .claude/worktrees/version-matrix-goldens--133       | #<pr> → feature/version-matrix-goldens | not-started |
| #134  | pr/134-accounting-docs         | .claude/worktrees/version-matrix-goldens--134       | #<pr> → feature/version-matrix-goldens | not-started |
| #135  | pr/135-yaml-loader             | .claude/worktrees/version-matrix-goldens--135       | #<pr> → feature/version-matrix-goldens | not-started |

## Contracts

| Name                   | Realization | Realized in            | Status  |
| ---------------------- | ----------- | ---------------------- | ------- |
| `MutationInspector`    | sequential (merges to feature branch before #133 branches) | #132 | pending |
| `golden.Serialize*`    | sequential (merges to feature branch before #133 branches) | #132 | pending |
| `goldengen` public API | sequential (merges to feature branch before #134/#135 branch) | #133 | pending |

All three are sequential merge dependencies; no pre-merge stub PRs are needed. #134 and #135 share only #133's public API and expose nothing to each other.

## Bubble-up log

- _No concerns yet._

## Resume checklist

For a fresh Claude session resuming this work:

1. Read this state file in full.
2. Read the plan at the path in the `plan:` frontmatter.
3. Read the spec at the path in the `spec:` frontmatter.
4. Verify each open PR's actual state via `gh pr view <num>`.
5. For each `in-progress` or `draft` row, `cd` to the worktree path and check `git status` + `git log --oneline main..HEAD`.
6. Re-dispatch subagents as needed per `feature-dev-workflow:developing-a-feature` (parallel waves still in flight; the orchestrator watch loop continues).
