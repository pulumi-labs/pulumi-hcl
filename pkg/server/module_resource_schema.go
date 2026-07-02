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

package server

import (
	"maps"

	pulumischema "github.com/pulumi/pulumi/pkg/v3/codegen/schema"
)

// ModuleResourceToken is the token of the single, statically-known resource the
// fully dynamic provider serves.
const ModuleResourceToken = "hcl:index:Module"

const goImportBasePath = "github.com/pulumi-labs/pulumi-hcl/sdk/go/hcl"

// moduleResourceSchema returns the static schema for the fully dynamic provider.
func moduleResourceSchema(version string) pulumischema.PackageSpec {
	mapOfAny := func() pulumischema.TypeSpec {
		return pulumischema.TypeSpec{
			Type:                 "object",
			AdditionalProperties: &pulumischema.TypeSpec{Ref: "pulumi.json#/Any"},
		}
	}

	inputs := map[string]pulumischema.PropertySpec{
		"source": {
			// Plain: the module source must be known at Construct time so the
			// module (and the providers it references) can be resolved.
			TypeSpec:    pulumischema.TypeSpec{Type: "string", Plain: true},
			Description: "The module source address (a registry reference, URL, or local path).",
		},
		"version": {
			// Plain, like source: the version constraint must be known at Construct
			// time to resolve a registry module source.
			TypeSpec:    pulumischema.TypeSpec{Type: "string", Plain: true},
			Description: "An optional version constraint, used when source is a registry module reference.",
		},
		"inputs": {
			TypeSpec:    mapOfAny(),
			Description: "The module's input variables, keyed by variable name.",
		},
	}

	outputs := map[string]pulumischema.PropertySpec{
		"outputs": {
			TypeSpec:    mapOfAny(),
			Description: "The module's output values, keyed by output name.",
		},
	}

	// Inputs are also outputs
	maps.Copy(outputs, inputs)

	return pulumischema.PackageSpec{
		Name:              "hcl",
		DisplayName:       "Any HCL Module",
		Publisher:         "Pulumi",
		Version:           version,
		Description:       "Instantiate a Terraform/OpenTofu module as a Pulumi component.",
		Repository:        "https://github.com/pulumi-labs/pulumi-hcl",
		PluginDownloadURL: "github://api.github.com/pulumi-labs/pulumi-hcl",
		Meta:              &pulumischema.MetadataSpec{SupportPack: true},
		Language: map[string]pulumischema.RawMessage{
			"go": pulumischema.RawMessage(
				`{"importBasePath":"` + goImportBasePath + `","respectSchemaVersion":true}`),
		},
		Resources: map[string]pulumischema.ResourceSpec{
			ModuleResourceToken: {
				ObjectTypeSpec: pulumischema.ObjectTypeSpec{
					Type:        "object",
					Description: "A Terraform/OpenTofu module instantiated as a component resource.",
					Properties:  outputs,
					Required:    []string{"outputs"},
				},
				IsComponent:     true,
				InputProperties: inputs,
				RequiredInputs:  []string{"source"},
			},
		},
	}
}
