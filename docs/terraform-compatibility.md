# Terraform HCL Compatibility

Pulumi HCL should be able to run all valid Terraform config programs without changes.

## CLI Reference

| Terraform           | Pulumi           |
|---------------------|------------------|
| `terraform plan`    | `pulumi preview` |
| `terraform apply`   | `pulumi up`      |
| `terraform destroy` | `pulumi destroy` |
| `terraform state`   | `pulumi state`   |
| `terraform import`  | `pulumi import`  |
| Workspaces          | Stacks           |

Terraform state files are not compatible. Import existing resources with `pulumi import`.

## Built-in Functions

Pulumi HCL supports nearly all of Terraform's built-in functions with identical behavior. The sections below document the exceptions.

### Functions in Terraform but not supported here

| Function          | Category        | Notes                                                                         |
|-------------------|-----------------|-------------------------------------------------------------------------------|
| `templatestring`  | String          | Renders an inline template string with a given context object.                |
| `plantimestamp`   | Date and Time   | Returns the timestamp at the start of a plan, which has no Pulumi equivalent. |
| `ephemeralasnull` | Type Conversion | Replaces ephemeral values with `null`; Pulumi has no ephemeral value concept. |

The `provider::terraform::*` provider functions and `terraform.applying` are Terraform-internal and have no equivalent here.

### Functions supported here but not in Terraform

| Function             | Category        | Notes                                                             |
|----------------------|-----------------|-------------------------------------------------------------------|
| `entries`            | Collection      | Converts a map or object to a list of `{key, value}` objects.     |
| `pulumiResourceName` | Pulumi-specific | Returns the Pulumi resource name for a resource reference.        |
| `pulumiResourceType` | Pulumi-specific | Returns the Pulumi resource type for a resource reference.        |
| `fileAsset`          | Asset/Archive   | Creates a Pulumi `FileAsset` from a local file path.              |
| `stringAsset`        | Asset/Archive   | Creates a Pulumi `StringAsset` from a string value.               |
| `remoteAsset`        | Asset/Archive   | Creates a Pulumi `RemoteAsset` from a URL.                        |
| `fileArchive`        | Asset/Archive   | Creates a Pulumi `FileArchive` from a local path.                 |
| `assetArchive`       | Asset/Archive   | Creates a Pulumi `AssetArchive` from a map of assets or archives. |

## Getting Help

- [GitHub repository](https://github.com/pulumi-labs/pulumi-hcl) for issues and source
- [Pulumi Community Slack](https://slack.pulumi.com/) for questions
