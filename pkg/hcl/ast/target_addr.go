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

package ast

import (
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/zclconf/go-cty/cty"

	"github.com/pulumi-labs/pulumi-hcl/pkg/hcl/modulepath"
)

// TargetAddr is a parsed `moved`, `import`, or `removed` block address:
// optional module-call steps followed by an optional resource. A
// whole-module-call address (e.g. `module.a`) has an empty Type.
type TargetAddr struct {
	// Modules holds the module-call steps, outermost first.
	Modules []modulepath.Step
	// Type is the resource type, or "" for a whole-module-call address.
	Type string
	// Name is the resource name.
	Name string
	// KeyIndex is the count instance key, if the address is keyed by count.
	KeyIndex *int
	// KeyEach is the for_each instance key, if the address is keyed by
	// for_each.
	KeyEach *string
}

// Keyed reports whether the address names a specific count/for_each instance.
func (a TargetAddr) Keyed() bool { return a.KeyIndex != nil || a.KeyEach != nil }

// String renders the address in Terraform form, e.g.
// module.a.simple_resource.b.
func (a TargetAddr) String() string {
	var parts []string
	for _, s := range a.Modules {
		parts = append(parts, "module", s.LogicalName())
	}
	if a.Type != "" {
		parts = append(parts, a.Type, a.Name)
	}
	return strings.Join(parts, ".")
}

// ParseTargetAddr decodes an address traversal into its module-call steps and
// (optional) resource. It returns false for a traversal it cannot model.
func ParseTargetAddr(t hcl.Traversal) (TargetAddr, bool) {
	var a TargetAddr
	head := func(step hcl.Traverser) (string, bool) {
		switch s := step.(type) {
		case hcl.TraverseRoot:
			return s.Name, true
		case hcl.TraverseAttr:
			return s.Name, true
		}
		return "", false
	}
	i := 0
	for i < len(t) {
		name, ok := head(t[i])
		if !ok || name != "module" {
			break
		}
		i++
		if i >= len(t) {
			return a, false
		}
		modName, ok := head(t[i])
		if !ok {
			return a, false
		}
		i++
		step := modulepath.NewStep(modName)
		if i < len(t) {
			if idx, ok := t[i].(hcl.TraverseIndex); ok {
				keyed, ok := moduleStepFor(modName, idx.Key)
				if !ok {
					return a, false
				}
				step = keyed
				i++
			}
		}
		a.Modules = append(a.Modules, step)
	}
	if i >= len(t) {
		// Whole-module-call address (no trailing resource).
		return a, len(a.Modules) > 0
	}
	typ, ok := head(t[i])
	if !ok {
		return a, false
	}
	i++
	if i >= len(t) {
		return a, false
	}
	name, ok := head(t[i])
	if !ok {
		return a, false
	}
	i++
	a.Type, a.Name = typ, name
	if i < len(t) {
		if idx, ok := t[i].(hcl.TraverseIndex); ok {
			a.KeyIndex, a.KeyEach = instanceKeyFromCty(idx.Key)
			i++
		}
	}
	return a, i == len(t)
}

// instanceKeyFromCty decodes an instance-key value into its typed components:
// a count index for a number, a for_each key for a string. Both are nil for
// any other type, which the caller treats as an unkeyed address.
func instanceKeyFromCty(v cty.Value) (index *int, eachKey *string) {
	switch v.Type() {
	case cty.Number:
		iv, _ := v.AsBigFloat().Int64()
		n := int(iv)
		return &n, nil
	case cty.String:
		s := v.AsString()
		return nil, &s
	default:
		return nil, nil
	}
}

// moduleStepFor builds a module-call path step for `module.<name>[<key>]`,
// decoding the count index or for_each key. It returns false for a key that
// is neither a non-negative integer nor a string.
func moduleStepFor(name string, key cty.Value) (modulepath.Step, bool) {
	switch key.Type() {
	case cty.Number:
		iv, _ := key.AsBigFloat().Int64()
		if iv < 0 {
			return modulepath.Step{}, false
		}
		return modulepath.NewIndexedStep(name, int(iv)), true
	case cty.String:
		return modulepath.NewKeyedStep(name, key.AsString()), true
	default:
		return modulepath.Step{}, false
	}
}
