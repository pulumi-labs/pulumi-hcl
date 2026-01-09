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

// Output represents an output block in HCL.
//
// Terraform syntax:
//
//	output "instance_ip" {
//	  value       = aws_instance.example.public_ip
//	  description = "The public IP of the instance"
//	  sensitive   = true
//	  depends_on  = [aws_security_group.example]
//
//	  precondition {
//	    condition     = self.value != ""
//	    error_message = "IP must not be empty."
//	  }
//	}
type Output struct {
	// Name is the output name (the label on the output block).
	Name string

	// Value is the output value expression.
	Value hcl.Expression

	// Description is the output description, if specified.
	Description string

	// Sensitive indicates whether the output value should be hidden in logs.
	// In Pulumi, this maps to marking the output as a secret.
	Sensitive bool

	// DependsOn contains explicit dependencies for the output.
	DependsOn []hcl.Traversal

	// Preconditions contains precondition checks for the output.
	Preconditions []*CheckRule

	// DeclRange is the source range of the output block.
	DeclRange hcl.Range
}

// CheckRule represents a precondition or postcondition check.
type CheckRule struct {
	// Condition is the check condition expression.
	Condition hcl.Expression

	// ErrorMessage is the error message expression.
	ErrorMessage hcl.Expression

	// DeclRange is the source range of the check rule.
	DeclRange hcl.Range
}
