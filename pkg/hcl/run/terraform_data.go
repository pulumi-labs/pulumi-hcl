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

package run

import (
	"github.com/pulumi/pulumi/pkg/v3/codegen/schema"
	"github.com/pulumi/pulumi/sdk/v3/go/property"
)

const (
	// terraformDataType is Terraform's builtin `terraform_data` managed
	// resource. Its provider (`terraform`) ships no installable plugin;
	// Terraform/OpenTofu implement it internally.
	terraformDataType = "terraform_data"
	// stashToken is the Pulumi engine's builtin resource we lower onto.
	stashToken = "pulumi:index:Stash"
)

// terraformDataSchema is the synthetic schema terraform_data resolves to, so it
// flows through the generic registration path like any other resource. It is
// backed by the engine's builtin Stash resource (hence the Stash token) but
// described in terraform_data's own terms; the lowerTerraformData* hooks bridge
// the two property surfaces around the Stash registration:
//
//   - `input` is the value Stash stores.
//   - `output` mirrors `input`. It is read back from Stash's `input` output,
//     which tracks the current input across updates (Stash's own `output`
//     property is frozen at create time and is not used).
//   - `triggers_replace` is declared so it is evaluated from config, but it is
//     not sent to Stash; lowerTerraformDataInputs moves it to the engine's
//     replacement trigger so a change to it forces replacement (and a new id).
//
// `id` is the Stash resource id and is attached by the registration machinery,
// so it is not declared here.
func terraformDataSchema() *schema.Resource {
	// All of terraform_data's attributes are optional/nullable, so the property
	// types are wrapped in OptionalType (an un-wrapped type reads as required).
	anyProp := func(name string) *schema.Property {
		return &schema.Property{Name: name, Type: &schema.OptionalType{ElementType: schema.AnyType}}
	}
	return &schema.Resource{
		Token:           stashToken,
		InputProperties: []*schema.Property{anyProp("input"), anyProp("triggers_replace")},
		Properties: []*schema.Property{
			anyProp("input"), anyProp("output"), anyProp("triggers_replace"),
		},
	}
}

// lowerTerraformDataInputs adapts evaluated terraform_data inputs to Stash's
// input surface just before registration. It is a no-op for every other type.
//
// triggers_replace is not a Stash input: a change to it must force replacement
// (yielding a new id), which the engine's replacement trigger does — while a
// plain input change is an in-place update with a stable id. Its dependencies
// are already folded into DependsOn, so the property-level entry is dropped too.
func lowerTerraformDataInputs(resType string, inputs property.Map, opts *ResourceOptions) property.Map {
	if resType != terraformDataType {
		return inputs
	}
	if t, ok := inputs.GetOk("triggers_replace"); ok {
		opts.ReplacementTrigger = t
		inputs = inputs.Delete("triggers_replace")
	}
	delete(opts.PropertyDependencies, "triggers_replace")
	return inputs
}

// lowerTerraformDataOutputs adapts Stash's outputs back to terraform_data's
// surface just after registration. It is a no-op for every other type.
//
// terraform_data's `output` mirrors `input`, read back from Stash's tracking
// `input` output; `triggers_replace` is not a Stash output, so it is echoed
// from the replacement trigger captured by lowerTerraformDataInputs.
func lowerTerraformDataOutputs(resType string, outputs property.Map, opts *ResourceOptions) property.Map {
	if resType != terraformDataType {
		return outputs
	}
	outputs = outputs.Set("output", outputs.Get("input"))
	return outputs.Set("triggers_replace", opts.ReplacementTrigger)
}
