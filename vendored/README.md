# vendored

Third-party code copied verbatim from upstream projects.

## What lives here

| Path | Upstream | License |
| ---- | -------- | ------- |
| `communicator/` | [`opentofu/opentofu`](https://github.com/opentofu/opentofu) `internal/communicator/` | MPL-2.0 |

## Pinned revision

The pinned upstream commit is the argument passed to `go:generate` in
`doc.go`. To bump:

1. Update the SHA in `//go:generate` directive of `doc.go`.
2. Run `go generate ./vendored`.
3. Commit the resulting changes under `vendored/` together with the SHA bump.

## CI enforcement

CI runs `go generate ./vendored` and then `git diff --exit-code -- vendored/`.
Any drift between the committed tree and what the regen script produces fails
the build. Do not edit files under `vendored/communicator/` by hand.

## Import rewrites

`regen.sh` rewrites two import paths so the vendored code references our
in-tree packages:

| Upstream import | Rewritten to |
| --------------- | ------------ |
| `github.com/opentofu/opentofu/internal/communicator/...` | `github.com/pulumi-labs/pulumi-hcl/vendored/communicator/...` |
| `github.com/opentofu/opentofu/internal/provisioners` | `github.com/pulumi-labs/pulumi-hcl/pkg/provisioner/provisioners` |

The second target is an in-tree shim (Apache-2.0) that defines the small
`UIOutput` interface the vendored SSH package needs.

## License

Files under `communicator/` carry MPL-2.0 headers and remain MPL-2.0. Any
modification you make to those files (including hand-edits, which you should
not be making) must stay MPL-2.0. New files outside `vendored/` are
Apache-2.0 like the rest of the project. The MPL-2.0 license text is in
`LICENSE-MPL-2.0`; attribution is in `NOTICE`.
