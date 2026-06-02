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
	"maps"
	"slices"
	"strings"

	"github.com/pulumi/pulumi/pkg/v3/codegen/schema"
	"github.com/pulumi/pulumi/sdk/v3/go/property"
)

const (
	remoteStateType      = "terraform_remote_state"
	localReferenceToken  = "terraform:state:getLocalReference"
	remoteReferenceToken = "terraform:state:getRemoteReference"

	// TerraformStatePackage / TerraformStatePackageVersion is the external
	// pulumi-terraform package that provides the state-reference invokes.
	TerraformStatePackage        = "terraform"
	TerraformStatePackageVersion = "6.0.2"
)

// terraformRemoteStateSchema is the synthetic schema terraform_remote_state
// resolves to: the TF data source surface as inputs. lowerRemoteStateInvoke
// selects the concrete pulumi-terraform invoke (local or remote) by backend and
// translates the inputs to its arguments. The placeholder Token is always
// overridden there.
func terraformRemoteStateSchema() *schema.Function {
	opt := func(t schema.Type) schema.Type { return &schema.OptionalType{ElementType: t} }
	return &schema.Function{
		Token: localReferenceToken,
		Inputs: &schema.ObjectType{
			Properties: []*schema.Property{
				{Name: "backend", Type: opt(schema.StringType)},
				{Name: "config", Type: opt(schema.AnyType)},
				{Name: "workspace", Type: opt(schema.StringType)},
				{Name: "defaults", Type: opt(schema.AnyType)},
			},
		},
		ReturnType: &schema.ObjectType{
			Properties: []*schema.Property{{Name: "outputs", Type: schema.AnyType}},
		},
	}
}

// lowerRemoteStateInvoke rewrites a terraform_remote_state invoke into the matching
// pulumi-terraform state-reference invoke: the local backend uses getLocalReference
// and the remote backend uses getRemoteReference. The pulumi-terraform package
// serves no other backend, so any other backend is rejected; support can be added
// as the package gains backends.
//
// On the remote backend the top-level `workspace` is resolved like OpenTofu's
// remote backend: with config.workspaces.prefix the read workspace is
// `<prefix><workspace>` (combined into the invoke's `workspaces.name`); with
// config.workspaces.name only the implicit "default" workspace exists, so
// `workspace` must be "default". It is unsupported on the local backend
// (getLocalReference has no workspace input). `defaults` is unsupported.
func lowerRemoteStateInvoke(tfType string, req InvokeRequest) (InvokeRequest, error) {
	if tfType != remoteStateType {
		return req, nil
	}

	if v, ok := req.Args.GetOk("defaults"); ok && !v.IsNull() {
		return req, fmt.Errorf("terraform_remote_state: %q is not supported", "defaults")
	}

	backend := ""
	if b, ok := req.Args.GetOk("backend"); ok && b.IsString() {
		backend = b.AsString()
	}

	workspace, hasWorkspace := "", false
	if w, ok := req.Args.GetOk("workspace"); ok && w.IsString() {
		workspace, hasWorkspace = w.AsString(), true
	}

	// fields maps the chosen reference's recognized config keys to their invoke
	// argument names; desc describes it for the error message.
	var token, desc string
	var fields map[string]string
	switch backend {
	case "local":
		if hasWorkspace {
			return req, fmt.Errorf("terraform_remote_state: the local backend does not support the workspace attribute")
		}
		token = localReferenceToken
		desc = "the local backend"
		fields = map[string]string{"path": "path", "workspace_dir": "workspaceDir"}
	case "remote":
		token = remoteReferenceToken
		desc = fmt.Sprintf("backend %q", backend)
		fields = map[string]string{
			"organization": "organization",
			"hostname":     "hostname",
			"token":        "token",
			"workspaces":   "workspaces",
		}
	default:
		return req, fmt.Errorf(
			"terraform_remote_state: backend %q is not supported (supported backends: local, remote)", backend,
		)
	}

	args := map[string]property.Value{}
	var unexpected []string
	if cfg, ok := req.Args.GetOk("config"); ok && cfg.IsMap() {
		for k, v := range cfg.AsMap().All {
			if argKey, ok := fields[k]; ok {
				args[argKey] = v
			} else {
				unexpected = append(unexpected, k)
			}
		}
	}
	if len(unexpected) > 0 {
		slices.Sort(unexpected)
		return req, fmt.Errorf(
			"terraform_remote_state: %s does not read config field(s) %v (supported: %s)",
			desc, unexpected, strings.Join(slices.Sorted(maps.Keys(fields)), ", "),
		)
	}

	// Resolve the top-level `workspace` the way OpenTofu's remote backend does:
	//   - config.workspaces.prefix + workspace -> read `<prefix><workspace>`
	//   - config.workspaces.name exposes only the implicit "default" workspace,
	//     so workspace must be "default" (or absent); anything else is an error
	if hasWorkspace {
		ws, ok := args["workspaces"]
		if !ok || !ws.IsMap() {
			return req, fmt.Errorf("terraform_remote_state: workspace requires config.workspaces.prefix")
		}
		switch wsMap := ws.AsMap(); {
		case wsMap.Get("name").IsString():
			if workspace != "default" {
				return req, fmt.Errorf(
					"terraform_remote_state: workspace %q is invalid with config.workspaces.name (only \"default\" is)", workspace,
				)
			}
		case wsMap.Get("prefix").IsString():
			args["workspaces"] = property.New(map[string]property.Value{"name": property.New(wsMap.Get("prefix").AsString() + workspace)})
		default:
			return req, fmt.Errorf("terraform_remote_state: workspace requires config.workspaces.prefix")
		}
	}

	req.Token = token
	req.Args = property.NewMap(args)
	return req, nil
}
