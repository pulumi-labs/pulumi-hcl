---
name: farm-tfcompat-bugs
description: Run a sustained bug-farming round — keep a full swarm hunting until N distinct OpenTofu-vs-pulumi-hcl divergences are proven, then ship each as its own draft PR containing only the failing test. Use when the user asks to "find 10 more bugs", "raise the next N issues", or run another bug-hunting round.
---

# Farm: N proven divergences, N draft PRs

A **round** ends when N distinct, vetted divergences each have their own draft PR
containing **only the failing test**. This is the steady-state version of
[`swarm-tfcompat-bugs`](../swarm-tfcompat-bugs/SKILL.md), which stops at one bug and
reports. Everything there about partitioning, exclusion lists, and the direction rule
still applies — read it first. This skill adds what only shows up when you run the swarm
to a quota, round after round: **sustained headcount, adjudication under duplicate
pressure, PR staging, and saturation accounting.**

The three jobs that are actually yours (the lead's): **de-confliction**, **adjudication**,
and **shipping**. Hunters never open PRs. You vet every candidate personally.

## Round setup

1. `git fetch && git pull` master. Re-derive the exclusion list from *this* master:
   `git log` since the last round, `gh pr list --state all`, `gh issue list --state open`,
   and `ls tests/tfcompat/testdata/cases`. Last round's fixes are this round's exclusions.
2. **Read `docs/terraform-compatibility.md` and put its "Known Limitations" in every
   hunter prompt verbatim.** That file is the owner's ruling on which divergences are
   *accepted and documented* — unsupported `terraform`-block fields, the `List<Object>`
   empty-vs-null limit, destroy-ordering of late-created instances, unsupported/extra
   functions. A divergence listed there is settled, not a bug, no matter how cleanly it
   reproduces. It grows every round (an owner closing one of your PRs by documenting it
   lands here), so re-read it each round rather than trusting last round's copy.
3. **Fence anything the user is actively working.** If they have an open PR or a live
   branch in an area, that whole area is off-limits — not just the specific bug. State the
   fence as a positive constraint in every prompt ("your signal must be a value, type,
   mark, or op *kind* — never op *order* and never a dependency edge"), because a negative
   fence alone gets rounded down to "avoid that one test".
4. Carry the **exhausted-domain list** forward from previous rounds' memory notes and
   fold it into the exclusion list. Domains that returned clean negatives twice are not
   worth a third agent.
5. Partition into N disjoint domains, spawn all N in one message
   (`general-purpose`, `run_in_background: true`, unique `name`).

## What a hunter prompt needs beyond the swarm skill's list

- **`WRITE ALL FILES TO THIS WORKTREE: <absolute path>`** on its own line. Without it
  hunters write into the main checkout and you will hunt for their deliverables. This is
  the single highest-value line in the prompt.
- **Rebuild before judging:** `make build`, then
  `PATH="$PWD/bin:$PATH" go test ./tests/tfcompat/ -run 'Test(L1|L2)<Name>' -count=1 -v`.
  A stale `bin/` produces failures that no longer exist on master.
- **Capture, don't eyeball:** `go test … > /tmp/<name>.txt 2>&1`, then grep for
  `expected:|actual|Messages:|error:|does not contain`. The harness diff carries ANSI and
  wrapping that defeats a naive grep of scrollback.
- **Report negatives honestly, naming what was probed.** A clean negative is a result —
  it retires a domain. A fabricated bug costs more than an empty round.

## Sustaining headcount

Hold N hunters *working* until the quota is met — the drain is real: some domains come
back clean, some candidates get rejected, and later bugs cost more than early ones.

**Re-task with `SendMessage`, don't respawn.** A hunter that just proved its domain clean
already knows the harness, the providers, and the rejection bar. Send it the new domain
plus the *updated* exclusion list (including whatever just got filed). A fresh `Agent`
call pays for all that context again.

Expect to re-task roughly half the swarm once or twice to reach 10. If a hunter needs a
third domain, the search space is telling you something — see Saturation.

## Adjudication

**The bar: you personally reproduce it 3/3 deterministic FAIL, on freshly built binaries,
with `-count=1`.** Not the hunter's run — yours. A hunter FAIL that PASSes for you is a
stale-binary artifact; reject it and re-task.

Then walk the rejection classes. Every one of these has cost a real round:

| Reject when | Why |
|---|---|
| pulumi-hcl is *more lenient* than OpenTofu | Wrong direction; no migration depends on it |
| SDKv2 optional-attr null vs `""` | Zero-value artifact of the shim, not a divergence |
| Computed field recomputed on update | Self-healing, by design |
| Plan-time unknown-refinement diff | Unobservable to a user |
| Secret / sensitive **over**-propagation | Pulumi's stickier marks are by design |
| Only difference is number representation | Deferred `float64` family — flag it, don't file it |
| Signal is op *order* or a dependency edge | Fenced; belongs to the scheduling work |
| Structurally indistinguishable on the wire | E.g. MaxItemsOne empty-vs-absent block |
| Listed in `docs/terraform-compatibility.md` | Accepted, documented limitation — settled |

**Verify every claimed duplicate before rejecting on it.** `gh issue view` /
`gh pr view` the number. Issues get closed by a *different* PR than the one that names
them, and a "fixed in #X" that actually landed in #Y still fails on master — that is a
live bug, not a duplicate. Equally: a bug you remember being filed may be closed and
scoped to a path that doesn't cover your case.

**Borderline goes in the PR, with the relationship named.** If a candidate might be a
sibling of an open issue, or might be intended behavior in the Pulumi model, file it and
say so in the body. The owner folds or closes it in seconds; a silently dropped real bug
is gone. Do not let "possibly by design" become a rejection reason on your own authority.

**Take evidence-backed pushback from hunters.** A lead's exclusion can be wrong. When a
hunter comes back with an empirical probe contradicting a fence you set, verify it and
change the fence. One round's #430 exists only because a hunter refused a bad exclusion.

## Shipping: one draft PR per bug

Stage from a detached worktree so the session tree stays untouched:

```bash
git worktree add --detach <staging> origin/master
```

Then per bug, in `<staging>`: branch off `origin/master` → copy **only that bug's**
`tests/tfcompat/<case>_test.go` + `testdata/cases/<case>/` (plus its provider file if it
added one) → `git add` those exact paths → commit → push →
`gh pr create --draft`.

- **Test only. No fix, no changelog.** CI failing is the point; say so in the body.
- One bug per PR, off master, never stacked.
- Body: the divergence in one line (OpenTofu does X, pulumi-hcl does Y), the repro
  command, the captured failure output, and any flagged relationship to an open issue.
- Never `git add .`; never touch the user's index in the session tree.

## Closing the round

1. **Verify all N PRs exist** (`gh pr list`) and note any the user has already adopted or
   retitled — that is the signal the round landed.
2. **Clean both trees.** Hunters leak files into the main checkout as well as the
   worktree. `git status --short` both, `rm`/`rm -rf` the exact untracked scratch paths
   (test files, case dirs, deferred/rejected candidates), then `git worktree remove
   --force` + `git worktree prune` the staging tree. Never `git add`/`reset`/`rm --cached`.
3. **Write the round memory note**, linked to the previous round's. It must carry:
   master SHA, the PR numbers, the **rejections with their reasons**, and the
   **domains proven clean** — the next round starts from this note's exclusion list.
4. **Report to the user**: the PR table, the judgment calls you flagged, the rejections,
   the exhausted domains, and cleanup status.

## Saturation is a finding

Yield decays across rounds. When most hunters need re-tasking, when negatives repeat a
domain a third time, or when reaching the quota requires filing borderlines — say so
plainly in the closing report and name where the remaining yield is concentrated. "A
round 6 will likely return fewer than 10 without new surface landing first" is more
useful than ten thin PRs. Do not pad a round to hit N.

## Checklist

- [ ] Master pulled; exclusion list re-derived from this master + carried exhausted domains
- [ ] User's active work fenced as a positive constraint, in every prompt
- [ ] N disjoint domains, spawned in one message, each told the absolute worktree path
- [ ] Every candidate vetted by *you* at 3/3 on freshly built binaries
- [ ] Rejection classes walked; every claimed duplicate verified with `gh`
- [ ] Borderlines filed with the relationship flagged, not dropped
- [ ] One draft PR per bug, test-only, off master
- [ ] Both trees clean, staging worktree removed, index untouched
- [ ] Memory note written with PRs + rejections + clean domains
- [ ] Saturation stated honestly
