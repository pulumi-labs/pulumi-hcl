# Multi-Language Components (MLCs) in HCL

Multi-Language Components allow you to author reusable Pulumi components in HCL
that can be consumed from any Pulumi language (TypeScript, Python, Go, C#, Java, YAML, or HCL).

## Declaring a Component Module

An HCL module becomes an MLC when it has a `PulumiPlugin.yaml` containing
`runtime: hcl`. That file is all you need — the optional `component` and
`package` blocks inside the `terraform {}` block override the component's
identity and package metadata when the defaults don't suit:

```hcl
terraform {
  component {
    name = "VpcNetwork"
  }
  package {
    name    = "my-networking"
    version = "1.0.0"
  }
  required_providers {
    aws = {
      source  = "pulumi/aws"
      version = "6.0.0"
    }
  }
}

variable "cidr_block" {
  type    = string
  default = "10.0.0.0/16"
}

resource "aws_vpc" "vpc" {
  cidr_block = var.cidr_block
}

output "vpc_id" {
  value = aws_vpc.vpc.id
}
```

Note that a `pulumi/`-sourced provider takes a concrete semver version, not a
version constraint — `"6.0.0"`, never `">= 6.0"`.

A module with no `terraform` block at all is still a valid MLC. It takes the
default token `<package>:index:Module`, where `<package>` is the module's
directory name, and consumers in HCL reference it as `<package>_module`.

## Block Reference

### `component` block

Overrides the component's name and module segment. The block is optional; when
it is omitted, the component name defaults to `"Module"` and the module segment
to `"index"`. When the block is present, `name` is required.

| Field    | Required          | Default    | Description                                  |
|----------|-------------------|------------|----------------------------------------------|
| `name`   | Yes, if the block is present | `"Module"` | Component name (must be a valid Pulumi name) |
| `module` | No                | `"index"`  | Module segment of the resource token         |

With the default component name, the token is `{package}:index:Module` and is
referenced in HCL as `{package}_module` — e.g. a package `randommodule` is
consumed as `resource "randommodule_module" "x" { ... }`, mirroring the dynamic
`hcl:index:Module` resource (referenced as `hcl_module`).

### `package` block

Declares the package identity.

| Field     | Required | Default                       | Description                                    |
|-----------|----------|-------------------------------|------------------------------------------------|
| `name`    | No       | Directory name of the module  | Package name (must be a valid Pulumi name)     |
| `version` | No       | `"0.0.0-dev"`                 | Package version (must be valid semver)         |

## Resource Token

The component's resource token is formed as:

```
{package.name}:{component.module}:{component.name}
```

For the example above, the token would be `my-networking:index:VpcNetwork`.

## Submodules

A module package serves one component per consumable directory, following the
registry convention that both the root and `modules/<name>` directories are
consumed directly:

| Terraform directory                 | Pulumi component token    |
|-------------------------------------|---------------------------|
| the root, when it holds `.tf` files | `{package}:index:Module`  |
| `modules/<name>`                    | `{package}:<name>:Module` |

The root module names the package (its `package` and `component` blocks apply
as described above); submodules are always served as `Module` under a module
segment derived from their directory name, sanitized to lowercase
alphanumerics and hyphens (`modules/_user_data` becomes `user-data`,
`modules/a.b` becomes `a-b`). In generated Go SDKs, a module segment that is
not a valid Go package name maps to one (`user-data` becomes package
`userdata`, `2fa` becomes `_2fa`). A package whose root holds no `.tf` files at
all — common for registry packages such as `terraform-aws-modules/iam/aws` —
serves only its submodule components, named after its source (or directory)
and versioned by the resolved source version.

Each component resolves its providers independently, as the separate
configuration it is: sibling submodules may pin the same provider to
different versions, or use one local name for different providers. Local
packages are the exception — their components share the package's `sdks/`
descriptor pool, which holds one descriptor per provider name.

## Validation Rules

- `component.name` and `component.module` must be valid Pulumi names: one or more
  alphanumerics, hyphens, underscores, and periods. There is no restriction on the
  first character.
- `package.name` must be a valid Pulumi name when specified.
- `package.version` must be a valid [semver](https://semver.org/) string when specified.
- Only one `component` block and one `package` block are allowed per `terraform` block.
- `component` and `package` blocks are only valid in MLC modules. Using them in a
  regular Pulumi program (invoked via `pulumi up`) produces an error.

## Relationship to PulumiPlugin.yaml

The `PulumiPlugin.yaml` file tells the Pulumi engine how to run the component
provider. For HCL MLCs, name the plugin and specify the `hcl` runtime:

```yaml
name: my-networking
runtime: hcl
```

This file on its own is enough to make a module an MLC. Note that the package
name and version of the generated component come from the `package` block in the
HCL source, or from the module's directory name when that block is absent — not
from the `name` field here.

## Consuming an HCL component from other languages

A component's inputs are its `variable` blocks and its outputs are its `output`
blocks, but the names non-HCL consumers see are camelCased: a variable
`cidr_block` is `cidrBlock` in the generated TypeScript, Python, Go, and C# SDKs.

Every input is also echoed back as an output, so a consumer can read
`cidrBlock` off the component instance as well as pass it in. An input that is
not nullable is therefore always present on the result.
