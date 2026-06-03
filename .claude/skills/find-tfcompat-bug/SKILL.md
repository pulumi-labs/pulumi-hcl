---
name: find-tfcompat-bug
description: Find, prove, fix, and ship a single runtime divergence between OpenTofu and pulumi-hcl. Use when the user asks to "find another bug/mismatch", find a tfcompat mismatch, or hunt for behavior that differs from Terraform/OpenTofu.
---

# Find the next tfcompat bug

Hunt one genuine runtime divergence between OpenTofu (`tofu apply`) and pulumi-hcl,
prove it with a failing `tfcompat` test, fix the pulumi-hcl runtime to faithfully match
OpenTofu, verify failing-before / passing-after, and ship it as its own PR.

The divergence can be anywhere the two runtimes can disagree — an expression/function
result (the L1 seam), or resource, module, provider, or lifecycle behavior (the L2
seam). Functions are the easiest place to start, not the only place to look.

**Direction matters — only one direction is a bug.** A bug is an input that **works in
OpenTofu but does not work (or works differently) in pulumi-hcl**: OpenTofu produces a
value and pulumi-hcl errors or returns something else. That is the case a user migrating
real Terraform/OpenTofu configuration into pulumi-hcl actually hits. The reverse — an
input pulumi-hcl accepts that OpenTofu rejects (pulumi-hcl is more lenient) — is **not** a
bug to chase here: no OpenTofu config depends on it, and tightening pulumi-hcl to also
reject it removes capability without helping any migration. If the only divergence you can
find is pulumi-hcl being more permissive than OpenTofu, discard it and keep hunting.

Each invocation produces **exactly one** bug fix on its **own branch off master**
with its own changelog entry and PR. Do not stack fixes.

## Ground rules (from CLAUDE.md — do not violate)

- **Confirm assumptions with research. Don't guess.** Verify every behavioral claim
  against OpenTofu source, the cty stdlib source, the docs, or an empirical `tofu`
  probe — never from memory. A backwards memory of token semantics has already burned
  us once.
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
- OpenTofu docs: .../opentofu@*/website/docs/language/functions/*.mdx
- cty stdlib: !`go env GOPATH`/pkg/mod/github.com/zclconf/go-cty@*/cty/function/stdlib/
- pulumi-hcl runtime: `pkg/hcl/` (expression eval, resource / module / provider handling)
- pulumi-hcl function registry: `pkg/hcl/eval/functions.go`
  (`func Functions(baseDir string)` at the top binds every name to its impl)
- Function unit tests: `pkg/hcl/eval/functions_test.go`
- tfcompat harness + `Case` type: `tests/testutil/tfcompat/harness.go`
- Empirical probe: use the `tofu` on your PATH (!`which tofu`)

## Step 1 — Find a candidate divergence

Look for any input the two runtimes can evaluate differently. Two seams, easiest first:

- **L1 — expression / function:** a function in `pkg/hcl/eval/functions.go` with a
  **hand-rolled** `Impl` rather than a delegated `stdlib.*Func`. Hand-rolled impls are
  where function divergences hide. Diff against the OpenTofu impl/docs, and where cheap,
  probe both sides:

  ```bash
  # OpenTofu side — write a tiny main.tf and apply
  echo 'output "x" { value = <expr> }' > /tmp/probe/main.tf
  (cd /tmp/probe && tofu apply -auto-approve)
  ```

- **L2 — resource / module / lifecycle:** behavior around resources, modules,
  providers, `count`/`for_each`, pre/postconditions, or the preview / destroy / replace
  lifecycle. These exercise a provider (e.g. `providers.SimpleProvider`) — compare a
  small program end-to-end through apply, and where relevant preview and destroy.

Good hunting grounds: encoding/escaping funcs, numeric edge cases, empty-collection
errors, date/time token tables, string normalization, unknowns in preview,
precondition/postcondition error messages, replace/destroy lifecycle, module/provider
passthrough. Look for: wrong error on edge input, precision loss, off-by-one token
mapping, charset handling, escaping rules, a field that diverges only in state, an
operation that succeeds on one path and errors on the other.

Keep the direction in mind (see the intro): the case you want is one where **OpenTofu
succeeds and pulumi-hcl errors or returns a different value**. When an `operation that
succeeds on one path and errors on the other` turns up, check which side errors — if
pulumi-hcl is the one that succeeds and OpenTofu errors, that is pulumi-hcl being more
lenient, which is not the bug you are looking for. Discard it and keep hunting.

**Observability caveat:** pulumi serializes stack-output numbers as `float64`. Integer
precision past ~16 significant digits is lost on BOTH paths, so it cannot be exposed via
output comparison. Don't build a case whose only difference is unobservable in outputs —
reach for `AssertState`/`ExpectErr` (below), or prove it with a unit test, or pick a
different bug.

## Step 2 — Prove it with a failing test

Name the case `l1_<name>` for an expression/function bug or `l2_<name>` for a
resource/module/lifecycle bug, and create:

- `tests/tfcompat/<l1|l2>_<name>_test.go`
- `tests/tfcompat/testdata/cases/<l1|l2>_<name>/main.tf`

Existing cases (pick a fresh name):
!`ls tests/tfcompat/testdata/cases`

The test (copy the Apache header from any sibling `_test.go`):

```go
// L1 — pure expression, no providers
func TestL1<Name>(t *testing.T) {
	t.Parallel()
	tfcompat.RunCase(t, "l1_<name>", tfcompat.Case{})
}

// L2 — register the provider(s) the program uses
func TestL2<Name>(t *testing.T) {
	t.Parallel()
	tfcompat.RunCase(t, "l2_<name>", tfcompat.Case{
		Providers: []tfcompat.Provider{
			{Name: "simple", Factory: providers.SimpleProvider},
		},
	})
}
```

`RunCase` loads every file under `testdata/cases/<name>/` and runs it through both
`tofu apply` and `pulumi up`, asserting identical stack outputs. For an L1 case the
fixture is pure `locals` + `output` (one output per behavior). **Put the fixture on disk
under `testdata/cases/<name>/` and pass an empty `tfcompat.Case{}` — this is the default,
including multi-file cases** (e.g. a `templatefile` case ships its `main.tf` *and* the
template it reads; `RunCase` loads every file in the directory). The `Case` struct gives
you more knobs when outputs alone can't show the divergence:

- `Providers` / `Config` — register TF providers / set input variables.
- `AssertState` — assert on resource fields not reachable via outputs (e.g. `Protect`).
- `Stages` + `Mode` (`StageApply` / `StagePreview` / `StageDestroy`) — drive preview or
  destroy, or sequence multiple operations.
- `Stage.ExpectErr` — require BOTH runtimes to fail with a matching error substring; the
  way to prove an error-behavior divergence.

Reach for inline `Stages` with a `Files` map **only** when you need something the plain
disk fixture can't express — an `ExpectErr` assertion, or a multi-stage
preview/destroy/re-apply sequence. A normal output-comparison case (even one with several
files) belongs on disk, not inline.

Confirm the bug is real **on master before the fix**:

```bash
PATH="$PWD/bin:$PATH" go test ./tests/tfcompat/ -run 'Test(L1|L2)<Name>' -count=1 -v
```

It must FAIL, and the failure must show the genuine OpenTofu-vs-pulumi difference. If it
passes, the bug isn't real (or isn't observable) — go back to Step 2.

## Step 3 — Fix the divergence

Edit the pulumi-hcl runtime to match OpenTofu. For a function, fix the `Impl` in
`pkg/hcl/eval/functions.go` — and prefer delegating to the cty `stdlib.*Func` when
OpenTofu itself binds that stdlib function (check the binding map); several bugs were
hand-rolled reimplementations that should have been `stdlib.IndentFunc` /
`stdlib.FormatDateFunc`. For an L2 bug the fix lives elsewhere under `pkg/hcl/`. Fix
minimally and faithfully; run `go mod tidy` if you add a dependency.

Add unit coverage next to the code you changed. For functions, add cases to
`pkg/hcl/eval/functions_test.go`; note `evalExpr` calls `t.Fatalf` on diagnostics, so
**error-path** cases must call `<fn>.Call(...)` directly rather than via `evalExpr`.

## Step 4 — Verify failing-before / passing-after

```bash
make build
PATH="$PWD/bin:$PATH" go test ./pkg/hcl/... -run '<UnitTest>' -count=1 -v
PATH="$PWD/bin:$PATH" go test ./tests/tfcompat/ -run 'Test(L1|L2)<Name>' -count=1 -v
```

Both must now PASS (with `-count=1` — the rebuilt binary won't be picked up otherwise).

## Step 5 — Sweep for related divergences before shipping

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

## Step 6 — Changelog + branch + PR

Consult the submit-pr skill for opening a PR.

When writing your PR & changelog:

- Say **OpenTofu**, not "Terraform" — OpenTofu is the runtime we compare against —
- keep the `<thing>` (function or feature name) in backticks.
- Apply the same rules to the PR title.
- Keep the changelog `body` **terse** — a short phrase naming the change (e.g.
  ``Add `base64gunzip`, `urldecode` and `cidrcontains` ``), not a sentence
  explaining it. The divergence/fix narrative goes in the PR description.

Then, on its **own branch off master** (not stacked on a prior fix):

Report the divergence, the fix, the verification results, and the PR URL.
