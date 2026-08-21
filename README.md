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
go install github.com/pulumi/pulumi-hcl/cmd/pulumi-resource-hcl@latest # for the provider
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

## Terraform Compatibility

This plugin supports the majority of Terraform's HCL syntax. For detailed compatibility information and known limitations, see [docs/terraform-compatibility.md](docs/terraform-compatibility.md).

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
