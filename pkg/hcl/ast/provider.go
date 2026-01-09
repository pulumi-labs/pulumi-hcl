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

// Provider represents a provider block in HCL.
//
// Terraform syntax:
//
//	provider "aws" {
//	  region = "us-west-2"
//	  alias  = "west"
//	}
type Provider struct {
	// Name is the provider name (e.g., "aws").
	Name string

	// Alias is the provider alias, if specified.
	// This allows multiple configurations of the same provider.
	Alias string

	// Config is the body containing provider configuration attributes.
	Config hcl.Body

	// DeclRange is the source range of the provider block.
	DeclRange hcl.Range
}

// Key returns the key used to store this provider in the Config.Providers map.
// For providers without an alias, this is just the name.
// For providers with an alias, this is "name.alias".
func (p *Provider) Key() string {
	if p.Alias != "" {
		return p.Name + "." + p.Alias
	}
	return p.Name
}
