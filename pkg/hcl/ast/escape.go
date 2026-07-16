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

import "github.com/hashicorp/hcl/v2"

// EscapedBody is a block body merged with the contents of its `_` escaping
// block, whose arguments are always interpreted as block-type-specific even
// when their names collide with meta-arguments. The embedded Body carries the
// merge (hcl.MergeBodies), which hides the native syntax tree; Base and
// Escape keep the underlying bodies reachable for callers that walk
// *hclsyntax.Body directly (dependency scanning).
type EscapedBody struct {
	hcl.Body

	Base   hcl.Body // the block's own body, minus recognized meta-arguments
	Escape hcl.Body // the `_` block's body
}
