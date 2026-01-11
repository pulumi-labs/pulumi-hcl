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
	"context"
	"fmt"
	"strings"

	"github.com/pulumi/pulumi-language-hcl/pkg/hcl/ast"
	"github.com/pulumi/pulumi-language-hcl/pkg/hcl/transform"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/zclconf/go-cty/cty"
)

// processProvisioners processes all provisioners for a resource.
// Provisioners run in order, with each depending on the previous one.
// Terraform provisioners map to the Pulumi Command provider:
//   - local-exec  -> command:local:Command
//   - remote-exec -> command:remote:Command
//   - file        -> command:remote:CopyToRemote
func (e *Engine) processProvisioners(
	ctx context.Context,
	res *ast.Resource,
	parentURN string,
	resourceOutputs cty.Value,
	resourceKey string,
) error {
	if len(res.Provisioners) == 0 {
		return nil
	}

	// Set self to the resource outputs so provisioners can reference self
	e.evaluator.Context().SetSelf(resourceOutputs)
	defer e.evaluator.Context().ClearSelf()

	// Get the resource-level connection config if present
	var resourceConn *ast.Connection
	if res.Connection != nil {
		resourceConn = res.Connection
	}

	// Track the previous provisioner URN for dependency chaining
	var prevProvisionerURN string

	for i, prov := range res.Provisioners {
		// Determine the connection to use (provisioner-level overrides resource-level)
		conn := resourceConn
		if prov.Connection != nil {
			conn = prov.Connection
		}

		// Build dependencies - each provisioner depends on the parent resource and the previous provisioner
		deps := []string{parentURN}
		if prevProvisionerURN != "" {
			deps = append(deps, prevProvisionerURN)
		}

		// Generate a unique name for this provisioner
		provName := fmt.Sprintf("%s-provisioner-%d", resourceKey, i)

		var urn string
		var err error

		switch prov.Type {
		case "local-exec":
			urn, err = e.registerLocalExecProvisioner(ctx, prov, provName, deps, parentURN)
		case "remote-exec":
			urn, err = e.registerRemoteExecProvisioner(ctx, prov, conn, provName, deps, parentURN)
		case "file":
			urn, err = e.registerFileProvisioner(ctx, prov, conn, provName, deps, parentURN)
		default:
			return fmt.Errorf("unsupported provisioner type: %s", prov.Type)
		}

		if err != nil {
			// Handle on_failure = "continue"
			if prov.OnFailure == "continue" {
				// Log warning but continue to next provisioner
				continue
			}
			return fmt.Errorf("provisioner %s failed: %w", prov.Type, err)
		}

		prevProvisionerURN = urn
	}

	return nil
}

// registerLocalExecProvisioner registers a local-exec provisioner as a command:local:Command resource.
func (e *Engine) registerLocalExecProvisioner(
	ctx context.Context,
	prov *ast.Provisioner,
	name string,
	deps []string,
	parentURN string,
) (string, error) {
	inputs := make(resource.PropertyMap)

	attrs, _ := prov.Config.JustAttributes()

	// Map command attribute
	if attr, ok := attrs["command"]; ok {
		val, diags := attr.Expr.Value(e.evaluator.Context().HCLContext())
		if diags.HasErrors() {
			return "", fmt.Errorf("evaluating command: %s", diags.Error())
		}
		if val.Type() == cty.String {
			// The Command provider uses "create" for the command to run on creation
			inputs["create"] = resource.NewStringProperty(val.AsString())
		}
	}

	// Map working_dir to dir
	if attr, ok := attrs["working_dir"]; ok {
		val, diags := attr.Expr.Value(e.evaluator.Context().HCLContext())
		if diags.HasErrors() {
			return "", fmt.Errorf("evaluating working_dir: %s", diags.Error())
		}
		if val.Type() == cty.String {
			inputs["dir"] = resource.NewStringProperty(val.AsString())
		}
	}

	// Map interpreter
	if attr, ok := attrs["interpreter"]; ok {
		val, diags := attr.Expr.Value(e.evaluator.Context().HCLContext())
		if diags.HasErrors() {
			return "", fmt.Errorf("evaluating interpreter: %s", diags.Error())
		}
		if val.CanIterateElements() {
			pv, err := transform.CtyToPropertyValue(val)
			if err != nil {
				return "", fmt.Errorf("converting interpreter: %w", err)
			}
			inputs["interpreter"] = pv
		}
	}

	// Map environment
	if attr, ok := attrs["environment"]; ok {
		val, diags := attr.Expr.Value(e.evaluator.Context().HCLContext())
		if diags.HasErrors() {
			return "", fmt.Errorf("evaluating environment: %s", diags.Error())
		}
		pv, err := transform.CtyToPropertyValue(val)
		if err != nil {
			return "", fmt.Errorf("converting environment: %w", err)
		}
		inputs["environment"] = pv
	}

	// Handle when = "destroy" - map to delete instead of create
	if prov.When == "destroy" {
		if createCmd, ok := inputs["create"]; ok {
			inputs["delete"] = createCmd
			delete(inputs, "create")
		}
	}

	opts := &ResourceOptions{
		DependsOn: deps,
		Parent:    parentURN,
	}

	urn, _, _, err := e.registerResource(ctx, "command:local:Command", name, inputs, opts)
	return urn, err
}

// registerRemoteExecProvisioner registers a remote-exec provisioner as a command:remote:Command resource.
func (e *Engine) registerRemoteExecProvisioner(
	ctx context.Context,
	prov *ast.Provisioner,
	conn *ast.Connection,
	name string,
	deps []string,
	parentURN string,
) (string, error) {
	inputs := make(resource.PropertyMap)

	attrs, _ := prov.Config.JustAttributes()

	// Build the command from inline, script, or scripts
	var command string

	if attr, ok := attrs["inline"]; ok {
		// inline is a list of commands to run
		val, diags := attr.Expr.Value(e.evaluator.Context().HCLContext())
		if diags.HasErrors() {
			return "", fmt.Errorf("evaluating inline: %s", diags.Error())
		}
		if val.CanIterateElements() {
			var commands []string
			it := val.ElementIterator()
			for it.Next() {
				_, v := it.Element()
				if v.Type() == cty.String {
					commands = append(commands, v.AsString())
				}
			}
			command = strings.Join(commands, "\n")
		}
	} else if attr, ok := attrs["script"]; ok {
		// script is a path to a script file
		val, diags := attr.Expr.Value(e.evaluator.Context().HCLContext())
		if diags.HasErrors() {
			return "", fmt.Errorf("evaluating script: %s", diags.Error())
		}
		if val.Type() == cty.String {
			// For remote-exec, we need to copy the script first, then execute it
			// For simplicity, we'll use the script content directly if readable
			command = fmt.Sprintf("sh %s", val.AsString())
		}
	} else if attr, ok := attrs["scripts"]; ok {
		// scripts is a list of script file paths
		val, diags := attr.Expr.Value(e.evaluator.Context().HCLContext())
		if diags.HasErrors() {
			return "", fmt.Errorf("evaluating scripts: %s", diags.Error())
		}
		if val.CanIterateElements() {
			var commands []string
			it := val.ElementIterator()
			for it.Next() {
				_, v := it.Element()
				if v.Type() == cty.String {
					commands = append(commands, fmt.Sprintf("sh %s", v.AsString()))
				}
			}
			command = strings.Join(commands, "\n")
		}
	}

	if command != "" {
		inputs["create"] = resource.NewStringProperty(command)
	}

	// Build connection configuration
	if conn != nil {
		connProp, err := e.buildConnectionProperty(conn)
		if err != nil {
			return "", fmt.Errorf("building connection: %w", err)
		}
		inputs["connection"] = connProp
	}

	// Handle when = "destroy"
	if prov.When == "destroy" {
		if createCmd, ok := inputs["create"]; ok {
			inputs["delete"] = createCmd
			delete(inputs, "create")
		}
	}

	opts := &ResourceOptions{
		DependsOn: deps,
		Parent:    parentURN,
	}

	urn, _, _, err := e.registerResource(ctx, "command:remote:Command", name, inputs, opts)
	return urn, err
}

// registerFileProvisioner registers a file provisioner as a command:remote:CopyToRemote resource.
func (e *Engine) registerFileProvisioner(
	ctx context.Context,
	prov *ast.Provisioner,
	conn *ast.Connection,
	name string,
	deps []string,
	parentURN string,
) (string, error) {
	inputs := make(resource.PropertyMap)

	attrs, _ := prov.Config.JustAttributes()

	// Map source or content
	if attr, ok := attrs["source"]; ok {
		val, diags := attr.Expr.Value(e.evaluator.Context().HCLContext())
		if diags.HasErrors() {
			return "", fmt.Errorf("evaluating source: %s", diags.Error())
		}
		if val.Type() == cty.String {
			inputs["localPath"] = resource.NewStringProperty(val.AsString())
		}
	} else if attr, ok := attrs["content"]; ok {
		val, diags := attr.Expr.Value(e.evaluator.Context().HCLContext())
		if diags.HasErrors() {
			return "", fmt.Errorf("evaluating content: %s", diags.Error())
		}
		if val.Type() == cty.String {
			// For content, we need to create a temp file or use stdin
			// The Command provider's CopyToRemote doesn't directly support content
			// We'll need to create an Asset from the content
			inputs["content"] = resource.NewStringProperty(val.AsString())
		}
	}

	// Map destination to remotePath
	if attr, ok := attrs["destination"]; ok {
		val, diags := attr.Expr.Value(e.evaluator.Context().HCLContext())
		if diags.HasErrors() {
			return "", fmt.Errorf("evaluating destination: %s", diags.Error())
		}
		if val.Type() == cty.String {
			inputs["remotePath"] = resource.NewStringProperty(val.AsString())
		}
	}

	// Build connection configuration
	if conn != nil {
		connProp, err := e.buildConnectionProperty(conn)
		if err != nil {
			return "", fmt.Errorf("building connection: %w", err)
		}
		inputs["connection"] = connProp
	}

	opts := &ResourceOptions{
		DependsOn: deps,
		Parent:    parentURN,
	}

	urn, _, _, err := e.registerResource(ctx, "command:remote:CopyToRemote", name, inputs, opts)
	return urn, err
}

// buildConnectionProperty builds a connection property map from an ast.Connection.
func (e *Engine) buildConnectionProperty(conn *ast.Connection) (resource.PropertyValue, error) {
	connMap := make(resource.PropertyMap)

	attrs, _ := conn.Config.JustAttributes()

	// Map connection attributes
	attrMappings := map[string]string{
		"host":        "host",
		"port":        "port",
		"user":        "user",
		"password":    "password",
		"private_key": "privateKey",
	}

	for tfAttr, pulumiAttr := range attrMappings {
		if attr, ok := attrs[tfAttr]; ok {
			val, diags := attr.Expr.Value(e.evaluator.Context().HCLContext())
			if diags.HasErrors() {
				return resource.PropertyValue{}, fmt.Errorf("evaluating %s: %s", tfAttr, diags.Error())
			}
			pv, err := transform.CtyToPropertyValue(val)
			if err != nil {
				return resource.PropertyValue{}, fmt.Errorf("converting %s: %w", tfAttr, err)
			}
			connMap[resource.PropertyKey(pulumiAttr)] = pv
		}
	}

	// Default port to 22 if not specified for SSH
	if _, ok := connMap["port"]; !ok && conn.Type == "ssh" {
		connMap["port"] = resource.NewNumberProperty(22)
	}

	return resource.NewObjectProperty(connMap), nil
}
