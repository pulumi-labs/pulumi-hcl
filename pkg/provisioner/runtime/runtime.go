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

// Package runtime executes TF-compatible provisioners in-process. Designed
// to be called from a Pulumi hook callback with `self` already bound on
// hclCtx.
package runtime

import (
	"context"
	"fmt"

	"github.com/hashicorp/hcl/v2"
)

type Spec struct {
	Type   string // "local-exec" | "remote-exec" | "file"
	Config hcl.Body
	Conn   hcl.Body // nil for local-exec
}

func Run(ctx context.Context, spec *Spec, hclCtx *hcl.EvalContext) error {
	switch spec.Type {
	case "local-exec":
		return runLocalExec(ctx, spec, hclCtx)
	case "remote-exec":
		return runRemoteExec(ctx, spec, hclCtx)
	case "file":
		return runFile(ctx, spec, hclCtx)
	default:
		return fmt.Errorf("unsupported provisioner type: %q", spec.Type)
	}
}
