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
	"github.com/hashicorp/hcl/v2/hcldec"
	"github.com/zclconf/go-cty/cty"

	"github.com/pulumi-labs/pulumi-hcl/pkg/provisioner/communicator/shared"
)

// evalConnection produces a cty object matching the SSH communicator's
// superset schema. The communicator factory requires every known attribute
// present (null for missing optionals); CoerceValue fills those in.
func evalConnection(conn hcl.Body, hclCtx *hcl.EvalContext) (cty.Value, error) {
	if conn == nil {
		return cty.NilVal, fmt.Errorf("connection block required for remote-exec / file provisioners")
	}
	val, diags := hcldec.Decode(conn, connectionDecodeSpec(), hclCtx)
	if diags.HasErrors() {
		return cty.NilVal, fmt.Errorf("evaluating connection block: %s", diags.Error())
	}
	// The communicator reads attributes with AsString and friends, which
	// panic on marked values; connection marks never suppress output.
	val, _ = val.UnmarkDeep()
	coerced, err := shared.ConnectionBlockSupersetSchema.CoerceValue(val)
	if err != nil {
		return cty.NilVal, fmt.Errorf("coercing connection block: %w", err)
	}
	return coerced, nil
}

func connectionDecodeSpec() hcldec.Spec {
	attrs := map[string]struct {
		ty       cty.Type
		required bool
	}{
		"host":                {cty.String, true},
		"type":                {cty.String, false},
		"user":                {cty.String, false},
		"password":            {cty.String, false},
		"port":                {cty.Number, false},
		"timeout":             {cty.String, false},
		"script_path":         {cty.String, false},
		"target_platform":     {cty.String, false},
		"private_key":         {cty.String, false},
		"certificate":         {cty.String, false},
		"host_key":            {cty.String, false},
		"agent":               {cty.Bool, false},
		"agent_identity":      {cty.String, false},
		"proxy_scheme":        {cty.String, false},
		"proxy_host":          {cty.String, false},
		"proxy_port":          {cty.Number, false},
		"proxy_user_name":     {cty.String, false},
		"proxy_user_password": {cty.String, false},
		"bastion_host":        {cty.String, false},
		"bastion_host_key":    {cty.String, false},
		"bastion_port":        {cty.Number, false},
		"bastion_user":        {cty.String, false},
		"bastion_password":    {cty.String, false},
		"bastion_private_key": {cty.String, false},
		"bastion_certificate": {cty.String, false},
		// WinRM-only attributes; the ssh communicator ignores them, but the
		// superset schema accepts them on any connection type.
		"https":    {cty.Bool, false},
		"insecure": {cty.Bool, false},
		"cacert":   {cty.String, false},
		"use_ntlm": {cty.Bool, false},
	}
	specs := hcldec.ObjectSpec{}
	for name, a := range attrs {
		specs[name] = &hcldec.AttrSpec{Name: name, Type: a.ty, Required: a.required}
	}
	return specs
}
