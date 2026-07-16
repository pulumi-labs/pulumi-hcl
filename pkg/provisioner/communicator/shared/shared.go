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

// Package shared replaces OpenTofu's internal/communicator/shared. We
// reimplement instead of vendoring upstream to avoid pulling in the
// internal/configs/configschema dependency closure. regen.sh rewrites the
// upstream import path to this package.
package shared

import (
	"fmt"
	"net"

	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/convert"
)

// Stands in for configschema.Block; the vendored ssh communicator only
// calls CoerceValue.
type connectionSchema struct {
	objectType cty.Type
}

func (s *connectionSchema) CoerceValue(in cty.Value) (cty.Value, error) {
	out, err := convert.Convert(in, s.objectType)
	if err != nil {
		return cty.NilVal, err
	}
	return out, nil
}

// Mirrors upstream's ConnectionBlockSupersetSchema.
var ConnectionBlockSupersetSchema = &connectionSchema{
	objectType: cty.ObjectWithOptionalAttrs(
		map[string]cty.Type{
			"host":                cty.String,
			"type":                cty.String,
			"user":                cty.String,
			"password":            cty.String,
			"port":                cty.Number,
			"timeout":             cty.String,
			"script_path":         cty.String,
			"target_platform":     cty.String,
			"private_key":         cty.String,
			"certificate":         cty.String,
			"host_key":            cty.String,
			"agent":               cty.Bool,
			"agent_identity":      cty.String,
			"proxy_scheme":        cty.String,
			"proxy_host":          cty.String,
			"proxy_port":          cty.Number,
			"proxy_user_name":     cty.String,
			"proxy_user_password": cty.String,
			"bastion_host":        cty.String,
			"bastion_host_key":    cty.String,
			"bastion_port":        cty.Number,
			"bastion_user":        cty.String,
			"bastion_password":    cty.String,
			"bastion_private_key": cty.String,
			"bastion_certificate": cty.String,
			// For type=winrm only (enforced in winrm communicator)
			"https":    cty.Bool,
			"insecure": cty.Bool,
			"cacert":   cty.String,
			"use_ntlm": cty.Bool,
		},
		// Every attribute except host is optional.
		[]string{
			"type", "user", "password", "port", "timeout", "script_path",
			"target_platform", "private_key", "certificate", "host_key",
			"agent", "agent_identity", "proxy_scheme", "proxy_host",
			"proxy_port", "proxy_user_name", "proxy_user_password",
			"bastion_host", "bastion_host_key", "bastion_port", "bastion_user",
			"bastion_password", "bastion_private_key", "bastion_certificate",
			"https", "insecure", "cacert", "use_ntlm",
		},
	),
}

// IpFormat brackets IPv6 addresses for "host:port" SSH strings.
func IpFormat(ip string) string {
	ipObj := net.ParseIP(ip)
	if ipObj == nil || ipObj.To4() != nil {
		return ip
	}
	return fmt.Sprintf("[%s]", ip)
}
