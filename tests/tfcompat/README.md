# tfcompat test cases

Each `*_test.go` file in this directory pairs a Terraform program (under
`testdata/cases/<case-name>/`) with both execution paths and asserts they agree
on outputs and on the sequence of provider CRUD calls.

The harness, recorder, and reusable providers live in
[`../testutil/tfcompat/`](../testutil/tfcompat/README.md) — start there for how the comparison works and how
to add a new in-memory provider.

## Layout

```
tfcompat/
├── *_test.go                       # one test per case
└── testdata/
    └── cases/
        └── <case-name>/
            └── *.tf                # fed verbatim to both paths
```

A test's `<case-name>` argument to `tfcompat.RunCase` selects the matching
directory under `testdata/cases/`.

## Running

```bash
go test ./tests/tfcompat/ -v -count=1
```

Run a single case:

```bash
go test ./tests/tfcompat/ -run TestL2SimpleResource -v -count=1
```

Requires `tofu` on `PATH` (or set `TF_COMMAND_OVERRIDE=terraform`).

## Adding a case

1. Create `testdata/cases/<case-name>/main.tf` (plus any other `.tf` files the
   case needs — every `.tf` in the directory is loaded).
2. Add `<case-name>_test.go`:

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

3. Reuse a provider from `../testutil/tfcompat/providers/` if one fits;
   otherwise add a new in-memory provider there rather than inline.

## Naming

- `l1_*` — no provider code is exercised (literals, expressions, built-in
  functions).
- `l2_*` — provider code is exercised (resources, data sources, computed
  outputs).

Tests should be named after the noun under test, not the behavior under test. For example,
to test that variable defaults are applied correctly, we the test might be called
`l1_var`, not `l1_var_defaults_applied`.

## Scope

Cases here must be valid Terraform programs — both paths run the same `.tf`
files. Pulumi-only constructs (component / package blocks, parameterized
providers, Pulumi-specific resource options) belong in the language-host test
suite, not here.
