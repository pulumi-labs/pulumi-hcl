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
	"strings"

	"github.com/hashicorp/hcl/v2"

	"github.com/pulumi-labs/pulumi-hcl/pkg/provisioner/provisioners"
	"github.com/pulumi-labs/pulumi-hcl/vendored/communicator"
	"github.com/pulumi-labs/pulumi-hcl/vendored/communicator/remote"
)

var remoteExecSchema = &hcl.BodySchema{
	Attributes: []hcl.AttributeSchema{
		{Name: "inline"},
		{Name: "script"},
		{Name: "scripts"},
	},
}

func runRemoteExec(ctx context.Context, spec *Spec, hclCtx *hcl.EvalContext) error {
	content, diags := spec.Config.Content(remoteExecSchema)
	if diags.HasErrors() {
		return fmt.Errorf("remote-exec: %s", diags.Error())
	}

	ev := &evaluator{hclCtx: hclCtx}

	inline, err := ev.evalStringSlice(content, "inline")
	if err != nil {
		return err
	}
	script, err := ev.evalString(content, "script")
	if err != nil {
		return err
	}
	scripts, err := ev.evalStringSlice(content, "scripts")
	if err != nil {
		return err
	}
	if exactlyOneSet(len(inline) > 0, script != "", len(scripts) > 0) != 1 {
		return fmt.Errorf("remote-exec: exactly one of inline, script, or scripts must be set")
	}

	connVal, err := evalConnection(spec.Conn, hclCtx)
	if err != nil {
		return err
	}
	comm, err := communicator.New(connVal)
	if err != nil {
		return fmt.Errorf("remote-exec: building communicator: %w", err)
	}

	var uiOutput provisioners.UIOutput = stderrUIOutput{}
	stdout, stderr := io.Writer(os.Stdout), io.Writer(os.Stderr)
	if ev.sensitive {
		fmt.Fprintln(os.Stderr, suppressedOutputMsg)
		uiOutput = discardUIOutput{}
		stdout, stderr = io.Discard, io.Discard
	}
	if err := communicator.Retry(ctx, func() error { return comm.Connect(uiOutput) }); err != nil {
		return fmt.Errorf("remote-exec: connecting: %w", err)
	}
	defer func() { _ = comm.Disconnect() }()

	switch {
	case len(inline) > 0:
		return runInline(comm, inline, stdout, stderr)
	case script != "":
		return runScripts(comm, []string{script}, stdout, stderr)
	case len(scripts) > 0:
		return runScripts(comm, scripts, stdout, stderr)
	}
	return fmt.Errorf("remote-exec: no commands specified")
}

// TF joins inline lines into a single shell invocation.
func runInline(comm communicator.Communicator, inline []string, stdout, stderr io.Writer) error {
	return execRemote(comm, strings.Join(inline, "\n"), stdout, stderr)
}

// TF leaves uploaded scripts on the remote; we match that.
func runScripts(comm communicator.Communicator, paths []string, stdout, stderr io.Writer) error {
	for _, localPath := range paths {
		f, err := os.Open(localPath)
		if err != nil {
			return fmt.Errorf("remote-exec: opening script %q: %w", localPath, err)
		}
		remotePath := comm.ScriptPath()
		uploadErr := comm.UploadScript(remotePath, f)
		_ = f.Close()
		if uploadErr != nil {
			return fmt.Errorf("remote-exec: uploading %q to %q: %w", localPath, remotePath, uploadErr)
		}
		if err := execRemote(comm, remotePath, stdout, stderr); err != nil {
			return fmt.Errorf("remote-exec: %q: %w", localPath, err)
		}
	}
	return nil
}

func execRemote(comm communicator.Communicator, command string, stdout, stderr io.Writer) error {
	cmd := &remote.Cmd{
		Command: command,
		Stdout:  stdout,
		Stderr:  stderr,
	}
	cmd.Init()
	if err := comm.Start(cmd); err != nil {
		return fmt.Errorf("starting %q: %w", command, err)
	}
	if err := cmd.Wait(); err != nil {
		return err
	}
	return nil
}

func exactlyOneSet(bs ...bool) int {
	n := 0
	for _, b := range bs {
		if b {
			n++
		}
	}
	return n
}
