---
name: find-tfcompat-bug
description: Find and prove a single runtime divergence between OpenTofu and pulumi-hcl with a failing tfcompat test. Use when the user asks to "find another bug/mismatch", find a tfcompat mismatch, or hunt for behavior that differs from Terraform/OpenTofu.
---

# Find & prove the next tfcompat bug

Hunt one genuine runtime divergence between OpenTofu (`tofu apply`) and pulumi-hcl, and
**prove it with a failing `tfcompat` test**. That is the whole job of this skill: find a
real divergence and leave behind a test that fails on `master` for the right reason. You
**stop at the proven failure** — you do not fix it. To turn a proven failure into a
shipped fix, hand off to the [`fix-tfcompat-bug`](../fix-tfcompat-bug/SKILL.md) skill.

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

Each invocation produces **exactly one** proven failing test. Don't bundle several.

## Ground rules (from CLAUDE.md — do not violate)

- **Confirm assumptions with research. Don't guess.** Verify every behavioral claim
  against OpenTofu source, the cty stdlib source, the docs, or an empirical `tofu`
  probe — never from memory. A backwards memory of token semantics has already burned
  us once.
- Faithfully match OpenTofu. The goal is parity, not "better". No hacks.
- The deliverable is new test files only — you ADD `tests/tfcompat/` files. Don't edit
  runtime source under `pkg/` (that's the fixing skill's job), and don't touch staging the
  user left.

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

Confirm the bug is real **on master**:

```bash
PATH="$PWD/bin:$PATH" go test ./tests/tfcompat/ -run 'Test(L1|L2)<Name>' -count=1 -v
```

It must FAIL, and the failure must show the genuine OpenTofu-vs-pulumi difference (a real
output diff or a matched `ExpectErr`), not a harness/provider/fixture error. If it passes,
the bug isn't real (or isn't observable) — go back to Step 1.

## Done — hand off

You now have a proven failing tfcompat test, left **uncommitted** in the working tree.
Report:

1. **The divergence** — one line: what OpenTofu does vs. what pulumi-hcl does.
2. **The test** — name + the file paths you created.
3. **The failure output** — the captured diff/error proving it fails for the right reason.

To fix the divergence and ship it, continue with the
[`fix-tfcompat-bug`](../fix-tfcompat-bug/SKILL.md) skill. **Do not fix it here.**
