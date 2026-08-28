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

// DataSource represents a data block in HCL. A data source is a provider
// read: it holds no state, so the managed-resource surface — lifecycle
// arguments, provisioners, connections, timeouts, and most pulumi options —
// does not apply and is rejected at parse time.
//
// Terraform syntax:
//
//	data "aws_ami" "ubuntu" {
//	  most_recent = true
//	}
type DataSource struct {
	// Type is the data source type (e.g., "aws_ami").
	Type string

	// Name is the local name of the data source (e.g., "ubuntu").
	Name string

	// Config is the body containing data source attributes.
	// This excludes meta-arguments which are parsed separately.
	Config hcl.Body

	// Count is the count meta-argument expression, if present.
	Count hcl.Expression

	// ForEach is the for_each meta-argument expression, if present.
	ForEach hcl.Expression

	// DependsOn contains explicit dependencies from the depends_on meta-argument.
	DependsOn []hcl.Traversal

	// Provider is the raw expression from the `provider` attribute, if specified.
	// It is evaluated at runtime; the resulting value supplies the provider URN/ID.
	Provider hcl.Expression

	// ResourceParent is the parent resource reference, if specified. The read
	// itself registers nothing, but the reference orders it and is emitted by
	// PCL codegen for the invoke `parent` option.
	ResourceParent hcl.Traversal

	// Version is the version of the provider plugin to use for this data source.
	Version hcl.Expression

	// PluginDownloadURL is the URL from which the provider plugin should be downloaded.
	PluginDownloadURL hcl.Expression

	// Preconditions contains precondition checks (evaluated before the read).
	Preconditions []*CheckRule

	// Postconditions contains postcondition checks (evaluated after the read).
	Postconditions []*CheckRule

	// Timeouts contains timeout configuration, if present.
	Timeouts *Timeouts

	// DeclRange is the source range of the entire data block.
	DeclRange hcl.Range

	// TypeRange is the source range of the data source type.
	TypeRange hcl.Range
}
