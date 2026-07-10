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

package runtime

import (
	"fmt"

	"github.com/hashicorp/hcl/v2"
	"github.com/zclconf/go-cty/cty"

	"github.com/pulumi-labs/pulumi-hcl/pkg/hcl/eval"
)

// evaluator evaluates provisioner configuration attributes. The engine
// tracks cross-resource dependencies and sensitivity as cty marks, and
// marked values panic in AsString and friends, so every result is
// deep-unmarked here. Sensitive marks are remembered so the provisioner can
// suppress output that could leak the value.
type evaluator struct {
	hclCtx    *hcl.EvalContext
	sensitive bool
}

// attr evaluates the named attribute, returning ok=false when it is absent.
func (e *evaluator) attr(content *hcl.BodyContent, name string) (cty.Value, bool, error) {
	attr, ok := content.Attributes[name]
	if !ok {
		return cty.NilVal, false, nil
	}
	val, diags := attr.Expr.Value(e.hclCtx)
	if diags.HasErrors() {
		return cty.NilVal, false, fmt.Errorf("evaluating %s: %s", name, diags.Error())
	}
	val, marks := val.UnmarkDeep()
	if _, isSensitive := marks[eval.SensitiveMark]; isSensitive {
		e.sensitive = true
	}
	return val, true, nil
}

func (e *evaluator) evalString(content *hcl.BodyContent, name string) (string, error) {
	val, ok, err := e.attr(content, name)
	if err != nil || !ok {
		return "", err
	}
	if !val.IsKnown() || val.IsNull() {
		return "", nil
	}
	if val.Type() != cty.String {
		return "", fmt.Errorf("%s must be a string, got %s", name, val.Type().FriendlyName())
	}
	return val.AsString(), nil
}

func (e *evaluator) evalBool(content *hcl.BodyContent, name string) (bool, error) {
	val, ok, err := e.attr(content, name)
	if err != nil || !ok {
		return false, err
	}
	if !val.IsKnown() || val.IsNull() {
		return false, nil
	}
	if val.Type() != cty.Bool {
		return false, fmt.Errorf("%s must be a bool, got %s", name, val.Type().FriendlyName())
	}
	return val.True(), nil
}

func (e *evaluator) evalStringSlice(content *hcl.BodyContent, name string) ([]string, error) {
	val, ok, err := e.attr(content, name)
	if err != nil || !ok {
		return nil, err
	}
	if !val.IsKnown() || val.IsNull() {
		return nil, nil
	}
	if !val.CanIterateElements() {
		return nil, fmt.Errorf("%s must be a list of strings", name)
	}
	out := make([]string, 0, val.LengthInt())
	it := val.ElementIterator()
	for it.Next() {
		_, v := it.Element()
		if v.Type() != cty.String {
			return nil, fmt.Errorf("%s elements must be strings", name)
		}
		out = append(out, v.AsString())
	}
	return out, nil
}

func (e *evaluator) evalStringMap(content *hcl.BodyContent, name string) (map[string]string, error) {
	val, ok, err := e.attr(content, name)
	if err != nil || !ok {
		return nil, err
	}
	if !val.IsKnown() || val.IsNull() {
		return nil, nil
	}
	if !val.CanIterateElements() {
		return nil, fmt.Errorf("%s must be a map of strings", name)
	}
	out := make(map[string]string, val.LengthInt())
	it := val.ElementIterator()
	for it.Next() {
		k, v := it.Element()
		if k.Type() != cty.String || v.Type() != cty.String {
			return nil, fmt.Errorf("%s keys and values must be strings", name)
		}
		out[k.AsString()] = v.AsString()
	}
	return out, nil
}
