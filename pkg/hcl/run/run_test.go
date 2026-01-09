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

package run

import (
	"context"
	"testing"

	"github.com/pulumi/pulumi-language-hcl/pkg/hcl/parser"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
)

// mockResourceMonitor is a mock implementation of ResourceMonitor for testing.
type mockResourceMonitor struct {
	registeredResources []RegisterResourceRequest
	invokedFunctions    []InvokeRequest
}

func (m *mockResourceMonitor) RegisterResource(ctx context.Context, req RegisterResourceRequest) (*RegisterResourceResponse, error) {
	m.registeredResources = append(m.registeredResources, req)
	return &RegisterResourceResponse{
		URN:     "urn:pulumi:test::project::" + req.Type + "::" + req.Name,
		ID:      req.Name + "-id",
		Outputs: req.Inputs,
	}, nil
}

func (m *mockResourceMonitor) Invoke(ctx context.Context, req InvokeRequest) (*InvokeResponse, error) {
	m.invokedFunctions = append(m.invokedFunctions, req)
	return &InvokeResponse{
		Return: resource.PropertyMap{
			"id": resource.NewStringProperty("mock-id"),
		},
	}, nil
}

func (m *mockResourceMonitor) RegisterResourceOutputs(ctx context.Context, urn string, outputs resource.PropertyMap) error {
	// No-op for tests
	return nil
}

func TestEngine_BasicResource(t *testing.T) {
	src := []byte(`
variable "name" {
  type    = string
  default = "test"
}

resource "aws_instance" "web" {
  ami = "ami-12345"
  instance_type = var.name
}
`)

	p := parser.NewParser()
	config, diags := p.ParseSource("test.hcl", src)
	if diags.HasErrors() {
		t.Fatalf("parse error: %s", diags.Error())
	}

	mock := &mockResourceMonitor{}
	engine := NewEngine(config, &EngineOptions{
		ProjectName:     "test-project",
		StackName:       "dev",
		ResourceMonitor: mock,
		TestMode:        true, // Skip provider validation
	})

	ctx := context.Background()
	err := engine.Run(ctx)
	if err != nil {
		t.Fatalf("run error: %v", err)
	}

	// Should have registered the stack + one resource
	if len(mock.registeredResources) != 2 {
		t.Fatalf("expected 2 registered resources (stack + resource), got %d", len(mock.registeredResources))
	}

	// First resource should be the stack
	if mock.registeredResources[0].Type != "pulumi:pulumi:Stack" {
		t.Errorf("expected first resource to be stack, got %s", mock.registeredResources[0].Type)
	}

	req := mock.registeredResources[1]
	if req.Name != "aws_instance.web" {
		t.Errorf("expected resource name 'aws_instance.web', got %s", req.Name)
	}
	if req.Inputs["ami"].StringValue() != "ami-12345" {
		t.Errorf("expected ami 'ami-12345', got %v", req.Inputs["ami"])
	}
	if req.Inputs["instance_type"].StringValue() != "test" {
		t.Errorf("expected instance_type 'test', got %v", req.Inputs["instance_type"])
	}
}

func TestEngine_LocalsAndVariables(t *testing.T) {
	src := []byte(`
variable "env" {
  type    = string
  default = "dev"
}

locals {
  prefix = "myapp-${var.env}"
}

resource "aws_s3_bucket" "mybucket" {
  bucket = "${local.prefix}-bucket"
}
`)

	p := parser.NewParser()
	config, diags := p.ParseSource("test.hcl", src)
	if diags.HasErrors() {
		t.Fatalf("parse error: %s", diags.Error())
	}

	mock := &mockResourceMonitor{}
	engine := NewEngine(config, &EngineOptions{
		ProjectName:     "test-project",
		StackName:       "dev",
		ResourceMonitor: mock,
		TestMode:        true,
	})

	ctx := context.Background()
	err := engine.Run(ctx)
	if err != nil {
		t.Fatalf("run error: %v", err)
	}

	if len(mock.registeredResources) != 2 {
		t.Fatalf("expected 2 registered resources (stack + resource), got %d", len(mock.registeredResources))
	}

	req := mock.registeredResources[1]
	bucketName := req.Inputs["bucket"].StringValue()
	if bucketName != "myapp-dev-bucket" {
		t.Errorf("expected bucket 'myapp-dev-bucket', got %s", bucketName)
	}
}

func TestEngine_ResourceDependencies(t *testing.T) {
	src := []byte(`
resource "aws_vpc" "main" {
  cidr_block = "10.0.0.0/16"
}

resource "aws_subnet" "main" {
  vpc_id     = aws_vpc.main.id
  cidr_block = "10.0.1.0/24"
}
`)

	p := parser.NewParser()
	config, diags := p.ParseSource("test.hcl", src)
	if diags.HasErrors() {
		t.Fatalf("parse error: %s", diags.Error())
	}

	mock := &mockResourceMonitor{}
	engine := NewEngine(config, &EngineOptions{
		ProjectName:     "test-project",
		StackName:       "dev",
		ResourceMonitor: mock,
		TestMode:        true,
	})

	ctx := context.Background()
	err := engine.Run(ctx)
	if err != nil {
		t.Fatalf("run error: %v", err)
	}

	// Should have stack + 2 resources registered in dependency order
	if len(mock.registeredResources) != 3 {
		t.Fatalf("expected 3 registered resources (stack + 2 resources), got %d", len(mock.registeredResources))
	}

	// VPC should be registered first (after stack)
	if mock.registeredResources[1].Name != "aws_vpc.main" {
		t.Errorf("expected aws_vpc.main first, got %s", mock.registeredResources[1].Name)
	}

	// Subnet should be registered second
	if mock.registeredResources[2].Name != "aws_subnet.main" {
		t.Errorf("expected aws_subnet.main second, got %s", mock.registeredResources[2].Name)
	}
}

func TestEngine_NoResourceMonitor(t *testing.T) {
	// Test that engine works without a resource monitor (for validation/testing)
	src := []byte(`
variable "name" {
  type    = string
  default = "test"
}

resource "aws_instance" "web" {
  ami = var.name
}
`)

	p := parser.NewParser()
	config, diags := p.ParseSource("test.hcl", src)
	if diags.HasErrors() {
		t.Fatalf("parse error: %s", diags.Error())
	}

	// No resource monitor - should still work
	engine := NewEngine(config, &EngineOptions{
		ProjectName: "test-project",
		StackName:   "dev",
		TestMode:    true,
	})

	ctx := context.Background()
	err := engine.Run(ctx)
	if err != nil {
		t.Fatalf("run error: %v", err)
	}
}

func TestValidate(t *testing.T) {
	t.Run("valid config", func(t *testing.T) {
		src := []byte(`
variable "name" {
  type = string
}

resource "aws_instance" "web" {
  ami = var.name
}
`)
		p := parser.NewParser()
		config, diags := p.ParseSource("test.hcl", src)
		if diags.HasErrors() {
			t.Fatalf("parse error: %s", diags.Error())
		}

		errs := Validate(config)
		if len(errs) != 0 {
			t.Errorf("expected no errors, got %v", errs)
		}
	})

	t.Run("missing dependency", func(t *testing.T) {
		src := []byte(`
resource "aws_instance" "web" {
  ami = nonexistent_resource.foo.id
}
`)
		p := parser.NewParser()
		config, diags := p.ParseSource("test.hcl", src)
		if diags.HasErrors() {
			t.Fatalf("parse error: %s", diags.Error())
		}

		errs := Validate(config)
		// Should have a warning about missing dependency
		if len(errs) == 0 {
			t.Error("expected validation errors for missing dependency")
		}
	})
}

func TestEngine_DependsOn(t *testing.T) {
	src := []byte(`
resource "aws_s3_bucket" "mybucket" {
  bucket = "my-bucket"
}

resource "aws_instance" "web" {
  ami = "ami-12345"

  depends_on = [aws_s3_bucket.mybucket]
}
`)

	p := parser.NewParser()
	config, diags := p.ParseSource("test.hcl", src)
	if diags.HasErrors() {
		t.Fatalf("parse error: %s", diags.Error())
	}

	mock := &mockResourceMonitor{}
	engine := NewEngine(config, &EngineOptions{
		ProjectName:     "test-project",
		StackName:       "dev",
		ResourceMonitor: mock,
		TestMode:        true,
	})

	ctx := context.Background()
	err := engine.Run(ctx)
	if err != nil {
		t.Fatalf("run error: %v", err)
	}

	// Should have stack + 2 resources, bucket first due to depends_on
	if len(mock.registeredResources) != 3 {
		t.Fatalf("expected 3 registered resources (stack + 2 resources), got %d", len(mock.registeredResources))
	}

	// Bucket should be first (after stack)
	if mock.registeredResources[1].Name != "aws_s3_bucket.mybucket" {
		t.Errorf("expected bucket first, got %s", mock.registeredResources[1].Name)
	}

	// Instance should have depends_on set
	if len(mock.registeredResources[2].Dependencies) == 0 {
		t.Error("expected instance to have dependencies from depends_on")
	}
}

func TestEngine_Lifecycle(t *testing.T) {
	src := []byte(`
resource "aws_instance" "web" {
  ami = "ami-12345"

  lifecycle {
    prevent_destroy = true
    ignore_changes  = [tags]
  }
}
`)

	p := parser.NewParser()
	config, diags := p.ParseSource("test.hcl", src)
	if diags.HasErrors() {
		t.Fatalf("parse error: %s", diags.Error())
	}

	mock := &mockResourceMonitor{}
	engine := NewEngine(config, &EngineOptions{
		ProjectName:     "test-project",
		StackName:       "dev",
		ResourceMonitor: mock,
		TestMode:        true,
	})

	ctx := context.Background()
	err := engine.Run(ctx)
	if err != nil {
		t.Fatalf("run error: %v", err)
	}

	if len(mock.registeredResources) != 2 {
		t.Fatalf("expected 2 registered resources (stack + resource), got %d", len(mock.registeredResources))
	}

	req := mock.registeredResources[1]

	// prevent_destroy maps to Protect
	if !req.Protect {
		t.Error("expected Protect=true from prevent_destroy")
	}

	// ignore_changes should be set
	if len(req.IgnoreChanges) == 0 {
		t.Error("expected IgnoreChanges to be set")
	}
}
