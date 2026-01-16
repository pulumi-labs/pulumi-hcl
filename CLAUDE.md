# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with
code in this repository.

## Project Overview

Pulumi HCL Language Plugin - A Pulumi language host that enables writing
infrastructure-as-code using Terraform-compatible HCL syntax. Parses HCL files
and translates them to Pulumi resource registrations at runtime.

## Build & Development Commands

```bash
# Build binaries (outputs to bin/)
make build

# Run all tests with race detection
make test

# Run a specific test
go test -v -run TestName ./pkg/hcl/...

# Run tests with coverage
make test_cover

# Lint (uses golangci-lint)
make lint

# Format code
make fmt

# Install to $GOPATH/bin
make install

# Install to ~/.pulumi/bin for local testing
make dev
```

## Architecture

### Component Flow

```text
Pulumi Engine (gRPC) → LanguageHost → Parser → Graph → Engine → ResourceMonitor
```

1. **cmd/pulumi-language-hcl**: Entry point, starts gRPC server implementing
   `LanguageRuntimeServer`
2. **pkg/server**: gRPC interface implementation (`Run`, `GetRequiredPlugins`)
3. **pkg/hcl/parser**: HCL parsing using `hashicorp/hcl/v2`, produces AST
4. **pkg/hcl/ast**: Type definitions for parsed HCL blocks (Resource, Variable)
5. **pkg/hcl/graph**: Dependency extraction, topological sort, parallel
   execution scheduling
6. **pkg/hcl/eval**: Expression evaluation with Terraform-compatible functions
7. **pkg/hcl/run**: Execution engine - orchestrates resource registration
8. **pkg/hcl/packages**: Provider schema loading, TF→Pulumi type token mapping
9. **pkg/hcl/transform**: Type conversions: `cty.Value` ↔ `PropertyMap`,
   camelCase ↔ snake_case
10. **pkg/hcl/modules**: Remote and local module loading and caching

### Key Execution Phases

The engine processes nodes in specific phases (see `processNodesParallel` in
run.go:354):

1. **Variables** - sequential, sets up eval context
2. **Locals** - sequential, may depend on variables
3. **Resources/DataSources/Modules** - parallel with dependency tracking

### Type Resolution

Terraform-style types (e.g., `aws_instance`) are resolved to Pulumi tokens
(e.g., `aws:ec2/instance:Instance`) via:

1. Provider's `-get-provider-info` output (for bridged providers)
2. Cached in `~/.pulumi/plugins/resource-{provider}-v{version}/pulumi-hcl.cache`

### Name Conversion

- Input: snake_case → camelCase (sending to Pulumi)
- Output: camelCase → snake_case (reading from Pulumi)
- Handled by `transform.CtyToPropertyValue` and `camelToSnake` in run.go

## Key Types

- `ast.Config` - Complete parsed HCL configuration
- `graph.Node` - Dependency graph node with type enum (Variable, Local, etc.)
- `run.Engine` - Execution orchestrator holding eval context, package loader,
  resource monitor
- `packages.ProviderInfo` - TF→Pulumi type mappings loaded from provider
  binaries

## Testing

Tests use `TestMode` flag (`EngineOptions.TestMode = true`) to skip
provider/schema validation. The evaluator tests in `pkg/hcl/eval/` cover HCL
expression evaluation and Terraform function compatibility.

## Two Binaries

1. **pulumi-language-hcl** - Language host (main runtime)
2. **pulumi-converter-hcl** - Converter for migration tooling
