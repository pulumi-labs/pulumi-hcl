// Copyright 2026, Pulumi Corporation.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package parser implements HCL parsing for Terraform-compatible configurations.
package parser

import (
	"github.com/hashicorp/hcl/v2"
)

// rootSchema defines the top-level blocks allowed in an HCL configuration file.
var rootSchema = &hcl.BodySchema{
	Blocks: []hcl.BlockHeaderSchema{
		{Type: "terraform"},
		{Type: "pulumi"},
		{Type: "language"},
		{Type: "provider", LabelNames: []string{"name"}},
		{Type: "variable", LabelNames: []string{"name"}},
		{Type: "locals"},
		{Type: "resource", LabelNames: []string{"type", "name"}},
		{Type: "data", LabelNames: []string{"type", "name"}},
		{Type: "output", LabelNames: []string{"name"}},
		{Type: "module", LabelNames: []string{"name"}},
		{Type: "moved"},
		{Type: "removed"},
		{Type: "import"},
		{Type: "check", LabelNames: []string{"name"}},
		{Type: "call", LabelNames: []string{"resource", "method"}},
	},
}

// checkSchema defines the structure of a top-level check block.
var checkSchema = &hcl.BodySchema{
	Blocks: []hcl.BlockHeaderSchema{
		{Type: "assert"},
		{Type: "data", LabelNames: []string{"type", "name"}},
	},
}

// terraformSchema defines the structure of a terraform block.
//
// `required_version`, `experiments`, `backend`, and `provider_meta` are
// accepted for Terraform compatibility: the parser warns and ignores them
// rather than failing.
var terraformSchema = &hcl.BodySchema{
	Attributes: []hcl.AttributeSchema{
		{Name: "required_version_range"},
		{Name: "required_version"},
		{Name: "experiments"},
	},
	Blocks: []hcl.BlockHeaderSchema{
		{Type: "required_providers"},
		{Type: "component"},
		{Type: "package"},
		{Type: "backend", LabelNames: []string{"type"}},
		{Type: "provider_meta", LabelNames: []string{"provider"}},
	},
}

// pulumiBlockSchema defines the structure of a top-level pulumi block
// (Pulumi-specific program-level options).
var pulumiBlockSchema = &hcl.BodySchema{
	Attributes: []hcl.AttributeSchema{
		{Name: "terraform_provider_version"},
	},
}

// languageSchema defines the structure of a language block. `edition` and
// `experiments` are reserved: a module targeting a future language edition or
// an active experiment fails loudly instead of misbehaving.
var languageSchema = &hcl.BodySchema{
	Attributes: []hcl.AttributeSchema{
		{Name: "edition"},
		{Name: "experiments"},
	},
	Blocks: []hcl.BlockHeaderSchema{
		{Type: "compatible_with"},
	},
}

// languageCompatibleWithSchema lists only the argument Pulumi HCL interprets
// in a compatible_with block; decode with PartialContent so the rest is
// ignored.
var languageCompatibleWithSchema = &hcl.BodySchema{
	Attributes: []hcl.AttributeSchema{
		{Name: "pulumi"},
	},
}

// terraformComponentSchema defines the structure of a component sub-block.
var terraformComponentSchema = &hcl.BodySchema{
	Attributes: []hcl.AttributeSchema{
		{Name: "name", Required: true},
		{Name: "module"},
	},
}

// terraformPackageSchema defines the structure of a package sub-block.
var terraformPackageSchema = &hcl.BodySchema{
	Attributes: []hcl.AttributeSchema{
		{Name: "name"},
		{Name: "version"},
	},
}

// providerSchema defines the structure of a provider block. Only `alias` and
// `for_each` (Terraform-standard meta-arguments) live at the top level;
// Pulumi-specific options go in the nested `pulumi` block so they cannot
// collide with a provider's own configuration attributes.
var providerSchema = &hcl.BodySchema{
	Attributes: []hcl.AttributeSchema{
		{Name: "alias"},
		{Name: "for_each"},
	},
	Blocks: []hcl.BlockHeaderSchema{
		{Type: "pulumi"},
		{Type: "_"}, // meta-argument escaping block
	},
}

// pulumiProviderOptionsSchema defines the Pulumi-specific options allowed
// inside a provider block's nested `pulumi` block.
var pulumiProviderOptionsSchema = &hcl.BodySchema{
	Attributes: []hcl.AttributeSchema{
		{Name: "env_var_mappings"},
		{Name: "plugin_download_url"},
		{Name: "additional_secret_outputs"},
		{Name: "version"},
	},
}

// variableSchema defines the structure of a variable block.
var variableSchema = &hcl.BodySchema{
	Attributes: []hcl.AttributeSchema{
		{Name: "type"},
		{Name: "default"},
		{Name: "description"},
		{Name: "sensitive"},
		{Name: "nullable"},
		{Name: "ephemeral"},
	},
	Blocks: []hcl.BlockHeaderSchema{
		{Type: "validation"},
	},
}

// validationSchema defines the structure of a validation block.
var validationSchema = &hcl.BodySchema{
	Attributes: []hcl.AttributeSchema{
		{Name: "condition", Required: true},
		{Name: "error_message", Required: true},
	},
}

// outputSchema defines the structure of an output block.
var outputSchema = &hcl.BodySchema{
	Attributes: []hcl.AttributeSchema{
		{Name: "value", Required: true},
		{Name: "description"},
		{Name: "sensitive"},
		{Name: "ephemeral"},
		{Name: "depends_on"},
	},
	Blocks: []hcl.BlockHeaderSchema{
		{Type: "precondition"},
	},
}

// resourceSchema defines the structure of a resource block. Only the
// Terraform-standard meta-arguments live at the top level; Pulumi-specific
// options go in the nested `pulumi` block so they cannot collide with a
// resource's own provider-specific attributes (which may be named e.g.
// "version" or "parent").
//
// Note: The actual resource attributes are provider-specific and not validated here.
var resourceSchema = &hcl.BodySchema{
	Attributes: []hcl.AttributeSchema{
		{Name: "count"},
		{Name: "for_each"},
		{Name: "depends_on"},
		{Name: "provider"},
		{Name: "providers"},
	},
	Blocks: []hcl.BlockHeaderSchema{
		{Type: "pulumi"},
		{Type: "lifecycle"},
		{Type: "connection"},
		{Type: "provisioner", LabelNames: []string{"type"}},
		{Type: "timeouts"},
		{Type: "_"}, // meta-argument escaping block
	},
}

// dataBlockSchema defines the structure of a data block. Data sources are
// provider reads: the managed-resource blocks (connection, provisioner,
// timeouts) are listed only so the parser can reject them with a diagnostic
// rather than passing them through as data source attributes.
var dataBlockSchema = &hcl.BodySchema{
	Attributes: []hcl.AttributeSchema{
		{Name: "count"},
		{Name: "for_each"},
		{Name: "depends_on"},
		{Name: "provider"},
	},
	Blocks: []hcl.BlockHeaderSchema{
		{Type: "pulumi"},
		{Type: "lifecycle"},
		{Type: "connection"},
		{Type: "provisioner", LabelNames: []string{"type"}},
		{Type: "timeouts"},
		{Type: "_"}, // meta-argument escaping block
	},
}

// pulumiDataOptionsSchema defines the Pulumi-specific options allowed inside
// a data block's nested `pulumi` block: only the options the invoke path
// honors.
var pulumiDataOptionsSchema = &hcl.BodySchema{
	Attributes: []hcl.AttributeSchema{
		{Name: "parent"},
		{Name: "version"},
		{Name: "plugin_download_url"},
	},
}

// pulumiResourceOptionsSchema defines the Pulumi-specific options allowed
// inside a resource block's nested `pulumi` block.
var pulumiResourceOptionsSchema = &hcl.BodySchema{
	Attributes: []hcl.AttributeSchema{
		{Name: "name"},
		{Name: "additional_secret_outputs"},
		{Name: "parent"},
		{Name: "protect"},
		{Name: "retain_on_delete"},
		{Name: "deleted_with"},
		{Name: "replace_with"},
		{Name: "hide_diffs"},
		{Name: "replace_on_changes"},
		{Name: "import_id"},
		{Name: "env_var_mappings"},
		{Name: "version"},
		{Name: "plugin_download_url"},
		{Name: "aliases"},
	},
}

// lifecycleSchema defines the structure of a lifecycle block.
var lifecycleSchema = &hcl.BodySchema{
	Attributes: []hcl.AttributeSchema{
		{Name: "create_before_destroy"},
		{Name: "prevent_destroy"},
		{Name: "ignore_changes"},
		{Name: "replace_triggered_by"},
	},
	Blocks: []hcl.BlockHeaderSchema{
		{Type: "precondition"},
		{Type: "postcondition"},
	},
}

// connectionSchema defines the structure of a connection block.
var connectionSchema = &hcl.BodySchema{
	Attributes: []hcl.AttributeSchema{
		{Name: "type"},
		{Name: "host"},
		{Name: "port"},
		{Name: "user"},
		{Name: "password"},
		{Name: "private_key"},
		{Name: "certificate"},
		{Name: "agent"},
		{Name: "agent_identity"},
		{Name: "host_key"},
		{Name: "target_platform"},
		{Name: "timeout"},
		{Name: "bastion_host"},
		{Name: "bastion_host_key"},
		{Name: "bastion_port"},
		{Name: "bastion_user"},
		{Name: "bastion_password"},
		{Name: "bastion_private_key"},
		{Name: "bastion_certificate"},
		// WinRM specific
		{Name: "https"},
		{Name: "insecure"},
		{Name: "use_ntlm"},
		{Name: "cacert"},
	},
}

// provisionerSchema defines the structure of a provisioner block.
// Note: The actual provisioner-specific attributes (command, working_dir, inline, etc.)
// are intentionally NOT listed here so they remain in the Config body for dynamic evaluation.
var provisionerSchema = &hcl.BodySchema{
	Attributes: []hcl.AttributeSchema{
		{Name: "when"},
		{Name: "on_failure"},
	},
	Blocks: []hcl.BlockHeaderSchema{
		{Type: "connection"},
		{Type: "_"}, // meta-argument escaping block
	},
}

// moduleSchema defines the structure of a module block. The nested `pulumi`
// block holds Pulumi-specific options so they cannot collide with the
// module's own input variables.
var moduleSchema = &hcl.BodySchema{
	Attributes: []hcl.AttributeSchema{
		{Name: "source", Required: true},
		{Name: "version"},
		{Name: "count"},
		{Name: "for_each"},
		{Name: "depends_on"},
		{Name: "providers"},
	},
	Blocks: []hcl.BlockHeaderSchema{
		{Type: "pulumi"},
		{Type: "_"}, // meta-argument escaping block
	},
}

// pulumiModuleOptionsSchema defines the Pulumi-specific options allowed
// inside a module block's nested `pulumi` block.
var pulumiModuleOptionsSchema = &hcl.BodySchema{
	Attributes: []hcl.AttributeSchema{
		{Name: "name"},
		{Name: "protect"},
	},
}

// preconditionSchema defines the structure of a precondition/postcondition block.
var preconditionSchema = &hcl.BodySchema{
	Attributes: []hcl.AttributeSchema{
		{Name: "condition", Required: true},
		{Name: "error_message", Required: true},
	},
}

// timeoutsSchema defines the structure of a timeouts block.
var timeoutsSchema = &hcl.BodySchema{
	Attributes: []hcl.AttributeSchema{
		{Name: "create"},
		{Name: "read"},
		{Name: "update"},
		{Name: "delete"},
	},
}

// movedSchema defines the structure of a moved block.
var movedSchema = &hcl.BodySchema{
	Attributes: []hcl.AttributeSchema{
		{Name: "from", Required: true},
		{Name: "to", Required: true},
	},
}

// removedSchema defines the structure of a removed block.
var removedSchema = &hcl.BodySchema{
	Attributes: []hcl.AttributeSchema{
		{Name: "from", Required: true},
	},
	Blocks: []hcl.BlockHeaderSchema{
		{Type: "lifecycle"},
		{Type: "provisioner", LabelNames: []string{"type"}},
	},
}

// removedLifecycleSchema defines the structure of a removed block's lifecycle
// block.
var removedLifecycleSchema = &hcl.BodySchema{
	Attributes: []hcl.AttributeSchema{
		{Name: "destroy"},
	},
}

// importSchema defines the structure of an import block.
var importSchema = &hcl.BodySchema{
	Attributes: []hcl.AttributeSchema{
		{Name: "to", Required: true},
		{Name: "id", Required: true},
		{Name: "provider"},
		{Name: "for_each"},
	},
}
