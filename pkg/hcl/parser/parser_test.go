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
)

func TestParseBasicConfig(t *testing.T) {
	src := []byte(`
terraform {
  required_providers {
    aws = {
      source  = "pulumi/aws"
      version = "~> 6.0"
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
		} else if !r.Lifecycle.PreventDestroy {
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
