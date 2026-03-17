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
//	  component {
//	    name   = "MyComponent"
//	    module = "index"
//	  }
//	  package {
//	    name    = "my-package"
//	    version = "1.0.0"
//	  }
//	}
type Pulumi struct {
	// RequiredVersionRange is the version range expression for the Pulumi CLI.
	RequiredVersionRange hcl.Expression

	// Component declares this module as a multi-language component.
	Component *ComponentBlock

	// Package declares the package identity for an MLC module.
	Package *PackageBlock

	// DeclRange is the source range of the pulumi block.
	DeclRange hcl.Range
}

// ComponentBlock declares a component within a pulumi block.
type ComponentBlock struct {
	// Name is the component name (required). Must be a valid Pulumi name.
	Name string
	// Module is the module segment of the resource token. Defaults to "index".
	Module string
	// DeclRange is the source range of this block.
	DeclRange hcl.Range
}

// PackageBlock declares the package identity within a pulumi block.
type PackageBlock struct {
	// Name is the package name. Defaults to filepath.Base(modulePath) at runtime.
	Name string
	// Version is the package version. Defaults to "0.0.0-dev".
	Version string
	// DeclRange is the source range of this block.
	DeclRange hcl.Range
}
