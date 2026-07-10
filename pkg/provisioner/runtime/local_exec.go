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

	ev := &evaluator{hclCtx: hclCtx}

	command, err := ev.evalString(content, "command")
	if err != nil {
		return err
	}
	if command == "" {
		return fmt.Errorf("local-exec: command must be non-empty")
	}

	workingDir, err := ev.evalString(content, "working_dir")
	if err != nil {
		return err
	}

	interpreter, err := ev.evalStringSlice(content, "interpreter")
	if err != nil {
		return err
	}
	if len(interpreter) == 0 {
		interpreter = defaultInterpreter()
	}

	environment, err := ev.evalStringMap(content, "environment")
	if err != nil {
		return err
	}

	quiet, err := ev.evalBool(content, "quiet")
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
	if ev.sensitive {
		fmt.Fprintln(os.Stderr, suppressedOutputMsg)
	}
	if quiet || ev.sensitive {
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
