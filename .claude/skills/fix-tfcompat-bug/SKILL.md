---
name: fix-tfcompat-bug
description: Fix a proven OpenTofu-vs-pulumi-hcl divergence and ship it as its own PR. Use after find-tfcompat-bug has produced a failing tfcompat test — fix the runtime to match OpenTofu, sweep for the sibling bug, add unit coverage, and open a PR. Use when asked to fix/ship a known tfcompat mismatch.
---

# Fix & ship a proven tfcompat bug

This skill starts from a **proven failing tfcompat test** — a divergence already found and
captured by the [`find-tfcompat-bug`](../find-tfcompat-bug/SKILL.md) skill, with a
`tests/tfcompat/<l1|l2>_<name>_test.go` + `testdata/cases/<name>/` fixture that **fails on
`master` for the right reason** (a genuine OpenTofu-vs-pulumi diff or matched `ExpectErr`,
not a harness error). Edit the pulumi-hcl runtime to faithfully match OpenTofu, verify
failing-before / passing-after, sweep for the sibling bug, and ship it as its own PR.

If you do **not** yet have such a failing test, stop and run `find-tfcompat-bug` first —
do not invent a fix without a proven failure.

This skill ships **exactly one** bug fix on its **own branch off master** with its own
changelog entry and PR. Do not stack fixes.

## Ground rules (from CLAUDE.md — do not violate)

- **Confirm assumptions with research. Don't guess.** Verify every behavioral claim
  against OpenTofu source, the cty stdlib source, the docs, or an empirical `tofu` probe.
- Faithfully match OpenTofu. The goal is parity, not "better". No hacks.
- `git add` specific files only, never `git add .`. Don't touch staging the user left.
- Don't self-attribute as commit co-author. Complete punctuation in commit bodies.
- Commit / push / open PR only when the user asks (the find+fix+verify is automatic;
  shipping waits for the modeled flow below, which the user has standing-approved for
  this loop — confirm if unsure).

## Source-of-truth locations

The Go module cache root is !`go env GOPATH`/pkg/mod.

- OpenTofu funcs: !`go env GOPATH`/pkg/mod/github.com/pulumi/opentofu@*/lang/funcs/*.go
- OpenTofu binding map: .../opentofu@*/lang/functions.go
- cty stdlib: !`go env GOPATH`/pkg/mod/github.com/zclconf/go-cty@*/cty/function/stdlib/
- pulumi-hcl runtime: `pkg/hcl/` (expression eval, resource / module / provider handling)
- pulumi-hcl function registry: `pkg/hcl/eval/functions.go`
- Function unit tests: `pkg/hcl/eval/functions_test.go`

## Step 1 — Fix the divergence

Edit the pulumi-hcl runtime to match OpenTofu. For a function, fix the `Impl` in
`pkg/hcl/eval/functions.go` — and prefer delegating to the cty `stdlib.*Func` when
OpenTofu itself binds that stdlib function (check the binding map); several bugs were
hand-rolled reimplementations that should have been `stdlib.IndentFunc` /
`stdlib.FormatDateFunc`. For an L2 bug the fix lives elsewhere under `pkg/hcl/`. Fix
minimally and faithfully; run `go mod tidy` if you add a dependency.

Add unit coverage next to the code you changed. For functions, add cases to
`pkg/hcl/eval/functions_test.go`; note `evalExpr` calls `t.Fatalf` on diagnostics, so
**error-path** cases must call `<fn>.Call(...)` directly rather than via `evalExpr`.

## Step 2 — Verify failing-before / passing-after

```bash
make build
PATH="$PWD/bin:$PATH" go test ./pkg/hcl/... -run '<UnitTest>' -count=1 -v
PATH="$PWD/bin:$PATH" go test ./tests/tfcompat/ -run 'Test(L1|L2)<Name>' -count=1 -v
```

Both must now PASS (with `-count=1` — the rebuilt binary won't be picked up otherwise).
The tfcompat case that failed on `master` must now pass against your fix.

## Step 3 — Sweep for related divergences before shipping

**Before you open the PR, check whether the same root cause produces a sibling
bug, and fix it in the same PR.** A divergence almost never lives alone: the same
helper, library choice, or code path usually backs a *sibling* operation, and
shipping only one half leaves the matching bug behind for the next migration to
hit.

Look in particular for:

- **The inverse operation.** encode ↔ decode, parse ↔ format, marshal ↔
  unmarshal, get ↔ set. Fixing `yamlencode` (hand-rolled on `yaml.v3` instead of
  go-cty-yaml) left an *identical* `yamldecode` bug — same wrong library, mirror
  symptom — that had to ship as a separate follow-up. Don't make that mistake:
  when you fix one direction, probe the other in the same session.
- **Sibling functions sharing the impl.** Functions bound to the same hand-rolled
  helper or the same family (`base64*`, `file*`, `cidr*`, the `to*` converters).
  Grep for other call sites of any helper you touched.

For each candidate, run a quick `tofu` probe against the equivalent pulumi-hcl
result. If it also diverges in the migration-affecting direction, fold the fix
into this PR and extend the one tfcompat case to cover both (one combined
`l1_<family>` case is fine — e.g. `l1_yaml` exercising both `yamlencode` and
`yamldecode`). If a sibling is genuinely out of scope, say so explicitly in the
PR body rather than leaving it silently unfixed.

## Step 4 — Changelog + branch + PR

Consult the submit-pr skill for opening a PR.

When writing your PR & changelog:

- Keep the changelog `body` **terse** — a short phrase naming the change from the
  user's point of view (e.g. ``Coerce a `variable` `default` to its declared `type` ``
  or ``Add `base64gunzip`, `urldecode` and `cidrcontains` ``), not a sentence explaining
  it, and with **no trailing punctuation**. Keep the `<thing>` (function/feature name)
  in backticks.
- The changelog body must **not mention OpenTofu** (no "match OpenTofu", "like
  OpenTofu", "instead of erroring", or other comparison framing). Parity with OpenTofu
  is the whole project's premise, so stating it in the changelog is noise — just
  describe what changed. The OpenTofu-vs-pulumi divergence narrative belongs in the **PR
  description**, where you do say "OpenTofu" (never "Terraform").
- **Code comments must not mention OpenTofu either.** Do not add comments saying the
  code "matches OpenTofu" / "mirrors OpenTofu's X" — that applies to the entire codebase
  and restates the premise. A comment should explain something the code itself doesn't
  (e.g. *why* a helper is reimplemented rather than imported); if the only thing a
  comment would say is "this matches OpenTofu," delete it.

Then, on its **own branch off master** (not stacked on a prior fix):

Report the divergence, the fix, the verification results, and the PR URL.
