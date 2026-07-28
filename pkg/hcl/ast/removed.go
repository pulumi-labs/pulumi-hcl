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

// Removed represents a removed block: the resource or module at From has been
// deleted from the configuration and its remote objects should be destroyed.
//
// Terraform syntax:
//
//	removed {
//	  from = aws_instance.example
//	  lifecycle {
//	    destroy = true
//	  }
//	}
//
// Only destroy = true is supported: a resource absent from the program is
// destroyed by the Pulumi engine on its own, while destroy = false (forget)
// has no engine mapping and parses with an error diagnostic.
type Removed struct {
	// From is the address of the removed resource or module. It carries no
	// instance keys; a removed block applies to every instance.
	From TargetAddr

	// Destroy reports whether the block declares lifecycle { destroy = true }.
	// A false value (explicit, or from an omitted lifecycle block) is
	// unsupported and carries an error diagnostic.
	Destroy bool

	// Provisioners holds destroy-time provisioners to run when the removed
	// resource is deleted. Only when = destroy provisioners are allowed.
	Provisioners []*Provisioner

	// DeclRange is the source range of the removed block.
	DeclRange hcl.Range
}
