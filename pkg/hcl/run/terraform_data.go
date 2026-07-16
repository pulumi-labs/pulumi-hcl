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
	"github.com/pulumi-labs/pulumi-hcl/pkg/hcl/packages"
	"github.com/pulumi/pulumi/sdk/v3/go/property"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/convert"
)

// lowerTerraformDataInputs adapts evaluated terraform_data inputs to Stash's
// input surface just before registration. It is a no-op for every other type.
//
// triggers_replace is not a Stash input: a change to it must force replacement
// (yielding a new id), which the engine's replacement trigger does — while a
// plain input change is an in-place update with a stable id. Its dependencies
// are already folded into DependsOn, so the property-level entry is dropped too.
//
// The engine treats a null trigger on either side of a diff as "no trigger",
// so a bare triggers_replace value could never diff against an absent one:
// setting or clearing triggers_replace would go unnoticed. terraform_data
// therefore always registers a non-null trigger, the fixed 2-tuple
// [triggers_replace, replace_triggered_by trigger] (each null when absent),
// which also keeps triggers_replace from clobbering a replace_triggered_by
// trigger already captured on the options.
//
// input is optional in terraform_data but required by Stash, so an omitted input
// is supplied as an explicit null: Stash stores it and mirrors it back as a null
// output, matching terraform_data used purely for its triggers_replace lifecycle.
func lowerTerraformDataInputs(resType string, inputs property.Map, opts *ResourceOptions) property.Map {
	if resType != packages.TerraformDataType {
		return inputs
	}
	triggers := inputs.Get("triggers_replace")
	inputs = inputs.Delete("triggers_replace")
	delete(opts.PropertyDependencies, "triggers_replace")
	opts.ReplacementTrigger = property.New([]property.Value{triggers, opts.ReplacementTrigger})
	if _, ok := inputs.GetOk("input"); !ok {
		inputs = inputs.Set("input", property.New(property.Null))
	}
	return inputs
}

// lowerTerraformDataOutputs adapts Stash's outputs back to terraform_data's
// surface just after registration. It is a no-op for every other type.
//
// terraform_data's `output` mirrors `input`, read back from Stash's tracking
// `input` output; `triggers_replace` is not a Stash output, so it is echoed
// from the trigger tuple encoded by lowerTerraformDataInputs.
func lowerTerraformDataOutputs(resType string, outputs property.Map, opts *ResourceOptions) property.Map {
	if resType != packages.TerraformDataType {
		return outputs
	}
	outputs = outputs.Set("output", outputs.Get("input"))
	return outputs.Set("triggers_replace", opts.ReplacementTrigger.AsArray().Get(0))
}

// restoreTerraformDataOutputTypes restores the cty types of terraform_data's
// dynamically typed attributes after the Pulumi property round-trip, which
// flattens a cty set to an ordered array that would otherwise re-expand as a
// tuple. `input` and `triggers_replace` echo the program's evaluated values
// and `output` mirrors `input`, so each re-expanded value converts back to the
// corresponding evaluated type. A value that no longer converts, or an
// evaluation with no known type, is left as re-expanded.
func restoreTerraformDataOutputTypes(resType string, outputs, evaluated map[string]cty.Value) {
	if resType != packages.TerraformDataType {
		return
	}
	for outName, inName := range map[string]string{
		"input":            "input",
		"output":           "input",
		"triggers_replace": "triggers_replace",
	} {
		src, ok := evaluated[inName]
		if !ok || src.Type() == cty.DynamicPseudoType {
			continue
		}
		v, ok := outputs[outName]
		if !ok {
			continue
		}
		if converted, err := convert.Convert(v, src.Type()); err == nil {
			outputs[outName] = converted
		}
	}
}
