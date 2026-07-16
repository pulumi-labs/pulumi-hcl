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
	"fmt"
	"strings"

	"github.com/pulumi-labs/pulumi-hcl/pkg/hcl/eval"
	"github.com/pulumi-labs/pulumi-hcl/pkg/hcl/packages"
	"github.com/pulumi-labs/pulumi-hcl/pkg/hcl/transform"
	"github.com/pulumi/pulumi/sdk/v3/go/property"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/convert"
	ctyjson "github.com/zclconf/go-cty/cty/json"
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
	// An ignore_changes path into input (input["k"], input[0]) ignores the
	// whole attribute: input is a single dynamic attribute, and its keys live
	// under the {type, value} wrapper in the engine's encoding, where a nested
	// glob would match nothing.
	for i, g := range opts.IgnoreChanges {
		text, err := g.MarshalText()
		if err != nil {
			continue
		}
		if s := string(text); strings.HasPrefix(s, "input.") || strings.HasPrefix(s, "input[") {
			opts.IgnoreChanges[i] = property.GlobFromSegments(property.NewSegment("input"))
		}
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

// terraform_data's attributes are dynamically typed, and the Pulumi property
// encoding cannot represent every cty type: a set flattens to an ordered array
// that would re-expand as a tuple. Each evaluated input is therefore boxed as
// a {type, value} wrapper object before registration — persisting the cty type
// through the engine and its state alongside the value, the way OpenTofu
// stores dynamic attributes — and unboxed wherever outputs re-expand to cty,
// including from prior state (e.g. a destroy-time provisioner's self).
const (
	tdataTypeKey  = "type"
	tdataValueKey = "value"
)

// wrapTerraformDataInputs boxes terraform_data's evaluated inputs as
// {type, value} wrappers. It is a no-op for every other type.
func wrapTerraformDataInputs(resType string, inputs property.Map, evaluated map[string]cty.Value) property.Map {
	if resType != packages.TerraformDataType {
		return inputs
	}
	for _, name := range []string{"input", "triggers_replace"} {
		v, ok := inputs.GetOk(name)
		if !ok {
			continue
		}
		src, ok := evaluated[name]
		if !ok {
			continue
		}
		typeJSON, err := ctyjson.MarshalType(src.Type())
		if err != nil {
			continue
		}
		inputs = inputs.Set(name, property.New(map[string]property.Value{
			tdataTypeKey:  property.New(string(typeJSON)),
			tdataValueKey: v,
		}))
	}
	return inputs
}

// unwrapTerraformDataOutputs replaces the re-expanded cty values of
// terraform_data's dynamically typed attributes with their wrapper contents,
// read from the pre-expansion property outputs — where the wrapper's shape is
// exact, unlike its re-expanded form (whose cty shape depends on whether the
// two fields' types unify). It is a no-op for every other type.
func unwrapTerraformDataOutputs(resType string, outputs map[string]cty.Value, props property.Map) error {
	if resType != packages.TerraformDataType {
		return nil
	}
	for _, name := range []string{"input", "output", "triggers_replace"} {
		v, ok, err := unwrapTerraformDataValue(props.Get(name))
		if err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		if ok {
			outputs[name] = v
		}
	}
	return nil
}

// unwrapTerraformDataValue unboxes one property-encoded {type, value} wrapper
// into the cty value of its recorded type. It reports false for the values at
// these positions that are legitimately not wrappers — the null and unknown
// placeholders of an absent or not-yet-known attribute — which re-expand
// correctly without help. Any map here is a wrapper (evaluated inputs are
// always wrapped), so one that fails to decode is corrupt state and an error.
func unwrapTerraformDataValue(pv property.Value) (cty.Value, bool, error) {
	if !pv.IsMap() {
		return cty.Value{}, false, nil
	}
	m := pv.AsMap()
	typeJSON, typeOk := m.GetOk(tdataTypeKey)
	value, valueOk := m.GetOk(tdataValueKey)
	if !typeOk || !valueOk || m.Len() != 2 || !typeJSON.IsString() {
		return cty.Value{}, false, fmt.Errorf("malformed {%s, %s} wrapper", tdataTypeKey, tdataValueKey)
	}
	recorded, err := ctyjson.UnmarshalType([]byte(typeJSON.AsString()))
	if err != nil {
		return cty.Value{}, false, fmt.Errorf("invalid recorded cty type %q: %w", typeJSON.AsString(), err)
	}
	inner := transform.PropertyValueToCty(value)
	if converted, err := convert.Convert(inner, recorded); err == nil {
		inner = converted
	}
	if pv.Secret() {
		inner = inner.Mark(eval.SensitiveMark)
	}
	return inner, true, nil
}
