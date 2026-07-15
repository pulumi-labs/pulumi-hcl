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
	"github.com/zclconf/go-cty/cty/convert"

	"github.com/pulumi-labs/pulumi-hcl/pkg/hcl/eval"
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

	sensitive := configSensitive(content, hclCtx)

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
	if sensitive {
		fmt.Fprintln(os.Stderr, suppressedOutputMsg)
	}
	if quiet || sensitive {
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

// configSensitive reports whether any attribute of the provisioner's
// configuration evaluates to a value carrying the sensitive mark, in which
// case the provisioner's output must be suppressed so the value cannot leak
// through logging.
func configSensitive(content *hcl.BodyContent, hclCtx *hcl.EvalContext) bool {
	for _, attr := range content.Attributes {
		val, diags := attr.Expr.Value(hclCtx)
		if diags.HasErrors() {
			continue
		}
		_, marks := val.UnmarkDeep()
		if _, ok := marks[eval.SensitiveMark]; ok {
			return true
		}
	}
	return false
}

// The eval helpers deep-unmark every result: the engine tracks cross-resource
// dependencies and sensitivity as cty marks, and marked values panic in
// AsString and friends.

// evalAttr evaluates the named attribute and coerces the result to ty, the
// way schema-based decoding coerces provisioner config (number 5 and bool
// true become "5" and "true" under map(string)). Missing attributes and
// unknown values yield a null of ty.
func evalAttr(content *hcl.BodyContent, name string, ty cty.Type, hclCtx *hcl.EvalContext) (cty.Value, error) {
	attr, ok := content.Attributes[name]
	if !ok {
		return cty.NullVal(ty), nil
	}
	val, diags := attr.Expr.Value(hclCtx)
	if diags.HasErrors() {
		return cty.NullVal(ty), fmt.Errorf("evaluating %s: %s", name, diags.Error())
	}
	val, _ = val.UnmarkDeep()
	if !val.IsKnown() {
		return cty.NullVal(ty), nil
	}
	converted, err := convert.Convert(val, ty)
	if err != nil {
		return cty.NullVal(ty), fmt.Errorf("%s: %s", name, err)
	}
	return converted, nil
}

func evalString(content *hcl.BodyContent, name string, hclCtx *hcl.EvalContext) (string, error) {
	val, err := evalAttr(content, name, cty.String, hclCtx)
	if err != nil || val.IsNull() {
		return "", err
	}
	return val.AsString(), nil
}

func evalOptionalString(content *hcl.BodyContent, name string, hclCtx *hcl.EvalContext) (string, error) {
	return evalString(content, name, hclCtx)
}

func evalOptionalBool(content *hcl.BodyContent, name string, hclCtx *hcl.EvalContext) (bool, error) {
	val, err := evalAttr(content, name, cty.Bool, hclCtx)
	if err != nil || val.IsNull() {
		return false, err
	}
	return val.True(), nil
}

func evalStringSlice(content *hcl.BodyContent, name string, hclCtx *hcl.EvalContext) ([]string, error) {
	val, err := evalAttr(content, name, cty.List(cty.String), hclCtx)
	if err != nil || val.IsNull() {
		return nil, err
	}
	out := make([]string, 0, val.LengthInt())
	for it := val.ElementIterator(); it.Next(); {
		_, v := it.Element()
		if v.IsNull() {
			continue
		}
		out = append(out, v.AsString())
	}
	return out, nil
}

func evalStringMap(content *hcl.BodyContent, name string, hclCtx *hcl.EvalContext) (map[string]string, error) {
	val, err := evalAttr(content, name, cty.Map(cty.String), hclCtx)
	if err != nil || val.IsNull() {
		return nil, err
	}
	out := make(map[string]string, val.LengthInt())
	for it := val.ElementIterator(); it.Next(); {
		k, v := it.Element()
		if v.IsNull() {
			continue
		}
		out[k.AsString()] = v.AsString()
	}
	return out, nil
}
