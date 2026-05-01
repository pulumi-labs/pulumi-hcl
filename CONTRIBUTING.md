# Contributing

## Release process

Releases are driven by [changie](https://changie.dev) and [GoReleaser](https://goreleaser.com). The flow is:

1. **Add a changelog fragment with every user-visible PR.** 

Run `changie new` from the repo root and answer the prompts:
- **Component**: one of `language-host`, `converter`, `codegen`, `runtime`.
- **Kind**: `Improvements` (auto-bumps minor) or `Bug Fixes` (auto-bumps patch).
- **Body**: a short description written from the user's perspective.
- **PR**: the GitHub PR number this change ships in.

This writes a file under `.changes/unreleased/`. Commit it with your PR.

2. **Cut a release PR.** 

When you're ready to ship, run:

```sh
changie batch auto      # or an explicit version, e.g. `changie batch v0.2.0`
changie merge
```

`changie batch` consumes everything in `.changes/unreleased/` and writes
`.changes/<version>.md`. `changie merge` rewrites the top-level `CHANGELOG.md` from the
header template plus all batched versions. Open a PR titled "Release `<version>`" with
just those two changes.

3. **Merge the release PR to `master`.** 

The `Release` workflow fires on any push to `master` that touches `CHANGELOG.md`. It:
- reads the latest version with `changie latest`,
- creates and pushes a `v<version>` git tag,
- runs GoReleaser, which builds `pulumi-language-hcl` and `pulumi-converter-hcl` for
  linux/darwin/windows × amd64/arm64 and publishes them to a GitHub release whose body is
  `.changes/<version>.md`.

The workflow no-ops if the tag already exists, so re-running it is safe.

### Versioning

The `auto` argument to `changie batch` picks the next version from the fragments: any
`Improvements` entry bumps minor, otherwise `Bug Fixes` bumps patch. Override by passing
an explicit `vX.Y.Z` when a release needs a major bump or a different version than `auto`
would compute.
