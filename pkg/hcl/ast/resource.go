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

package ast

import (
	"github.com/hashicorp/hcl/v2"
)

// Resource represents a resource or data block in HCL.
//
// Terraform syntax:
//
//	resource "aws_instance" "example" {
//	  ami           = "ami-0c55b159cbfafe1f0"
//	  instance_type = "t2.micro"
//
//	  lifecycle {
//	    create_before_destroy = true
//	  }
//	}
type Resource struct {
	// Type is the resource type (e.g., "aws_instance" or "aws:ec2/instance:Instance").
	Type string

	// Name is the local name of the resource (e.g., "example").
	Name string

	// Config is the body containing resource attributes.
	// This excludes meta-arguments which are parsed separately.
	Config hcl.Body

	// Count is the count meta-argument expression, if present.
	Count hcl.Expression

	// ForEach is the for_each meta-argument expression, if present.
	ForEach hcl.Expression

	// DependsOn contains explicit dependencies from the depends_on meta-argument.
	DependsOn []hcl.Traversal

	// Provider is the provider configuration reference, if specified.
	Provider *ProviderRef

	// Lifecycle contains lifecycle configuration, if present.
	Lifecycle *Lifecycle

	// Connection contains connection configuration for provisioners, if present.
	Connection *Connection

	// Provisioners contains provisioner blocks, in order.
	Provisioners []*Provisioner

	// DeclRange is the source range of the entire resource block.
	DeclRange hcl.Range

	// TypeRange is the source range of the resource type.
	TypeRange hcl.Range

	// IsDataSource indicates if this is a data source (data block) vs managed resource.
	IsDataSource bool
}

// ProviderRef is a reference to a provider configuration.
type ProviderRef struct {
	// Name is the provider local name (e.g., "aws").
	Name string

	// Alias is the provider alias, if specified (e.g., "west" in "aws.west").
	Alias string

	// Range is the source range of the provider reference.
	Range hcl.Range
}

// Lifecycle contains lifecycle configuration for a resource.
type Lifecycle struct {
	// CreateBeforeDestroy indicates whether to create the new resource before destroying the old one.
	CreateBeforeDestroy bool

	// PreventDestroy indicates whether destruction of the resource should be prevented.
	// In Pulumi, this maps to the "protect" resource option.
	PreventDestroy bool

	// IgnoreChanges lists the attributes whose changes should be ignored.
	IgnoreChanges []hcl.Traversal

	// IgnoreAllChanges indicates all attribute changes should be ignored.
	IgnoreAllChanges bool

	// ReplaceTriggeredBy lists expressions that trigger resource replacement.
	ReplaceTriggeredBy []hcl.Expression

	// Aliases contains resource aliases (Pulumi-specific).
	Aliases []string

	// DeclRange is the source range of the lifecycle block.
	DeclRange hcl.Range
}

// Connection contains connection configuration for remote provisioners.
type Connection struct {
	// Type is the connection type ("ssh" or "winrm").
	Type string

	// Config is the body containing connection attributes.
	Config hcl.Body

	// DeclRange is the source range of the connection block.
	DeclRange hcl.Range
}

// Provisioner represents a provisioner block within a resource.
type Provisioner struct {
	// Type is the provisioner type ("local-exec", "remote-exec", "file").
	Type string

	// Config is the body containing provisioner attributes.
	Config hcl.Body

	// Connection overrides the resource-level connection for this provisioner.
	Connection *Connection

	// When indicates when the provisioner runs ("create" or "destroy").
	When string

	// OnFailure indicates behavior on failure ("continue" or "fail").
	OnFailure string

	// DeclRange is the source range of the provisioner block.
	DeclRange hcl.Range
}
