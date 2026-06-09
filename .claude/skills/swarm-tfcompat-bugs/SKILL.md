---
name: swarm-tfcompat-bugs
description: Run a swarm of parallel agents that each hunt for one OpenTofu-vs-pulumi-hcl runtime divergence and prove it with a failing tfcompat test. Use when the user asks to "spin up N agents", "run a swarm", or hunt for a tfcompat mismatch in parallel.
---

# Swarm: hunt a tfcompat divergence in parallel

Dispatch N agents that each independently hunt a **different** OpenTofu-vs-pulumi-hcl
runtime divergence and **prove it with a single failing `tfcompat` test**. The goal of a
swarm run is to surface **one genuine failure** and report it back to the user — not to
fix anything, not to open PRs. You (the lead) own **de-confliction** (every agent works a
disjoint area so no two chase the same divergence) and **adjudication** (deciding which
returned candidate is a real, in-direction failure worth reporting).

Each agent runs the [`find-tfcompat-bug`](../find-tfcompat-bug/SKILL.md) skill — which is
itself entirely find-and-prove and stops at a failing test. Agents **must not** continue
into the [`fix-tfcompat-bug`](../fix-tfcompat-bug/SKILL.md) skill (fix, sweep, changelog,
branch, PR). Read `find-tfcompat-bug` first for the find-and-prove technique; this one adds
the swarm orchestration and de-confliction around it.

## The deliverable

A swarm run is done when **one** agent has produced a **genuine, in-direction failing
tfcompat test** and you have confirmed it. Report back to the user:

1. **The divergence** — one line: what OpenTofu does vs. what pulumi-hcl does.
2. **The failing test** — the `tests/tfcompat/<l1|l2>_<name>_test.go` +
   `testdata/cases/<name>/` files the agent created, left in the working tree (uncommitted).
3. **The failure output** — the actual diff/error from running the test on `master`,
   proving it fails for the right reason (real OpenTofu-vs-pulumi difference, not a harness
   error).

Do **not** fix the divergence, write a changelog, branch, or open a PR. Stop at a proven,
reported failure. The user drives what happens next.

## The lead's job

1. **Partition the search space into disjoint domains — one per agent.** This is what
   prevents collisions. Never spawn N agents with the same instructions. Good seams to
   split along (mix and match to reach N):
   - L1 function families: collection funcs; string funcs; encoding funcs; crypto/hash
     funcs; cidr/IP funcs; date/time + numeric funcs; the `to*` type converters +
     `can`/`try`/`type`; the sensitivity-marks seam (`sensitive`/`nonsensitive`/
     `issensitive`); file-content/file-hash/`path.*` resolution.
   - L2 seams: resource/module/provider plumbing; `count`/`for_each`/`dynamic`
     meta-arguments; lifecycle (`prevent_destroy`/`ignore_changes`/`replace_triggered_by`/
     `create_before_destroy`); pre/postcondition + variable `validation`; preview/destroy
     lifecycle; sensitivity propagation through state.
   - The HCL expression-eval seam: for-expressions, splat, conditionals, string templates/
     heredocs/trim markers, index/attr access, `==`/`!=` coercion. (Note: this seam
     delegates to upstream `hclsyntax`/`cty` and has historically been clean — assign it
     only when you have spare agents.)

2. **Build one shared exclusion list and put it in every agent's prompt.** It must name
   (a) every bug already fixed/shipped, and (b) seams already proven clean. Without this,
   agents re-discover already-fixed bugs and report a "failure" that is actually already
   handled. Re-derive it each run from `git log`, the open PRs (`gh pr list`), and the
   existing `tests/tfcompat/testdata/cases/` directory.

3. **Tell each agent the other agents' domains** ("stay out of these") as a second
   guard against overlap.

4. **Spawn all N in a single message**, each with `subagent_type: general-purpose` and
   `run_in_background: true` (so they run truly concurrently and notify you on
   completion). Give each a unique `name` (e.g. `hunter-<domain>`). Worktree isolation is
   **not** needed — agents only *add* new test files under `tests/tfcompat/` and don't
   edit shared source, so they can share the working tree. (If you do hand out worktrees,
   remember the winning test files live in that worktree, not the main tree — collect them.)

5. **Collect candidates as they land and adjudicate.** For each returned candidate,
   confirm it is a **real, in-direction** failure before declaring victory (see "Vetting"
   below). The first candidate that passes vetting is the deliverable — report it and wind
   the swarm down.

6. **A negative is a success, not a failure.** An agent that returns an honest "no
   in-direction divergence in my domain" has done its job — do not pressure it to invent
   one. A fabricated or out-of-direction "failure" is worse than a clean negative.

7. **Keep hunting only until you have ONE confirmed failure.** This is a single-failure
   hunt, not a steady-state swarm. You do **not** need to maintain headcount at N, spawn
   replacements indefinitely, or monitor any PRs. Once one candidate is vetted and
   reported, stop spawning and let remaining agents finish or wind them down. If **all**
   agents return negatives, you may spawn a fresh wave over still-unclaimed domains (with
   an updated exclusion list) — but if the search space is genuinely picked over, say so
   rather than spawning into exhausted ground.

## What every agent prompt must contain

- "Read and follow `.claude/skills/find-tfcompat-bug/SKILL.md` — find a candidate
  divergence and prove it with a failing test. **Stop at the proven failure. Do NOT
  continue into `fix-tfcompat-bug`: no fix, no changelog, no branch, no PR.**" Plus the
  global `CLAUDE.md` rules
  (research don't guess; faithfully verify against OpenTofu source/docs/probe; no hacks).
- **Its assigned domain** (hunt only here) and **the other agents' domains** (stay out).
- **The shared exclusion list.**
- **The direction rule, restated:** a bug is OpenTofu-succeeds / pulumi-hcl-errors-or-
  differs. pulumi-being-*more-lenient* is NOT the bug — discard it and keep hunting.
- **Build + run:** `make build`, then run the candidate test with the freshly built
  binaries on PATH and `-count=1`:
  `PATH="$PWD/bin:$PATH" go test ./tests/tfcompat/ -run 'Test(L1|L2)<Name>' -count=1 -v`.
  The test **must FAIL on master**, and the failure must show the genuine
  OpenTofu-vs-pulumi difference (a real output diff or matching `ExpectErr`), not a
  harness/setup error.
- **The deliverable:** exactly ONE proven failing tfcompat case — the new
  `_test.go` + `testdata/cases/<name>/` files left **uncommitted** in the working tree,
  plus the captured failure output. Honest negative if the domain is genuinely clean.
- **The required final message:** (1) one-line divergence (OpenTofu vs pulumi-hcl),
  (2) the test name + paths of the files it created, (3) the captured failing-test output
  proving it fails for the right reason — or an honest negative naming what it probed.

## Vetting a returned candidate (the lead's adjudication)

Before reporting a candidate as *the* failure, confirm:

- [ ] **It reproduces.** Re-run the agent's test yourself on `master` with freshly built
      binaries and `-count=1`. It must fail.
- [ ] **It fails for the right reason.** The failure is a genuine OpenTofu-vs-pulumi
      output diff or a matched `ExpectErr` — not a missing provider, a fixture typo, a
      panic in the harness, or an unrelated build error.
- [ ] **The direction is right.** OpenTofu succeeds (or produces value X); pulumi-hcl
      errors or produces a different value. pulumi-hcl being *more lenient* than OpenTofu
      is **not** the bug — reject it and keep looking.
- [ ] **It isn't on the exclusion list.** Not an already-shipped fix or a known-clean seam
      re-surfaced.

A candidate that fails any check is sent back (or the agent stands down); keep adjudicating
incoming candidates until one passes all four.

## De-confliction checklist (the one thing that makes a swarm work)

- [ ] N agents → N disjoint domains, no two overlapping.
- [ ] Shared exclusion list names every shipped/claimed/clean area.
- [ ] Each agent told the others' domains.
- [ ] All spawned in one message: `general-purpose`, background, unique names.
- [ ] Each agent stops at a **proven failing test** — no fix, no changelog, no PR.
- [ ] Lead vets each candidate (reproduces / right reason / right direction / not excluded)
      before reporting it as the single deliverable.
