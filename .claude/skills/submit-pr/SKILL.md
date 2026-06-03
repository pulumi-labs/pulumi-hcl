---
name: submit-pr
description: Open a PR for the current change — branch, optional changelog fragment, and gh pr create. Use when asked to submit, open, or ship a PR.
---

# Submit a PR

Ship the working-tree change as a PR off `master`.

## 1. Branch & commit

- Never commit to `master` — branch first: `git checkout -b !`git config github.user`/<short-name>`.
- Stage specific paths only (`git add <path> …`), never `git add .`.
- Commit body in complete sentences; do not list yourself as co-author.

## 2. Changelog — only when the change is user-visible

Add a fragment **iff** a user would observe the change: behavior, stack output,
API, CLI, or error text. **Skip it** for refactors, tests, CI, comments, or any
internal-only change.

Write `.changes/unreleased/<component>-<kind>-<number>.yaml`:

```yaml
kind: bug-fixes        # or: Improvements
body: Add `base64gunzip`, `urldecode` and `cidrcontains`
time: $(date -u +"%Y-%m-%dT%H:%M:%SZ") #RFC3339 with +02:00 tz
custom:
    Component: runtime   # language-host | converter | codegen | runtime
    PR: "<number>"
```

- `body` is **terse** — a short phrase naming the user-facing change (e.g.
  ``Add `base64gunzip`, `urldecode` and `cidrcontains` ``), **not** an
  explanatory sentence. The why and how belong in the PR description, not here.
- Quote every function or feature name in `backticks`.
- It **does not end in punctuation**.
- One fragment per user-visible change.

## 3. Pick the number

- If the change closes an issue, use that issue number.
- Otherwise it is the next PR number: guess it as one past the highest of
  `gh pr list --state all --limit 1` and `gh issue list --state all --limit 1`,
  then **check** it after `gh pr create` — if the real number differs, rename the
  fragment file and update its `PR:` field to match with a force push.

## 4. Create

`gh pr create` — title and body describe the change; link any issue with its full
`https://github.com/…` URL.
