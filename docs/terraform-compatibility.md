# Terraform HCL Compatibility

Pulumi HCL should be able to run all valid Terraform config programs without changes.

## Known Limitations

A small number of Terraform features are not modeled:

- **`backend`, `required_version`, `provider_meta`, and `experiments`** in the `terraform` block are accepted but ignored with a warning. Pulumi manages state independently and tracks its own version constraints via `required_version_range` or a `language` block's `compatible_with { pulumi = ... }` argument; language experiments have no Pulumi HCL equivalent.
- **`cloud`** in the `terraform` block is not accepted at all. Unlike the arguments above it is not part of the `terraform` block's schema, so a `cloud` block is a parse error rather than a warning. Remove it — Pulumi's own backend configuration lives outside the program.
- **WinRM `connection` blocks** are not supported — `connection` accepts `type = "ssh"` only.
- **`List<Object>` empty vs null** — HCL block syntax cannot distinguish an empty `List<Object>` from a null one, a known incompatibility with some Pulumi programs.
- **Resource-wide destroy ordering of late-created instances** — Terraform rebuilds destroy-time dependencies from configuration, so every instance of a `count`/`for_each` resource waits for a consumer's delete even when the consumer referenced only one instance (`depends_on = [a["x"]]`). Pulumi records each resource's dependencies once, when it is created, and cannot depend on an instance that registers later. A sibling instance created *after* the consumer is therefore not held back by it during destroy and may be deleted first.
- **`ignore_changes` on `terraform_data`'s `triggers_replace`** is honored only when it is present from the resource's creation. Adding `ignore_changes = [triggers_replace]` in the same update that changes `triggers_replace` (on a resource first created without it) still forces one replacement: `triggers_replace` is carried as a replacement trigger rather than a stored input, so it cannot be reconciled against the prior state the way an ignored input is.

State files are not interchangeable — Pulumi cannot read a Terraform state file as its own — but you do not have to import resources one at a time. Point the converter at an existing Terraform or OpenTofu state file to import everything it describes in bulk:

```bash
pulumi import --from hcl terraform.tfstate
```

This reads the state file, maps each Terraform resource type to its Pulumi token, and imports the resources into your stack. Only root-module resources are imported: resources nested inside a `module` block are skipped with a warning, as are `terraform_data` resources and any resource with no `id` in state. Anything that cannot be imported is reported as a warning rather than aborting the run, so the importable remainder still lands.

By default `create_before_destroy` matches Terraform's delete-first replacement order.

## CLI Reference

| Terraform           | Pulumi           |
|---------------------|------------------|
| `terraform plan`    | `pulumi preview` |
| `terraform apply`   | `pulumi up`      |
| `terraform destroy` | `pulumi destroy` |
| `terraform state`   | `pulumi state`   |
| `terraform import`  | `pulumi import`  |
| Workspaces          | Stacks           |

## Built-in Functions

Pulumi HCL supports nearly all of Terraform's built-in functions with identical behavior. The sections below document the exceptions.

### Functions in Terraform but not supported here

Every Terraform built-in function is supported, including the `provider::terraform::*` provider functions
(`decode_tfvars`, `encode_expr`, `encode_tfvars`). The only exception is the `terraform.applying` symbol,
which is Terraform-internal and has no equivalent here.

### Functions supported here but not in Terraform

| Function             | Category        | Notes                                                             |
|----------------------|-----------------|-------------------------------------------------------------------|
| `entries`            | Collection      | Converts a map or object to a list of `{key, value}` objects.     |
| `pulumiresourcename` | Pulumi-specific | Returns the Pulumi resource name for a resource reference.        |
| `pulumiresourcetype` | Pulumi-specific | Returns the Pulumi resource type for a resource reference.        |
| `pulumiresourceurn`  | Pulumi-specific | Returns the Pulumi URN for a resource reference.                  |
| `fileasset`          | Asset/Archive   | Creates a Pulumi `FileAsset` from a local file path.              |
| `stringasset`        | Asset/Archive   | Creates a Pulumi `StringAsset` from a string value.               |
| `remoteasset`        | Asset/Archive   | Creates a Pulumi `RemoteAsset` from a URL.                        |
| `filearchive`        | Asset/Archive   | Creates a Pulumi `FileArchive` from a local path.                 |
| `remotearchive`      | Asset/Archive   | Creates a Pulumi `RemoteArchive` from a URL.                      |
| `assetarchive`       | Asset/Archive   | Creates a Pulumi `AssetArchive` from a map of assets or archives. |

## Getting Help

- [GitHub repository](https://github.com/pulumi/pulumi-hcl) for issues and source
- [Pulumi Community Slack](https://slack.pulumi.com/) for questions
