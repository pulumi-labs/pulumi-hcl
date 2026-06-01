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

	"github.com/pulumi/pulumi/pkg/v3/codegen/schema"
	"github.com/pulumi/pulumi/sdk/v3/go/property"
)

const (
	remoteStateType     = "terraform_remote_state"
	localReferenceToken = "terraform:state:getLocalReference"

	// TerraformStatePackage / TerraformStatePackageVersion is the external
	// pulumi-terraform package that provides the state-reference invokes.
	TerraformStatePackage        = "terraform"
	TerraformStatePackageVersion = "6.0.2"
)

// terraformRemoteStateSchema is the synthetic schema terraform_remote_state
// resolves to: the TF data source surface as inputs, backed by the
// getLocalReference token. lowerRemoteStateInvoke translates the inputs to the
// invoke's arguments.
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

// lowerRemoteStateInvoke rewrites a terraform_remote_state invoke into the
// pulumi-terraform getLocalReference invoke, mapping config.path and
// config.workspace_dir onto path/workspaceDir. Only the local backend is
// supported. It is a no-op for any other type.
func lowerRemoteStateInvoke(tfType string, req InvokeRequest) (InvokeRequest, error) {
	if tfType != remoteStateType {
		return req, nil
	}

	backend := ""
	if b, ok := req.Args.GetOk("backend"); ok && b.IsString() {
		backend = b.AsString()
	}
	if backend != "local" {
		return req, fmt.Errorf(
			"terraform_remote_state: only the %q backend is currently supported, got %q", "local", backend)
	}

	args := map[string]property.Value{}
	if cfg, ok := req.Args.GetOk("config"); ok && cfg.IsMap() {
		config := cfg.AsMap()
		if p, ok := config.GetOk("path"); ok {
			args["path"] = p
		}
		if w, ok := config.GetOk("workspace_dir"); ok {
			args["workspaceDir"] = w
		}
	}
	req.Args = property.NewMap(args)
	return req, nil
}
