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

// Local represents a single local value from a locals block.
//
// Terraform syntax:
//
//	locals {
//	  common_tags = {
//	    Environment = "dev"
//	    Project     = "example"
//	  }
//	  instance_count = 3
//	}
type Local struct {
	// Name is the local value name.
	Name string

	// Value is the expression that computes the local value.
	Value hcl.Expression

	// DeclRange is the source range of the local value assignment.
	DeclRange hcl.Range
}
