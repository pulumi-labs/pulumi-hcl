# Multi-Language Components (MLCs) in HCL

Multi-Language Components allow you to author reusable Pulumi components in HCL
that can be consumed from any Pulumi language (TypeScript, Python, Go, C#, Java, YAML, or HCL).

## Declaring a Component Module

An HCL module becomes an MLC when it has a `PulumiPlugin.yaml` containing
`runtime: hcl`. The `component` and `package` blocks inside the `terraform {}`
block declare the component's identity and package metadata:

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
      version = ">= 6.0"
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

## Block Reference

### `component` block

Declares this module as a component resource. The block is optional; when it is
omitted, the component name defaults to `"Module"` and the module segment to
`"index"`. When the block is present, `name` is required.

| Field    | Required | Default    | Description                                  |
|----------|----------|------------|----------------------------------------------|
| `name`   | Yes      | `"Module"` | Component name (must be a valid Pulumi name) |
| `module` | No       | `"index"`  | Module segment of the resource token         |

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

## Validation Rules

- `component.name` and `component.module` must be valid Pulumi names (alphanumeric,
  hyphens, underscores; must start with a letter or underscore).
- `package.name` must be a valid Pulumi name when specified.
- `package.version` must be a valid [semver](https://semver.org/) string when specified.
- Only one `component` block and one `package` block are allowed per `terraform` block.
- `component` and `package` blocks are only valid in MLC modules. Using them in a
  regular Pulumi program (invoked via `pulumi up`) produces an error.

## Relationship to PulumiPlugin.yaml

The `PulumiPlugin.yaml` file tells the Pulumi engine how to run the component
provider. For HCL MLCs, it should specify the `hcl` runtime:

```yaml
runtime: hcl
```

The `component` and `package` blocks in the HCL source replace any need to
hardcode the provider name or version elsewhere.
