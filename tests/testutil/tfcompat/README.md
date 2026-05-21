# tfcompat — Terraform compatibility test harness

`tfcompat` verifies that running a Terraform `.tf` program through the real
Pulumi engine + `pulumi-language-hcl` runtime produces the same observable
behavior as running the same program through `tofu apply`.

Two paths share one wrapped TF provider per name. Both paths see the same
SDK-level CRUD calls; recordings of those calls are compared between paths
along with stack outputs.

## Layout

Tests live in `tests/tfcompat/` (one `_test.go` per case). The shared
harness, reusable providers, and sample testdata index live here under
`tests/testutil/tfcompat/`.

```
tests/
├── tfcompat/                       # one _test.go per case
│   ├── l2_simple_resource_test.go
│   └── testdata/
│       └── cases/
│           └── <case-name>/
│               └── *.tf
└── testutil/
    ├── tfcompat/                   # shared harness (this package)
    │   └── providers/              # reusable in-memory TF providers
    ├── pulexec/                    # `pulumi up` runner
    └── tfexec/                     # `tofu apply` runner + Recorder
```

## What's compared

Each test runs both paths in parallel against the same wrapped providers:

- **Path A** — `tofu apply` against TF providers via reattach.
- **Path B** — `pulumi up` against bridged TF providers via
  `pulumi-language-hcl`.

The harness asserts equality of two things:

1. **Stack outputs** — `terraform.tfstate` outputs vs. Pulumi stack outputs.
2. **Provider operations** — set-equal recordings of every `CreateContext`,
   `ReadContext`, `UpdateContext`, `DeleteContext`, and data-source
   `ReadContext` call. Order-independent (sorted by kind/type/inputs).

Recordings are captured at the `*schema.Provider` CRUD boundary so both
transports produce identical shapes when behavior matches.

## Test levels

- **L1** tests do not exercise provider code (literals, expressions,
  built-in functions).
- **L2** tests exercise provider code (custom resources, invokes,
  computed outputs).

Naming follows `pulumi-converter-terraform/tests/conformance/`.

## Running

```bash
go test ./tests/tfcompat/ -v -count=1
```

Requires `tofu` (default) or `terraform` on `PATH`. Override with
`TF_COMMAND_OVERRIDE=terraform`.

## Writing a new case

1. Create `tests/tfcompat/testdata/cases/<case-name>/main.tf` (plus any
   additional `.tf` or auxiliary files).
2. Add `tests/tfcompat/<case-name>_test.go`:

   ```go
   func TestL2MyCase(t *testing.T) {
       t.Parallel()
       tfcompat.RunCase(t, "<case-name>", tfcompat.Case{
           Providers: []tfcompat.Provider{
               {Name: "simple", Factory: providers.SimpleProvider},
           },
       })
   }
   ```

3. Reuse providers from `testutil/tfcompat/providers` or add a new
   in-memory provider there.

## Why `.tf` (not `.hcl`)

OpenTofu only picks up `.tf` / `.tf.json`. `pulumi-language-hcl`'s parser
also picks up `.tf` so the same file feeds both paths verbatim.

## Scope

Pulumi HCL is a superset of Terraform HCL, so not every well-formed HCL
program is a tfcompat candidate — only the TF-compatible subset is. In
particular:

- A `terraform { required_providers { ... } }` block with a
  `pulumi/*`-style source has no meaning on the TF side; both paths
  discover the provider from the resource type prefix and from
  `opttest.AttachProvider`, so fixtures can omit the block.
- Pulumi-only constructs (component / package blocks, Pulumi-specific
  resource options, parameterized providers, etc.) belong in the
  language-host test suite, not here.

Only Create + DataSource Read are exercised by the first test. The
recorder shapes for Update/Delete are wired but not yet covered by a
case.
