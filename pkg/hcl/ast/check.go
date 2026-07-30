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

// Check represents a top-level check block.
//
// Terraform syntax:
//
//	check "health" {
//	  assert {
//	    condition     = data.http.example.status_code == 200
//	    error_message = "${data.http.example.url} returned an unhealthy status."
//	  }
//	}
//
// A check's assertions are non-blocking: a failed assertion reports a warning
// and the operation continues, unlike a resource precondition or postcondition.
type Check struct {
	// Name is the check name (the label on the check block).
	Name string

	// Asserts contains the assertions evaluated for this check.
	Asserts []*CheckRule

	// DataResource is the check's optional scoped data source, read fresh on
	// every operation and visible only to this check's assertions. A check may
	// declare at most one.
	DataResource *DataSource

	// DeclRange is the source range of the check block.
	DeclRange hcl.Range
}
