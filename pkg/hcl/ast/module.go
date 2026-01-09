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

// Module represents a module block in HCL.
//
// In Pulumi HCL, modules map to Pulumi components (MLCs - Multi-Language Components).
//
// Terraform syntax:
//
//	module "vpc" {
//	  source  = "./modules/vpc"
//	  version = "~> 3.0"
//
//	  cidr_block = "10.0.0.0/16"
//	  name       = "main"
//
//	  providers = {
//	    aws = aws.west
//	  }
//
//	  depends_on = [module.network]
//	}
type Module struct {
	// Name is the module instance name (the label on the module block).
	Name string

	// Source is the module source address.
	// Can be:
	// - Local path: "./modules/vpc"
	// - Pulumi package: "pulumi/eks"
	// - Registry: "hashicorp/consul/aws"
	Source string

	// Version is the version constraint for registry/package modules.
	Version string

	// Config is the body containing module input variables.
	// This excludes meta-arguments which are parsed separately.
	Config hcl.Body

	// Count is the count meta-argument expression, if present.
	Count hcl.Expression

	// ForEach is the for_each meta-argument expression, if present.
	ForEach hcl.Expression

	// DependsOn contains explicit dependencies from the depends_on meta-argument.
	DependsOn []hcl.Traversal

	// Providers maps provider names to provider references for passing to the module.
	Providers map[string]*ProviderRef

	// DeclRange is the source range of the module block.
	DeclRange hcl.Range
}
