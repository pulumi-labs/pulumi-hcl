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

// Terraform represents a terraform block with program-level options.
//
// Syntax:
//
//	terraform {
//	  required_version_range = ">=3.0.0"
//
//	  required_providers {
//	    aws = {
//	      source  = "pulumi/aws"
//	      version = "~> 6.0"
//	    }
//	  }
//
//	  component {
//	    name   = "MyComponent"
//	    module = "index"
//	  }
//	  package {
//	    name    = "my-package"
//	    version = "1.0.0"
//	  }
//	}
type Terraform struct {
	// RequiredVersionRange is the version range expression for the Pulumi CLI.
	RequiredVersionRange hcl.Expression

	// RequiredProviders maps provider local name to its requirements.
	RequiredProviders map[string]*RequiredProvider

	// Component declares this module as a multi-language component.
	Component *ComponentBlock

	// Package declares the package identity for an MLC module.
	Package *PackageBlock

	// DeclRange is the source range of the terraform block.
	DeclRange hcl.Range
}

// ComponentBlock declares a component within a terraform block.
type ComponentBlock struct {
	// Name is the component name (required). Must be a valid Pulumi name.
	Name string
	// Module is the module segment of the resource token. Defaults to "index".
	Module string
	// DeclRange is the source range of this block.
	DeclRange hcl.Range
}

// PackageBlock declares the package identity within a terraform block.
type PackageBlock struct {
	// Name is the package name. Defaults to filepath.Base(modulePath) at runtime.
	Name string
	// Version is the package version. Defaults to "0.0.0-dev".
	Version string
	// DeclRange is the source range of this block.
	DeclRange hcl.Range
}

// RequiredProvider represents a provider requirement in the required_providers block.
type RequiredProvider struct {
	// Name is the local name for this provider (e.g., "aws").
	Name string

	// Source is the provider source address (e.g., "pulumi/aws").
	Source string

	// Version is the version constraint (e.g., "~> 5.0").
	Version string

	// DeclRange is the source range of this provider requirement.
	DeclRange hcl.Range
}
