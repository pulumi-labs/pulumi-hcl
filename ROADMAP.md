# Pulumi HCL Language Plugin - Product Roadmap

> **Document Purpose**: Detailed implementation specifications for enhancing
> Terraform/OpenTofu compatibility and developer experience.
>
> **Last Updated**: 2025-01-16

## Executive Summary

The pulumi-language-hcl plugin provides ~90% Terraform HCL compatibility. This
roadmap addresses the remaining gaps to enable zero-friction migration from
Terraform/OpenTofu while leveraging Pulumi's state management, secrets, and
policy features.

### Architecture Boundary

Understanding what this plugin does vs. what Pulumi handles:

| Responsibility | Owner | Notes |
|----------------|-------|-------|
| HCL Parsing & Evaluation | **This Plugin** | Core responsibility |
| Dependency Graph & Execution | **This Plugin** | Orchestration |
| Resource Registration | **Plugin → Pulumi** | Plugin translates, Pulumi persists |
| State Management | **Pulumi Engine** | Plugin never touches state |
| Secrets Encryption | **Pulumi Engine** | Plugin marks sensitive, Pulumi encrypts |
| Policy Enforcement | **CrossGuard** | Works automatically |
| Cost Estimation | **Pulumi Cloud** | Not plugin concern |

---

## Priority Tiers

- **🔴 Critical (P0)**: Blocks migration for significant portion of users
- **🟠 High (P1)**: Significant UX improvement or common use case
- **🟡 Medium (P2)**: Nice-to-have, improves completeness
- **🟢 Low (P3)**: Edge cases or future consideration

---

## Feature Specifications

---

### Feature 1: Dynamic Blocks Support

**Priority**: 🔴 Critical (P0)
**Status**: NOT IMPLEMENTED
**Effort**: Medium (2-3 weeks)
**Impact**: Unblocks ~40% of production Terraform configurations

#### Problem Statement

Dynamic blocks are heavily used in production Terraform code for generating
repeated nested blocks programmatically. Without this feature, many existing
Terraform configurations cannot run on Pulumi HCL.

```hcl
# Currently FAILS - blocks migration for many users
resource "aws_security_group" "example" {
  dynamic "ingress" {
    for_each = var.ingress_rules
    content {
      from_port   = ingress.value.port
      to_port     = ingress.value.port
      protocol    = "tcp"
      cidr_blocks = ingress.value.cidrs
    }
  }
}
```

#### Competitive Research

**Terraform/HashiCorp Implementation:**

- Uses `hashicorp/hcl/v2/ext/dynblock` package for expansion
- Syntax: `dynamic "block_name" { for_each = ..., iterator = ..., content { } }`
- Iterator variables: `block_name.key`, `block_name.value` (or custom via
  `iterator`)
- Source:
  [Terraform Dynamic Blocks Docs](https://developer.hashicorp.com/terraform/language/expressions/dynamic-blocks)

**OpenTofu Implementation:**

- Identical to Terraform - uses same HCL library and dynblock package
- Full compatibility with Terraform dynamic block syntax
- Source:
  [OpenTofu Dynamic Blocks](https://notes.kodekloud.com/docs/OpenTofu-A-Beginners-Guide-to-a-Terraform-Fork-Including-Migration-From-Terraform/OpenTofu-Functions-and-Conditional-Expressions/Dynamic-Blocks-and-Splat-Expressions)

**Key Finding:** The `hashicorp/hcl/v2` library (already in go.mod as v2.23.0)
includes `ext/dynblock` package with full dynamic block expansion support. No
new dependencies required.

#### Technical Analysis

**Current State:**

1. **Parser** (`pkg/hcl/parser/schema.go:94-107`): `resourceSchema` does not
   include `dynamic` block type
2. **Resource Processing** (`pkg/hcl/run/run.go:739-755`): Uses
   `JustAttributes()` which ignores all nested blocks
3. **Evaluation Context** (`pkg/hcl/eval/context.go`): Already has `SetCount()`
   and `SetEach()` - infrastructure ready

**Root Cause:** Dynamic blocks are neither parsed nor expanded; nested blocks
are ignored during resource registration.

#### Implementation Specification

##### Phase 1: Schema & AST (Days 1-2)

**File: `pkg/hcl/parser/schema.go`**

```go
// Add to resourceSchema.Blocks array (around line 106)
{Type: "dynamic", LabelNames: []string{"type"}},
```

**File: `pkg/hcl/ast/dynamic.go` (NEW FILE)**

```go
package ast

import "github.com/hashicorp/hcl/v2"

// DynamicBlock represents a dynamic block that generates nested blocks
type DynamicBlock struct {
    // Type is the block type to generate (e.g., "ingress", "filter")
    Type string

    // ForEach is the expression to iterate over (required)
    ForEach hcl.Expression

    // Iterator is the name of the iterator variable (defaults to Type)
    Iterator string

    // Labels are additional labels for the generated blocks (optional)
    Labels hcl.Expression

    // Content is the body template for generated blocks
    Content hcl.Body

    // DeclRange is the source range for error reporting
    DeclRange hcl.Range
}
```

**File: `pkg/hcl/ast/resource.go`**

```go
// Add to Resource struct
type Resource struct {
    // ... existing fields ...

    // DynamicBlocks contains dynamic block definitions for this resource
    DynamicBlocks []*DynamicBlock
}
```

##### Phase 2: Parser Implementation (Days 3-4)

**File: `pkg/hcl/parser/parser.go`**

Add parsing logic in `parseResourceBlock()` function:

```go
// After existing block handling (lifecycle, connection, etc.)
case "dynamic":
    dynBlock, diags := parseDynamicBlock(block)
    if diags.HasErrors() {
        return nil, diags
    }
    res.DynamicBlocks = append(res.DynamicBlocks, dynBlock)
```

```go
func parseDynamicBlock(block *hcl.Block) (*ast.DynamicBlock, hcl.Diagnostics) {
    var diags hcl.Diagnostics

    dynBlock := &ast.DynamicBlock{
        Type:      block.Labels[0],
        Iterator:  block.Labels[0], // Default to block type
        DeclRange: block.DefRange,
    }

    content, _, moreDiags := block.Body.PartialContent(&hcl.BodySchema{
        Attributes: []hcl.AttributeSchema{
            {Name: "for_each", Required: true},
            {Name: "iterator"},
            {Name: "labels"},
        },
        Blocks: []hcl.BlockHeaderSchema{
            {Type: "content"},
        },
    })
    diags = append(diags, moreDiags...)

    // Extract for_each (required)
    if attr, exists := content.Attributes["for_each"]; exists {
        dynBlock.ForEach = attr.Expr
    }

    // Extract iterator (optional)
    if attr, exists := content.Attributes["iterator"]; exists {
        val, moreDiags := attr.Expr.Value(nil)
        diags = append(diags, moreDiags...)
        if !moreDiags.HasErrors() {
            dynBlock.Iterator = val.AsString()
        }
    }

    // Extract labels (optional)
    if attr, exists := content.Attributes["labels"]; exists {
        dynBlock.Labels = attr.Expr
    }

    // Extract content block (required)
    for _, blk := range content.Blocks {
        if blk.Type == "content" {
            dynBlock.Content = blk.Body
            break
        }
    }

    return dynBlock, diags
}
```

##### Phase 3: Expansion Logic (Days 5-8)

**Option A: Use dynblock.Expand() (Recommended)**

**File: `pkg/hcl/run/run.go`**

```go
import "github.com/hashicorp/hcl/v2/ext/dynblock"

// In registerResourceInstance(), before line 740
func (e *Engine) expandDynamicBlocks(body hcl.Body) (hcl.Body, hcl.Diagnostics) {
    // Use the library's built-in expansion
    return dynblock.Expand(body, e.evaluator.Context().HCLContext()), nil
}
```

##### Phase 4: Integration (Days 6-8)

**File: `pkg/hcl/run/run.go`**

Modify `registerResourceInstance()` to use expansion before processing attributes.

---

### Feature 2: Workspace → Stack Name Mapping

**Priority**: 🔴 Critical (P0)
**Status**: HARDCODED TO "default"
**Effort**: Trivial (1-2 hours)
**Impact**: Enables environment-aware configurations

#### Problem Statement

The `terraform.workspace` variable is hardcoded to `"default"`, breaking
configurations that use workspaces for environment differentiation.

#### Implementation Specification

**File: `pkg/hcl/run/run.go`**

In `NewEngine()` function:

```go
// NEW: Set workspace to stack name for terraform.workspace compatibility
if opts.StackName != "" {
    // Extract just the stack portion from "org/project/stack" format
    workspace := extractStackName(opts.StackName)
    evaluator.Context().SetWorkspace(workspace)
}
```

---

### Feature 3: Stack Reference Data Source

**Priority**: 🔴 Critical (P0)
**Status**: DOCUMENTED BUT NOT IMPLEMENTED
**Effort**: Medium (3-5 days)
**Impact**: Enables cross-stack references essential for multi-stack architectures

#### Problem Statement

The README documents `data "pulumi_stack_reference"` but it's not actually
implemented.

#### Implementation Specification

**File: `pkg/hcl/run/run.go`**

Handle `pulumi_stack_reference` specially in `processDataSource`:
1. Extract `name` attribute.
2. Register as `pulumi:pulumi:StackReference` resource.
3. Extract `outputs` and `secretOutputNames` from response.
4. Set data source output to `{ name: ..., outputs: ... }`.

---

### Feature 4: terraform_remote_state Migration Warning

**Priority**: 🟡 Medium (P2)
**Status**: SILENTLY FAILS
**Effort**: Trivial (2-4 hours)
**Impact**: Guides users to correct solution during migration

#### Implementation Specification

**File: `pkg/hcl/run/run.go`**

Intercept `terraform_remote_state` data sources and return a helpful error
message explaining how to migrate to `pulumi_stack_reference`.

---

### Feature 5: Top-Level Check Blocks

**Priority**: 🟠 High (P1)
**Status**: NOT IMPLEMENTED
**Effort**: Medium (1-2 weeks)
**Impact**: Enables infrastructure validation patterns (Terraform 1.5+)

#### Implementation Specification

1. **Schema**: Add `check` to `rootSchema`.
2. **AST**: Create `Check` and `Assertion` types.
3. **Parser**: Parse check blocks and assertions.
4. **Execution**: Run checks after all resources are processed.
5. **Logic**: Failures should emit warnings, not errors.

---

### Feature 6: `replace_triggered_by` Lifecycle Option

**Priority**: 🟡 Medium (P2)
**Status**: PARSED BUT NOT IMPLEMENTED
**Effort**: Medium (1 week)
**Impact**: Enables dependency-triggered replacements

#### Implementation Specification

Create a synthetic input property (hash of watched values) and add it to the
`ReplaceOnChanges` list during resource registration.

---

### Feature 7: `pulumi hcl validate` Command

**Priority**: 🟠 High (P1)
**Status**: NOT IMPLEMENTED
**Effort**: Low (2-3 days)
**Impact**: Fast feedback loop for configuration errors

#### Implementation Specification

Add `ValidateProgram` to `LanguageHost` which:
1. Parses HCL.
2. Builds dependency graph.
3. Checks for unused variables/locals.
4. Validates types.
5. Returns diagnostics without execution.

---

### Feature 8: Enhanced Error Messages

**Priority**: 🟠 High (P1)
**Status**: BASIC ERRORS
**Effort**: Medium (1-2 weeks)
**Impact**: Significantly improved developer experience

#### Implementation Specification

Implement structured diagnostics with:
- Source snippets
- "Did you mean" suggestions
- Contextual help
- Documentation links

---

### Feature 9: OpenTofu Registry Support

**Priority**: 🟡 Medium (P2)
**Status**: NOT IMPLEMENTED
**Effort**: Low (1-2 days)
**Impact**: Support OpenTofu ecosystem users

#### Implementation Specification

**File: `pkg/hcl/packages/packages.go`**

Automatically map `opentofu/` provider sources to `pulumi/` (identical to
existing `hashicorp/` mapping).

---

### Feature 10: HCL → Other Languages Converter

**Priority**: 🟠 High (P1)
**Status**: STUB EXISTS, NOT IMPLEMENTED
**Effort**: High (4-6 weeks)
**Impact**: Enables "graduation" path from HCL to full languages

#### Implementation Specification

Implement `ConvertProgram` gRPC method in `cmd/pulumi-converter-hcl` to transform
HCL AST to PCL (Pulumi Configuration Language).

---

### Feature 11: Native Pulumi Provider Support

**Priority**: 🟡 Medium (P2)
**Status**: PARTIAL
**Effort**: Medium (1-2 weeks)
**Impact**: Access to 100+ Pulumi-native providers

#### Problem Statement

Currently optimized for Terraform-bridged providers. Native Pulumi providers
(Kubernetes, cloud-native, etc.) may not work correctly with HCL.

#### Implementation Specification

**File: `pkg/hcl/packages/packages.go`**

Enhance package resolution to support native Pulumi type tokens:
1. Detect native tokens (e.g., `kubernetes:apps/v1:Deployment`, `aws-native:vpc:Vpc`).
2. Skip Terraform-specific name mapping for these tokens.
3. Allow direct instantiation of any Pulumi resource via its URN-like type.

---

### Feature 12: Private Module Registry Authentication

**Priority**: 🟡 Medium (P2)
**Status**: NOT IMPLEMENTED
**Effort**: Medium (1 week)
**Impact**: Enterprise users with private modules

#### Implementation Specification

Implement authentication for private Terraform registries using `.terraformrc` or environment variables (`TF_TOKEN_...`).

---

### Feature 13: Debug Mode / Verbose Logging

**Priority**: 🟢 Low (P3)
**Status**: MINIMAL
**Effort**: Low (2-3 days)
**Impact**: Troubleshooting and development

#### Implementation Specification

Add `PULUMI_HCL_DEBUG` environment variable support to emit verbose logging.

---

### Feature 14: WinRM Connection Support

**Priority**: 🟢 Low (P3)
**Status**: NOT IMPLEMENTED
**Effort**: Medium (1 week)
**Impact**: Windows server provisioning

#### Implementation Specification

Add WinRM support to provisioners via `command` provider.

---

### Feature 15: Automatic Backend State Import Tool

**Priority**: 🔴 Critical (P0)
**Status**: NOT IMPLEMENTED
**Effort**: Medium (2 weeks)
**Impact**: Automates the most dangerous part of migration (state transfer)

#### Problem Statement

Users migrating from Terraform often have existing state in S3/GCS. Manually
importing resources using `pulumi import` is error-prone and tedious for large
stacks.

#### Implementation Specification

Create a new CLI command `pulumi hcl import-state` that:
1. Reads `terraform.tfstate` (local or remote backend).
2. Maps Terraform resource addresses (e.g., `aws_instance.web`) to Pulumi URNs.
3. Generates a Pulumi import file (`import.json`) or directly imports state.

**Command Design:**
```bash
$ pulumi hcl import-state --tfstate ./terraform.tfstate --stack dev
Importing 45 resources...
✓ aws_vpc.main -> aws:ec2/vpc:Vpc (vpc-12345)
✓ aws_instance.web -> aws:ec2/instance:Instance (i-abcde)
Successfully imported 45 resources to stack 'dev'.
```

**Technical Logic:**
- Use `pkg/hcl/packages` to resolve TF types to Pulumi types.
- Map resource names: `type.name` -> `name`.
- Use `pulumi stack import` format for bulk import.

---

### Feature 16: Hybrid HCL Component

**Priority**: 🟠 High (P1)
**Status**: NOT IMPLEMENTED
**Effort**: High (3-4 weeks)
**Impact**: Allows using HCL modules from TypeScript/Python programs

#### Problem Statement

Teams want to migrate incrementally, keeping some legacy HCL modules while
writing new infrastructure in TypeScript/Python.

#### Implementation Specification

Create a Pulumi ComponentResource `hcl.Module` (in a new SDK) that:
1. Accepts a `source` path to an HCL module.
2. Accepts `inputs` (variables).
3. Spawns the `pulumi-language-hcl` engine in-process or as a sidecar.
4. Registers resources as children of the component.
5. Exposes module outputs as component outputs.

**User Experience (TypeScript):**
```typescript
import * as hcl from "@pulumi/hcl";

const vpc = new hcl.Module("legacy-vpc", {
    source: "./modules/vpc",
    inputs: {
        cidr: "10.0.0.0/16",
    },
});

export const vpcId = vpc.outputs["vpc_id"];
```

---

### Feature 17: LSP / IDE Support

**Priority**: 🟢 Low (P3) - Separate Project
**Status**: NOT IMPLEMENTED
**Effort**: Very High (3-6 months)
**Impact**: Professional developer experience

#### Implementation Specification

Separate project (`pulumi-hcl-lsp`) implementing Language Server Protocol.

---

## Summary

### Implementation Order (Recommended)

| Phase | Features | Timeline |
|-------|----------|----------|
| **1** | 2 (Workspace), 4 (Remote State Warning), 9 (OpenTofu) | 1 week |
| **2** | 1 (Dynamic Blocks), 3 (Stack Reference) | 3 weeks |
| **3** | 15 (Auto State Import), 5 (Check Blocks), 7 (Validate Command) | 3 weeks |
| **4** | 16 (Hybrid Component), 8 (Error Messages) | 4 weeks |
| **5** | 10 (Converter), 6 (replace_triggered_by) | 4-6 weeks |
| **6** | 11-14 (Native Providers, Registry Auth, Debug, WinRM) | As needed |
| **Future** | 17 (LSP) | Separate project |

### Quick Wins (< 1 day each)

1. Feature 2: Workspace mapping (~2 hours)
2. Feature 4: terraform_remote_state warning (~2 hours)
3. Feature 9: OpenTofu registry (~2 hours)
4. Feature 13: Debug mode (~4 hours)

### High Impact / Medium Effort

1. Feature 1: Dynamic blocks (2-3 weeks)
2. Feature 3: Stack references (3-5 days)
3. Feature 15: Auto State Import (2 weeks)
4. Feature 16: Hybrid Component (3-4 weeks)