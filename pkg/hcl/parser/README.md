# parser

Parses HCL source into the `ast` package's types.

## Conventions

- Decode scalar attributes (strings, bools, numbers) with
  `gohcl.DecodeExpression(attr.Expr, nil, &target)` rather than hand-rolling
  `attr.Expr.Value(nil)` plus a type check. It converts the way upstream's
  decoders do (e.g. a string `"true"` satisfies a bool argument), so manual
  strictness is a parity bug, not a safeguard. Addresses are the exception:
  they are traversals, not evaluable expressions — use
  `hcl.AbsTraversalForExpr`, then `ast.ParseTargetAddr` for
  moved/import/removed-style targets.
- The parser validates shape (what a block may contain), not cross-block
  semantics. Checks that need the whole configuration or the module tree —
  address-still-declared, cross-block duplicates — belong in `graph`, which
  sees the merged, root-relative view.
