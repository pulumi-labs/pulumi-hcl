# vendored

Third-party code copied verbatim from upstream projects.

## What lives here

| Path | Upstream | License |
| ---- | -------- | ------- |
| `communicator/` | [`opentofu/opentofu`](https://github.com/opentofu/opentofu) `internal/communicator/` | MPL-2.0 |
| `ipaddr/` | [`opentofu/opentofu`](https://github.com/opentofu/opentofu) `internal/ipaddr/` | BSD-3-Clause (Go Authors) |
| `hcl2shim/` | [`opentofu/opentofu`](https://github.com/opentofu/opentofu) `internal/configs/hcl2shim/` | MPL-2.0 |
| `statefile/` | [`opentofu/opentofu`](https://github.com/opentofu/opentofu) `internal/states/statefile/` | MPL-2.0 |
| `states/` | [`opentofu/opentofu`](https://github.com/opentofu/opentofu) `internal/states/` | MPL-2.0 |
| `addrs/` | [`opentofu/opentofu`](https://github.com/opentofu/opentofu) `internal/addrs/` | MPL-2.0 |
| `tfdiags/` | [`opentofu/opentofu`](https://github.com/opentofu/opentofu) `internal/tfdiags/` | MPL-2.0 |
| `marks/` | [`opentofu/opentofu`](https://github.com/opentofu/opentofu) `internal/lang/marks/` | MPL-2.0 |
| `checks/` | [`opentofu/opentofu`](https://github.com/opentofu/opentofu) `internal/checks/` (Status types only) | MPL-2.0 |
| `legacy/hcl2shim/` | [`opentofu/opentofu`](https://github.com/opentofu/opentofu) `internal/legacy/hcl2shim/` (flatmap only) | MPL-2.0 |
| `version/` | [`opentofu/opentofu`](https://github.com/opentofu/opentofu) `version/` | MPL-2.0 |

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
| `github.com/opentofu/opentofu/internal/states...` | `github.com/pulumi-labs/pulumi-hcl/vendored/states...` (statefile likewise) |
| `github.com/opentofu/opentofu/internal/{addrs,tfdiags,checks}` | `github.com/pulumi-labs/pulumi-hcl/vendored/{addrs,tfdiags,checks}` |
| `github.com/opentofu/opentofu/internal/lang/marks` | `github.com/pulumi-labs/pulumi-hcl/vendored/marks` |
| `github.com/opentofu/opentofu/internal/legacy/hcl2shim` | `github.com/pulumi-labs/pulumi-hcl/vendored/legacy/hcl2shim` |
| `github.com/opentofu/opentofu/internal/encryption` | `github.com/pulumi-labs/pulumi-hcl/pkg/util/encryption` (shim) |
| `github.com/opentofu/opentofu/internal/configs` | `github.com/pulumi-labs/pulumi-hcl/pkg/util/configs` (shim) |
| `github.com/opentofu/opentofu/version` | `github.com/pulumi-labs/pulumi-hcl/vendored/version` |

The second target is an in-tree shim (Apache-2.0) that defines the small
`UIOutput` interface the vendored SSH package needs.

## License

Files under `communicator/` and `hcl2shim/` carry MPL-2.0 headers and remain MPL-2.0. Any
modification you make to those files (including hand-edits, which you should
not be making) must stay MPL-2.0. New files outside `vendored/` are
Apache-2.0 like the rest of the project. The MPL-2.0 license text is in
`LICENSE-MPL-2.0`; attribution is in `NOTICE`.

Files under `ipaddr/` are a fork of the Go standard library and remain under
the BSD-3-Clause license of the Go Authors; that license and patent grant are
carried in `ipaddr/LICENSE` and `ipaddr/PATENTS` and must be preserved.
