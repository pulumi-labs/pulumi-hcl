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
	"github.com/zclconf/go-cty/cty"
)

// Variable represents a variable block in HCL.
//
// Terraform syntax:
//
//	variable "instance_type" {
//	  type        = string
//	  default     = "t2.micro"
//	  description = "The EC2 instance type"
//	  sensitive   = false
//	  nullable    = true
//
//	  validation {
//	    condition     = length(var.instance_type) > 0
//	    error_message = "Instance type cannot be empty."
//	  }
//	}
type Variable struct {
	// Name is the variable name (the label on the variable block).
	Name string

	// Type is the type constraint expression, if specified.
	// When evaluated, this produces a cty.Type.
	Type hcl.Expression

	// TypeConstraint is the parsed type constraint, if Type was specified.
	TypeConstraint cty.Type

	// Default is the default value expression, if specified.
	Default hcl.Expression

	// Description is the variable description, if specified.
	Description string

	// Sensitive indicates whether the variable value should be hidden in logs.
	Sensitive bool

	// Nullable indicates whether the variable can be null (default true).
	Nullable bool

	// Validations contains validation rules for the variable.
	Validations []*Validation

	// DeclRange is the source range of the variable block.
	DeclRange hcl.Range
}

// Validation represents a validation block within a variable.
type Validation struct {
	// Condition is the validation condition expression.
	// Must evaluate to true for the value to be valid.
	Condition hcl.Expression

	// ErrorMessage is the error message to display when validation fails.
	ErrorMessage hcl.Expression

	// DeclRange is the source range of the validation block.
	DeclRange hcl.Range
}
