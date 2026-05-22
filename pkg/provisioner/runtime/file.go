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
	"os"
	"strings"

	"github.com/hashicorp/hcl/v2"

	"github.com/pulumi-labs/pulumi-hcl/vendored/communicator"
)

var fileSchema = &hcl.BodySchema{
	Attributes: []hcl.AttributeSchema{
		{Name: "source"},
		{Name: "content"},
		{Name: "destination", Required: true},
	},
}

func runFile(ctx context.Context, spec *Spec, hclCtx *hcl.EvalContext) error {
	content, diags := spec.Config.Content(fileSchema)
	if diags.HasErrors() {
		return fmt.Errorf("file: %s", diags.Error())
	}

	source, err := evalString(content, "source", hclCtx)
	if err != nil {
		return err
	}
	bodyContent, err := evalString(content, "content", hclCtx)
	if err != nil {
		return err
	}
	destination, err := evalString(content, "destination", hclCtx)
	if err != nil {
		return err
	}
	if destination == "" {
		return fmt.Errorf("file: destination must be non-empty")
	}
	if exactlyOneSet(source != "", bodyContent != "") != 1 {
		return fmt.Errorf("file: exactly one of source or content must be set")
	}

	connVal, err := evalConnection(spec.Conn, hclCtx)
	if err != nil {
		return err
	}
	comm, err := communicator.New(connVal)
	if err != nil {
		return fmt.Errorf("file: building communicator: %w", err)
	}

	uiOutput := stderrUIOutput{}
	if err := communicator.Retry(ctx, func() error { return comm.Connect(uiOutput) }); err != nil {
		return fmt.Errorf("file: connecting: %w", err)
	}
	defer func() { _ = comm.Disconnect() }()

	if bodyContent != "" {
		return comm.Upload(destination, strings.NewReader(bodyContent))
	}

	info, err := os.Stat(source)
	if err != nil {
		return fmt.Errorf("file: stat source: %w", err)
	}
	if info.IsDir() {
		return comm.UploadDir(destination, source)
	}
	f, err := os.Open(source)
	if err != nil {
		return fmt.Errorf("file: opening source: %w", err)
	}
	defer func() { _ = f.Close() }()
	return comm.Upload(destination, f)
}
