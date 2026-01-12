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

// Import represents an import block for importing existing resources.
//
// Terraform syntax:
//
//	import {
//	  to = aws_instance.web
//	  id = "i-1234567890abcdef0"
//	}
//
// This maps to Pulumi's ImportId resource option on the target resource.
type Import struct {
	// To is the target resource address.
	To hcl.Traversal

	// Id is the external resource ID to import.
	Id string

	// Provider is an optional provider reference for this import.
	Provider *ProviderRef

	// DeclRange is the source range of the import block.
	DeclRange hcl.Range
}
