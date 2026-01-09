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

// Terraform represents a terraform block in HCL.
//
// Terraform syntax:
//
//	terraform {
//	  required_version = ">= 1.0.0"
//
//	  required_providers {
//	    aws = {
//	      source  = "hashicorp/aws"
//	      version = "~> 5.0"
//	    }
//	  }
//
//	  backend "s3" {
//	    bucket = "my-state-bucket"
//	    key    = "terraform.tfstate"
//	  }
//	}
//
// Note: In Pulumi HCL, the backend block is ignored (Pulumi manages state differently).
// The required_version is also ignored (Pulumi has its own versioning).
type Terraform struct {
	// RequiredVersion is the version constraint string, if specified.
	// This is ignored in Pulumi but preserved for compatibility.
	RequiredVersion string

	// RequiredProviders maps provider local name to its requirements.
	RequiredProviders map[string]*RequiredProvider

	// Backend is the backend configuration, if specified.
	// This is ignored in Pulumi but preserved for parsing compatibility.
	Backend *Backend

	// DeclRange is the source range of the terraform block.
	DeclRange hcl.Range
}

// RequiredProvider represents a provider requirement in the required_providers block.
type RequiredProvider struct {
	// Name is the local name for this provider (e.g., "aws").
	Name string

	// Source is the provider source address (e.g., "hashicorp/aws" or "pulumi/aws").
	Source string

	// Version is the version constraint (e.g., "~> 5.0").
	Version string

	// ConfigurationAliases lists provider configuration aliases.
	ConfigurationAliases []string

	// DeclRange is the source range of this provider requirement.
	DeclRange hcl.Range
}

// Backend represents a backend configuration block.
// This is ignored in Pulumi HCL but parsed for compatibility.
type Backend struct {
	// Type is the backend type (e.g., "s3", "gcs", "azurerm").
	Type string

	// Config is the body containing backend configuration.
	Config hcl.Body

	// DeclRange is the source range of the backend block.
	DeclRange hcl.Range
}
