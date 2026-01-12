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

// Moved represents a moved block for resource renaming.
//
// Terraform syntax:
//
//	moved {
//	  from = aws_instance.old_name
//	  to   = aws_instance.new_name
//	}
//
// This maps to Pulumi's aliases resource option on the target resource.
type Moved struct {
	// From is the source resource address (the old name).
	From hcl.Traversal

	// To is the target resource address (the new name).
	To hcl.Traversal

	// DeclRange is the source range of the moved block.
	DeclRange hcl.Range
}
