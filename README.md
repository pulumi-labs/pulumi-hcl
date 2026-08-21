# Pulumi HCL Language Plugin

A Pulumi language plugin that enables running Pulumi against a Terraform HCL IaC program.

## Overview

This plugin allows you to use familiar Terraform/HCL syntax while leveraging Pulumi's state management, secrets handling, and cloud platform. It parses HCL files and translates them to Pulumi resource registrations at runtime.

See the [language reference](docs/language-reference.md) for the available syntax and the [execution model](docs/execution-model.md) for dependency, preview, and failure semantics.

```hcl
# main.tf
resource "aws_s3_bucket" "my_bucket" {
  bucket = "my-unique-bucket-name"

  tags = {
    Environment = "dev"
    ManagedBy   = "Pulumi"
  }
}

output "bucket_arn" {
  value = aws_s3_bucket.my_bucket.arn
}
```

## Installation

Pulumi HCL requires the [`pulumi`](https://github.com/pulumi/pulumi) CLI v3.256.0 or later. The CLI downloads the
language and converter plugins automatically the first time you use them — there is nothing to install by hand.

To build the plugins from source for development, install them directly onto your path:

```bash
go install github.com/pulumi/pulumi-hcl/cmd/pulumi-language-hcl@latest  # for the language
go install github.com/pulumi/pulumi-hcl/cmd/pulumi-converter-hcl@latest # for the converter
```

## Usage

1. Create a `Pulumi.yaml` with `runtime: hcl`:

```yaml
name: my-project
runtime: hcl
description: My HCL project
```

2. Create HCL files (`.tf` extension):

```hcl
# main.tf
resource "random_pet" "my_pet" {
  length = 2
}

output "pet_name" {
  value = random_pet.my_pet.id
}
```

3. Run Pulumi commands as usual:

```bash
pulumi up
```

## Design Overview

### Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                        Pulumi Engine                             │
│  ┌─────────────┐  ┌──────────────┐  ┌─────────────────────────┐ │
│  │   CLI       │  │ State Mgmt   │  │   Provider Plugins      │ │
│  └─────────────┘  └──────────────┘  └─────────────────────────┘ │
└─────────────────────────────────────────────────────────────────┘
                              │
                              │ gRPC (LanguageRuntime)
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│                   pulumi-language-hcl                            │
│  ┌─────────────────────────────────────────────────────────────┐│
│  │ Server (pkg/server)                                         ││
│  │  - LanguageRuntimeServer gRPC implementation                ││
│  │  - GetRequiredPlugins, Run, GetProgramDependencies          ││
│  └─────────────────────────────────────────────────────────────┘│
│  ┌─────────────────────────────────────────────────────────────┐│
│  │ Parser (pkg/hcl/parser)                                     ││
│  │  - Uses hashicorp/hcl/v2 (MPL licensed)                     ││
│  │  - Parses *.tf files into AST                               ││
│  └─────────────────────────────────────────────────────────────┘│
│  ┌─────────────────────────────────────────────────────────────┐│
│  │ AST (pkg/hcl/ast)                                           ││
│  │  - Config, Resource, Variable, Local, Output, Provider      ││
│  │  - Terraform-compatible block structures                    ││
│  └─────────────────────────────────────────────────────────────┘│
│  ┌─────────────────────────────────────────────────────────────┐│
│  │ Graph (pkg/hcl/graph)                                       ││
│  │  - Dependency extraction from HCL expressions               ││
│  │  - Topological sort for execution order                     ││
│  │  - Parallel execution scheduler                             ││
│  └─────────────────────────────────────────────────────────────┘│
│  ┌─────────────────────────────────────────────────────────────┐│
│  │ Evaluator (pkg/hcl/eval)                                    ││
│  │  - HCL expression evaluation                                ││
│  │  - Terraform-compatible function library                    ││
│  │  - Variable/resource reference resolution                   ││
│  └─────────────────────────────────────────────────────────────┘│
│  ┌─────────────────────────────────────────────────────────────┐│
│  │ Packages (pkg/hcl/packages)                                 ││
│  │  - Pulumi provider schema loading                           ││
│  │  - TF resource type → Pulumi token mapping                  ││
│  │  - Cached provider info for fast startup                    ││
│  └─────────────────────────────────────────────────────────────┘│
│  ┌─────────────────────────────────────────────────────────────┐│
│  │ Run Engine (pkg/hcl/run)                                    ││
│  │  - Orchestrates execution                                   ││
│  │  - Registers resources with Pulumi                          ││
│  │  - Handles count/for_each expansion                         ││
│  └─────────────────────────────────────────────────────────────┘│
│  ┌─────────────────────────────────────────────────────────────┐│
│  │ Transform (pkg/hcl/transform)                               ││
│  │  - cty.Value ↔ Pulumi PropertyValue conversion              ││
│  │  - camelCase ↔ snake_case name mapping                      ││
│  └─────────────────────────────────────────────────────────────┘│
└─────────────────────────────────────────────────────────────────┘
```

### Execution Flow

1. **Parse**: HCL files are parsed using `hashicorp/hcl/v2` into an AST
2. **Graph**: Dependencies are extracted and a DAG is built
3. **Execute**: Nodes are processed in parallel where dependencies allow:
   - Variables → set in evaluation context
   - Locals → evaluated and stored
   - Resources/Data Sources → registered with Pulumi (parallel)
   - Outputs → collected and registered on stack

### Type Resolution

Pulumi HCL supports Terraform-style resource type names:

```hcl
# Terraform-style
resource "aws_ec2_instance" "web" { }      # → aws:ec2/instance:Instance
```

Type resolution is conducted with the following algorithm:

1. The provider package name is extracted from the first underscore-delimited segment of the HCL type (e.g. `aws` from `aws_s3_bucket`). If `required_providers` is specified, the longest matching provider prefix is used to resolve ambiguity.
2. The provider's schema is loaded from the Pulumi registry.
3. Each resource in the provider schema has its module path and resource name lowercased and joined, with all underscores and slashes stripped, forming a lookup key.
4. The remaining portion of the HCL type (after the provider prefix) is compared against these keys (also with underscores stripped) to find the matching resource.

For example, `aws_ec2_instance` → provider `aws`, lookup key `ec2instance` → matches `aws:ec2/instance:Instance` in the AWS schema.

## Multi-Language Components

HCL modules can be published as reusable Pulumi components consumable from any language. See [docs/mlc.md](docs/mlc.md) for details on authoring MLCs with the `terraform { component { ... } package { ... } }` syntax.

## Terraform Compatibility

This plugin supports the majority of Terraform's HCL syntax. For detailed compatibility information and known limitations, see [docs/terraform-compatibility.md](docs/terraform-compatibility.md).

### Supported

- `resource` blocks with all meta-arguments (`count`, `for_each`, `depends_on`, `lifecycle`, `timeouts`)
- `data` source blocks
- `variable` blocks with defaults and types
- `locals` blocks
- `output` blocks
- `provider` blocks (including `alias` for multiple configurations)
- `terraform` block with `required_providers`
- `module` blocks (local, Git, Terraform Registry, HTTP sources)
- `provisioner` blocks (`local-exec`, `remote-exec`, `file`)
- `dynamic` blocks
- `moved` blocks (map to Pulumi aliases)
- `import` blocks (map to Pulumi import option)
- `check` blocks (non-blocking assertions, with optional scoped data sources)
- `lifecycle` meta-arguments, including `replace_triggered_by`
- Most Terraform built-in functions
- Resource and data source references
- Splat expressions (`resource.name[*].attr`)

### Not Supported

- `backend`, `required_version`, `provider_meta`, and `experiments` in the `terraform` block — accepted but ignored with a warning; Pulumi manages state independently
- `cloud` blocks in the `terraform` block — not accepted at all; a `cloud` block is a parse error
- WinRM `connection` blocks — `connection` supports `type = "ssh"` only
- `List<Object>` empty vs null distinction: HCL block syntax cannot distinguish between an empty and null `List<Object>`, which is a known incompatibility with some Pulumi programs

### Pulumi-Specific Extensions

```hcl
# Stack references
resource "pulumi_stack_reference" "network" {
  name = "myorg/networking/prod"
}

output "vpc_id" {
  value = pulumi_stack_reference.network.outputs["vpc_id"]
}
```

```hcl
# Method calls on resources
resource "aws_s3_bucket" "my_bucket" {
  bucket = "my-unique-bucket-name"
}

call "my_bucket" "get_object" {
  key = "config.json"
}
```

The `call` block invokes a method on an existing resource. The first label is the resource's logical name (matching a declared resource) and the second is the method name. Results are referenced as `call.<resource>.<method>.<output>`.

Three built-in functions provide access to a resource's Pulumi identity at runtime:
- `pulumiresourcename(resource)` — returns the logical name from the resource's URN
- `pulumiresourcetype(resource)` — returns the type token from the resource's URN
- `pulumiresourceurn(resource)` — returns the resource's URN

## License

Apache 2.0 - See [LICENSE](LICENSE) for details.

Note: This project uses `github.com/hashicorp/hcl/v2` which is licensed under MPL 2.0.
