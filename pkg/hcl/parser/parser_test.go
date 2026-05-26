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

package parser

import (
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/pulumi-labs/pulumi-hcl/pkg/hcl/ast"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseBasicConfig(t *testing.T) {
	src := []byte(`
terraform {
  required_providers {
    aws = {
      source  = "pulumi/aws"
      version = "6.0.0"
    }
  }
}

provider "aws" {
  region = "us-west-2"
}

variable "bucket_name" {
  type        = string
  description = "Name of the S3 bucket"
  default     = "my-bucket"
}

locals {
  common_tags = {
    Environment = "dev"
    ManagedBy   = "Pulumi"
  }
}

resource "aws_s3_bucket" "example" {
  bucket = var.bucket_name
  tags   = local.common_tags

  lifecycle {
    prevent_destroy = true
  }
}

data "aws_ami" "ubuntu" {
  most_recent = true
}

output "bucket_arn" {
  value       = aws_s3_bucket.example.arn
  description = "The ARN of the bucket"
  sensitive   = false
}

module "vpc" {
  source  = "./modules/vpc"
  version = "1.0.0"

  cidr_block = "10.0.0.0/16"
}
`)

	p := NewParser()
	config, diags := p.ParseSource("test.hcl", src)

	if diags.HasErrors() {
		for _, d := range diags {
			t.Errorf("Diagnostic: %s: %s", d.Summary, d.Detail)
		}
		t.FailNow()
	}

	// Verify terraform block
	if config.Terraform == nil {
		t.Error("Expected terraform block")
	} else {
		if len(config.Terraform.RequiredProviders) != 1 {
			t.Errorf("Expected 1 required provider, got %d", len(config.Terraform.RequiredProviders))
		}
		if rp, ok := config.Terraform.RequiredProviders["aws"]; ok {
			if rp.Source != "pulumi/aws" {
				t.Errorf("Expected source 'pulumi/aws', got %q", rp.Source)
			}
		} else {
			t.Error("Expected 'aws' in required providers")
		}
	}

	// Verify provider
	if len(config.Providers) != 1 {
		t.Errorf("Expected 1 provider, got %d", len(config.Providers))
	}
	if _, ok := config.Providers["aws"]; !ok {
		t.Error("Expected 'aws' provider")
	}

	// Verify variable
	if len(config.Variables) != 1 {
		t.Errorf("Expected 1 variable, got %d", len(config.Variables))
	}
	if v, ok := config.Variables["bucket_name"]; ok {
		if v.Description != "Name of the S3 bucket" {
			t.Errorf("Unexpected variable description: %q", v.Description)
		}
	} else {
		t.Error("Expected 'bucket_name' variable")
	}

	// Verify locals
	if len(config.Locals) != 1 {
		t.Errorf("Expected 1 local, got %d", len(config.Locals))
	}
	if _, ok := config.Locals["common_tags"]; !ok {
		t.Error("Expected 'common_tags' local")
	}

	// Verify resource
	if len(config.Resources) != 1 {
		t.Errorf("Expected 1 resource, got %d", len(config.Resources))
	}
	if r, ok := config.Resources["aws_s3_bucket.example"]; ok {
		if r.Type != "aws_s3_bucket" {
			t.Errorf("Unexpected resource type: %q", r.Type)
		}
		if r.Name != "example" {
			t.Errorf("Unexpected resource name: %q", r.Name)
		}
		if r.Lifecycle == nil {
			t.Error("Expected lifecycle block")
		} else if r.Lifecycle.PreventDestroy == nil || !*r.Lifecycle.PreventDestroy {
			t.Error("Expected prevent_destroy to be true")
		}
	} else {
		t.Error("Expected 'aws_s3_bucket.example' resource")
	}

	// Verify data source
	if len(config.DataSources) != 1 {
		t.Errorf("Expected 1 data source, got %d", len(config.DataSources))
	}
	if _, ok := config.DataSources["aws_ami.ubuntu"]; !ok {
		t.Error("Expected 'aws_ami.ubuntu' data source")
	}

	// Verify output
	if len(config.Outputs) != 1 {
		t.Errorf("Expected 1 output, got %d", len(config.Outputs))
	}
	if o, ok := config.Outputs["bucket_arn"]; ok {
		if o.Description != "The ARN of the bucket" {
			t.Errorf("Unexpected output description: %q", o.Description)
		}
	} else {
		t.Error("Expected 'bucket_arn' output")
	}

	// Verify module
	if len(config.Modules) != 1 {
		t.Errorf("Expected 1 module, got %d", len(config.Modules))
	}
	if m, ok := config.Modules["vpc"]; ok {
		if m.Source != "./modules/vpc" {
			t.Errorf("Unexpected module source: %q", m.Source)
		}
	} else {
		t.Error("Expected 'vpc' module")
	}
}

func TestParseProvisioners(t *testing.T) {
	src := []byte(`
resource "aws_instance" "web" {
  ami           = "ami-123"
  instance_type = "t3.micro"

  connection {
    type        = "ssh"
    user        = "ubuntu"
    host        = self.public_ip
  }

  provisioner "local-exec" {
    command = "echo hello"
  }

  provisioner "remote-exec" {
    inline = ["apt update"]
    when   = create
  }

  provisioner "file" {
    source      = "local.txt"
    destination = "/tmp/remote.txt"
  }
}
`)

	p := NewParser()
	config, diags := p.ParseSource("test.hcl", src)

	if diags.HasErrors() {
		for _, d := range diags {
			t.Errorf("Diagnostic: %s: %s", d.Summary, d.Detail)
		}
		t.FailNow()
	}

	r, ok := config.Resources["aws_instance.web"]
	if !ok {
		t.Fatal("Expected 'aws_instance.web' resource")
	}

	if r.Connection == nil {
		t.Error("Expected connection block")
	} else if r.Connection.Type != "ssh" {
		t.Errorf("Expected connection type 'ssh', got %q", r.Connection.Type)
	}

	if len(r.Provisioners) != 3 {
		t.Errorf("Expected 3 provisioners, got %d", len(r.Provisioners))
	}

	if r.Provisioners[0].Type != "local-exec" {
		t.Errorf("Expected first provisioner to be 'local-exec', got %q", r.Provisioners[0].Type)
	}

	if r.Provisioners[1].Type != "remote-exec" {
		t.Errorf("Expected second provisioner to be 'remote-exec', got %q", r.Provisioners[1].Type)
	}
	if r.Provisioners[1].When != "create" {
		t.Errorf("Expected when='create', got %q", r.Provisioners[1].When)
	}

	if r.Provisioners[2].Type != "file" {
		t.Errorf("Expected third provisioner to be 'file', got %q", r.Provisioners[2].Type)
	}
}

// TestRequiredProvidersShapes pins down how each documented required_providers
// shape parses, and which of them route through the terraform-provider plugin.
// Sources non-prefixed by "pulumi/" (including a missing source) are TF-style
// and default to "hashicorp/<name>".
func TestRequiredProvidersShapes(t *testing.T) {
	t.Parallel()
	const src = `terraform {
  required_providers {
    ex1 = "~> 4.0"
    ex2 = { version = "~> 4.0" }
    ex3 = { source = "example/abc" }
    ex4 = { source = "example/def", version = "5.0.0" }
    ex5 = { source = "pulumi/ghi", version = "6.7.8" }
  }
}`
	cfg, diags := NewParser().ParseSource("test.hcl", []byte(src))
	require.False(t, diags.HasErrors(), "diags: %v", diags)
	require.NotNil(t, cfg.Terraform)

	got := cfg.Terraform.RequiredProviders
	for _, v := range got {
		v.DeclRange = hcl.Range{}
	}
	assert.Equal(t, map[string]*ast.RequiredProvider{
		"ex1": {
			Name:    "ex1",
			Version: "~> 4.0",
		},
		"ex2": {
			Name:    "ex2",
			Version: "~> 4.0",
		},
		"ex3": {
			Name:   "ex3",
			Source: "example/abc",
		},
		"ex4": {
			Name:    "ex4",
			Source:  "example/def",
			Version: "5.0.0",
		},
		"ex5": {
			Name:    "ex5",
			Source:  "pulumi/ghi",
			Version: "6.7.8",
		},
	}, got)
}

func TestRequiredProvidersVersionMustBeSemver(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		hcl     string
		wantErr bool
	}{
		{
			name: "pulumi_source_semver_ok",
			hcl: `terraform {
  required_providers {
    aws = { source = "pulumi/aws", version = "6.0.0" }
  }
}`,
		},
		{
			name: "pulumi_source_constraint_rejected",
			hcl: `terraform {
  required_providers {
    aws = { source = "pulumi/aws", version = "~> 6.0" }
  }
}`,
			wantErr: true,
		},
		{
			// Empty source now defaults to "hashicorp/<name>" (TF-style),
			// so a constraint-form version is fine.
			name: "no_source_constraint_ok",
			hcl: `terraform {
  required_providers {
    aws = { version = "~> 6.0" }
  }
}`,
		},
		{
			name: "tf_source_constraint_ok",
			hcl: `terraform {
  required_providers {
    random = { source = "hashicorp/random", version = "~> 4.0.0" }
  }
}`,
		},
		{
			name: "pulumi_source_no_version_ok",
			hcl: `terraform {
  required_providers {
    aws = { source = "pulumi/aws" }
  }
}`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, diags := NewParser().ParseSource("test.hcl", []byte(tc.hcl))
			if tc.wantErr {
				require.True(t, diags.HasErrors(), "expected parse errors, got %v", diags)
				require.Equal(t, "Invalid provider version", diags[0].Summary)
			} else {
				require.False(t, diags.HasErrors(), "unexpected parse errors: %v", diags)
			}
		})
	}
}

func TestTerraformBlockCompat(t *testing.T) {
	t.Parallel()

	t.Run("multiple_terraform_blocks_merge", func(t *testing.T) {
		t.Parallel()
		src := `
terraform {
  backend "remote" {
    organization = "pulumi"
  }
}

terraform {
  required_providers {
    aws = { source = "hashicorp/aws", version = "6.19.0" }
  }
}`
		cfg, diags := NewParser().ParseSource("test.hcl", []byte(src))
		require.False(t, diags.HasErrors(), "unexpected errors: %v", diags)
		require.NotNil(t, cfg.Terraform)
		require.Contains(t, cfg.Terraform.RequiredProviders, "aws")
	})

	t.Run("backend_warns_continues", func(t *testing.T) {
		t.Parallel()
		src := `
terraform {
  backend "remote" {
    organization = "pulumi"
  }
  required_providers {
    aws = { source = "hashicorp/aws", version = "6.19.0" }
  }
}`
		cfg, diags := NewParser().ParseSource("test.hcl", []byte(src))
		require.False(t, diags.HasErrors(), "unexpected errors: %v", diags)
		require.Contains(t, cfg.Terraform.RequiredProviders, "aws")
		var found bool
		for _, d := range diags {
			if d.Severity == hcl.DiagWarning && d.Summary == "Ignoring terraform backend block" {
				found = true
				break
			}
		}
		require.True(t, found, "expected warning for backend block, got %v", diags)
	})

	t.Run("duplicate_required_provider_across_blocks", func(t *testing.T) {
		t.Parallel()
		src := `
terraform {
  required_providers {
    aws = { source = "hashicorp/aws", version = "6.19.0" }
  }
}

terraform {
  required_providers {
    aws = { source = "hashicorp/aws", version = "7.0.0" }
  }
}`
		_, diags := NewParser().ParseSource("test.hcl", []byte(src))
		require.True(t, diags.HasErrors(), "expected duplicate-provider error, got %v", diags)
		require.Equal(t, "Duplicate required_providers entry", diags[0].Summary)
	})

	t.Run("duplicate_required_version_range_across_blocks", func(t *testing.T) {
		t.Parallel()
		src := `
terraform {
  required_version_range = ">= 1.0.0"
}

terraform {
  required_version_range = ">= 2.0.0"
}`
		_, diags := NewParser().ParseSource("test.hcl", []byte(src))
		require.True(t, diags.HasErrors(), "expected duplicate-required_version_range error, got %v", diags)
		require.Equal(t, "Duplicate required_version_range attribute", diags[0].Summary)
	})

	t.Run("required_version_warns_continues", func(t *testing.T) {
		t.Parallel()
		src := `
terraform {
  required_version = ">= 1.0.0"
  required_providers {
    aws = { source = "hashicorp/aws", version = "6.19.0" }
  }
}`
		cfg, diags := NewParser().ParseSource("test.hcl", []byte(src))
		require.False(t, diags.HasErrors(), "unexpected errors: %v", diags)
		require.Contains(t, cfg.Terraform.RequiredProviders, "aws")
		var found bool
		for _, d := range diags {
			if d.Severity == hcl.DiagWarning && d.Summary == "Unsupported attribute: required_version" {
				found = true
				break
			}
		}
		require.True(t, found, "expected warning for required_version, got %v", diags)
	})
}

func TestParseProvisionerInvalidWhen(t *testing.T) {
	src := []byte(`
resource "aws_instance" "web" {
  provisioner "local-exec" {
    command = "true"
    when    = "sometimes"
  }
}
`)
	p := NewParser()
	_, diags := p.ParseSource("test.hcl", src)
	require.True(t, diags.HasErrors())
	require.Equal(t, "Invalid \"when\" value", diags[0].Summary)
}

func TestParseProvisionerInvalidOnFailure(t *testing.T) {
	src := []byte(`
resource "aws_instance" "web" {
  provisioner "local-exec" {
    command    = "true"
    on_failure = "panic"
  }
}
`)
	p := NewParser()
	_, diags := p.ParseSource("test.hcl", src)
	require.True(t, diags.HasErrors())
	require.Equal(t, "Invalid \"on_failure\" value", diags[0].Summary)
}

func TestParseMetaArguments(t *testing.T) {
	src := []byte(`
resource "aws_instance" "web" {
  count = 3

  ami           = "ami-123"
  instance_type = "t3.micro"

  depends_on = [aws_vpc.main, aws_subnet.primary]

  lifecycle {
    create_before_destroy = true
    ignore_changes        = [tags, ami]
  }
}

resource "aws_instance" "app" {
  for_each = toset(["a", "b", "c"])

  ami           = "ami-456"
  instance_type = "t3.small"

  lifecycle {
    ignore_changes = all
  }
}
`)

	p := NewParser()
	config, diags := p.ParseSource("test.hcl", src)

	if diags.HasErrors() {
		for _, d := range diags {
			t.Errorf("Diagnostic: %s: %s", d.Summary, d.Detail)
		}
		t.FailNow()
	}

	// Check count resource
	r1, ok := config.Resources["aws_instance.web"]
	if !ok {
		t.Fatal("Expected 'aws_instance.web' resource")
	}

	if r1.Count == nil {
		t.Error("Expected count expression")
	}

	if len(r1.DependsOn) != 2 {
		t.Errorf("Expected 2 depends_on entries, got %d", len(r1.DependsOn))
	}

	if r1.Lifecycle == nil {
		t.Error("Expected lifecycle block")
	} else {
		if r1.Lifecycle.CreateBeforeDestroy == nil || !*r1.Lifecycle.CreateBeforeDestroy {
			t.Error("Expected create_before_destroy to be true")
		}
		if len(r1.Lifecycle.IgnoreChanges) != 2 {
			t.Errorf("Expected 2 ignore_changes, got %d", len(r1.Lifecycle.IgnoreChanges))
		}
	}

	// Check for_each resource
	r2, ok := config.Resources["aws_instance.app"]
	if !ok {
		t.Fatal("Expected 'aws_instance.app' resource")
	}

	if r2.ForEach == nil {
		t.Error("Expected for_each expression")
	}

	if r2.Lifecycle == nil {
		t.Error("Expected lifecycle block")
	} else if !r2.Lifecycle.IgnoreAllChanges {
		t.Error("Expected ignore_changes = all")
	}
}

func TestParseProviderCallReserved(t *testing.T) {
	src := []byte(`
provider "call" {
  value = "x"
}
`)

	p := NewParser()
	_, diags := p.ParseSource("test.tf", src)
	if !diags.HasErrors() {
		t.Fatal("expected parse error for provider named \"call\"")
	}
	if got := diags.Error(); !contains(got, `"call" is reserved`) {
		t.Errorf("parser error missing expected substring; got: %q", got)
	}
}

// TF 1.3+ allows `optional(<type>, <default>)` for object attributes.
// HCL's typeexpr.TypeConstraint rejects the 2-arg form; we use the
// TypeConstraintWithDefaults variant. aws-ia/vpc, aws-ia/ipam, and many
// other registry modules use this idiom.
func TestVariableTypeOptionalWithDefault(t *testing.T) {
	t.Parallel()
	src := []byte(`
variable "subnets" {
  type = object({
    name    = string
    cidr    = optional(string, "10.0.0.0/16")
    enabled = optional(bool, true)
  })
}
`)
	_, diags := NewParser().ParseSource("test.tf", src)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %v", diags.Errs())
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
