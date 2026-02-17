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

// Pulumi represents a pulumi block with program-level options.
//
// Syntax:
//
//	pulumi {
//	  requiredVersionRange = ">=3.0.0"
//	}
type Pulumi struct {
	// RequiredVersionRange is the version range expression for the Pulumi CLI.
	RequiredVersionRange hcl.Expression

	// DeclRange is the source range of the pulumi block.
	DeclRange hcl.Range
}
