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
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"

	"github.com/hashicorp/hcl/v2"
	"github.com/zclconf/go-cty/cty"
)

// when and on_failure are stripped by the parser before this body reaches us.
var localExecSchema = &hcl.BodySchema{
	Attributes: []hcl.AttributeSchema{
		{Name: "command", Required: true},
		{Name: "working_dir"},
		{Name: "interpreter"},
		{Name: "environment"},
		{Name: "quiet"},
	},
}

func runLocalExec(ctx context.Context, spec *Spec, hclCtx *hcl.EvalContext) error {
	content, diags := spec.Config.Content(localExecSchema)
	if diags.HasErrors() {
		return fmt.Errorf("local-exec: %s", diags.Error())
	}

	command, err := evalString(content, "command", hclCtx)
	if err != nil {
		return err
	}
	if command == "" {
		return fmt.Errorf("local-exec: command must be non-empty")
	}

	workingDir, err := evalOptionalString(content, "working_dir", hclCtx)
	if err != nil {
		return err
	}

	interpreter, err := evalStringSlice(content, "interpreter", hclCtx)
	if err != nil {
		return err
	}
	if len(interpreter) == 0 {
		interpreter = defaultInterpreter()
	}

	environment, err := evalStringMap(content, "environment", hclCtx)
	if err != nil {
		return err
	}

	quiet, err := evalOptionalBool(content, "quiet", hclCtx)
	if err != nil {
		return err
	}

	// TF appends command as the final positional arg of interpreter,
	// e.g. ["/bin/sh", "-c", "echo hi"].
	args := append([]string{}, interpreter[1:]...)
	args = append(args, command)
	cmd := exec.CommandContext(ctx, interpreter[0], args...)
	cmd.Dir = workingDir
	if len(environment) > 0 {
		cmd.Env = os.Environ()
		for k, v := range environment {
			cmd.Env = append(cmd.Env, k+"="+v)
		}
	}
	if quiet {
		cmd.Stdout = io.Discard
		cmd.Stderr = io.Discard
	} else {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	}

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("local-exec %q: %w", command, err)
	}
	return nil
}

func defaultInterpreter() []string {
	if runtime.GOOS == "windows" {
		return []string{"cmd", "/C"}
	}
	return []string{"/bin/sh", "-c"}
}

func evalString(content *hcl.BodyContent, name string, hclCtx *hcl.EvalContext) (string, error) {
	attr, ok := content.Attributes[name]
	if !ok {
		return "", nil
	}
	val, diags := attr.Expr.Value(hclCtx)
	if diags.HasErrors() {
		return "", fmt.Errorf("evaluating %s: %s", name, diags.Error())
	}
	if !val.IsKnown() || val.IsNull() {
		return "", nil
	}
	if val.Type() != cty.String {
		return "", fmt.Errorf("%s must be a string, got %s", name, val.Type().FriendlyName())
	}
	return val.AsString(), nil
}

func evalOptionalString(content *hcl.BodyContent, name string, hclCtx *hcl.EvalContext) (string, error) {
	return evalString(content, name, hclCtx)
}

func evalOptionalBool(content *hcl.BodyContent, name string, hclCtx *hcl.EvalContext) (bool, error) {
	attr, ok := content.Attributes[name]
	if !ok {
		return false, nil
	}
	val, diags := attr.Expr.Value(hclCtx)
	if diags.HasErrors() {
		return false, fmt.Errorf("evaluating %s: %s", name, diags.Error())
	}
	if !val.IsKnown() || val.IsNull() {
		return false, nil
	}
	if val.Type() != cty.Bool {
		return false, fmt.Errorf("%s must be a bool, got %s", name, val.Type().FriendlyName())
	}
	return val.True(), nil
}

func evalStringSlice(content *hcl.BodyContent, name string, hclCtx *hcl.EvalContext) ([]string, error) {
	attr, ok := content.Attributes[name]
	if !ok {
		return nil, nil
	}
	val, diags := attr.Expr.Value(hclCtx)
	if diags.HasErrors() {
		return nil, fmt.Errorf("evaluating %s: %s", name, diags.Error())
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

func evalStringMap(content *hcl.BodyContent, name string, hclCtx *hcl.EvalContext) (map[string]string, error) {
	attr, ok := content.Attributes[name]
	if !ok {
		return nil, nil
	}
	val, diags := attr.Expr.Value(hclCtx)
	if diags.HasErrors() {
		return nil, fmt.Errorf("evaluating %s: %s", name, diags.Error())
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
