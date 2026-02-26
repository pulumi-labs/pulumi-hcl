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

// Call represents a call block in HCL, which invokes a method on a resource.
//
// HCL syntax:
//
//	call "resourceName" "method_name" {
//	  # method args
//	}
type Call struct {
	// ResourceName is the logical name of the resource to call the method on.
	ResourceName string

	// MethodName is the snake_case name of the method to invoke.
	MethodName string

	// Config is the body containing method arguments.
	Config hcl.Body

	// DeclRange is the source range of the call block.
	DeclRange hcl.Range
}

// CallKey returns the key for a call in the Calls map.
func CallKey(resourceName, methodName string) string {
	return resourceName + "." + methodName
}
