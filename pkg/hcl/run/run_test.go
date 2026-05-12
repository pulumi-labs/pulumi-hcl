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

package run_test

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/pulumi-labs/pulumi-hcl/pkg/hcl/parser"
	"github.com/pulumi-labs/pulumi-hcl/pkg/hcl/run"
	"github.com/pulumi-labs/pulumi-hcl/tests/testutil"
	"github.com/pulumi-labs/pulumi-hcl/tests/testutil/schemaloader"
	"github.com/pulumi/pulumi/pkg/v3/codegen/schema"
	"github.com/pulumi/pulumi/sdk/v3/go/property"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEngine_BasicResource(t *testing.T) {
	t.Parallel()

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

	mock := &testutil.MockResourceMonitor{}
	engine := run.NewEngine(config, &run.EngineOptions{
		ProjectName:     "test-project",
		StackName:       "dev",
		ResourceMonitor: mock,
		WorkDir:         t.TempDir(),
		RootDir:         t.TempDir(),
		SchemaLoader: schemaloader.New(t, schema.PackageSpec{
			Name: "aws",
			Resources: map[string]schema.ResourceSpec{
				"aws:index:Instance": {
					InputProperties: map[string]schema.PropertySpec{
						"ami":          {TypeSpec: schema.TypeSpec{Type: "string"}},
						"instanceType": {TypeSpec: schema.TypeSpec{Type: "string"}},
					},
					ObjectTypeSpec: schema.ObjectTypeSpec{
						Properties: map[string]schema.PropertySpec{
							"ami":          {TypeSpec: schema.TypeSpec{Type: "string"}},
							"instanceType": {TypeSpec: schema.TypeSpec{Type: "string"}},
						},
					},
				},
			},
		}),
	})

	err := engine.Run(t.Context())
	if err != nil {
		t.Fatalf("run error: %v", err)
	}

	// Should have registered the stack + one resource
	if len(mock.RegisteredResources) != 2 {
		t.Fatalf("expected 2 registered resources (stack + resource), got %d", len(mock.RegisteredResources))
	}

	// First resource should be the stack
	if mock.RegisteredResources[0].Type != "pulumi:pulumi:Stack" {
		t.Errorf("expected first resource to be stack, got %s", mock.RegisteredResources[0].Type)
	}

	req := mock.RegisteredResources[1]
	if req.Name != "web" {
		t.Errorf("expected resource name 'web', got %s", req.Name)
	}
	if req.Inputs.Get("ami").AsString() != "ami-12345" {
		t.Errorf("expected ami 'ami-12345', got %v", req.Inputs.Get("ami"))
	}
	if req.Inputs.Get("instanceType").AsString() != "test" {
		t.Errorf("expected instanceType 'test', got %v", req.Inputs.Get("instanceType"))
	}
}

func TestEngine_LocalsAndVariables(t *testing.T) {
	t.Parallel()

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

	mock := &testutil.MockResourceMonitor{}
	engine := run.NewEngine(config, &run.EngineOptions{
		ProjectName:     "test-project",
		StackName:       "dev",
		ResourceMonitor: mock,
		WorkDir:         t.TempDir(),
		RootDir:         t.TempDir(),
		SchemaLoader: schemaloader.New(t, schema.PackageSpec{
			Name: "aws",
			Resources: map[string]schema.ResourceSpec{
				"aws:s3:Bucket": {
					InputProperties: map[string]schema.PropertySpec{
						"bucket": {TypeSpec: schema.TypeSpec{Type: "string"}},
					},
					ObjectTypeSpec: schema.ObjectTypeSpec{
						Properties: map[string]schema.PropertySpec{
							"bucket": {TypeSpec: schema.TypeSpec{Type: "string"}},
						},
					},
				},
			},
		}),
	})

	err := engine.Run(t.Context())
	if err != nil {
		t.Fatalf("run error: %v", err)
	}

	if len(mock.RegisteredResources) != 2 {
		t.Fatalf("expected 2 registered resources (stack + resource), got %d", len(mock.RegisteredResources))
	}

	req := mock.RegisteredResources[1]
	bucketName := req.Inputs.Get("bucket").AsString()
	if bucketName != "myapp-dev-bucket" {
		t.Errorf("expected bucket 'myapp-dev-bucket', got %s", bucketName)
	}
}

func TestEngine_ResourceDependencies(t *testing.T) {
	t.Parallel()

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

	mock := &testutil.MockResourceMonitor{}
	engine := run.NewEngine(config, &run.EngineOptions{
		ProjectName:     "test-project",
		StackName:       "dev",
		ResourceMonitor: mock,
		WorkDir:         t.TempDir(),
		RootDir:         t.TempDir(),
		SchemaLoader: schemaloader.New(t, schema.PackageSpec{
			Name: "aws",
			Resources: map[string]schema.ResourceSpec{
				"aws:index:Vpc": {
					InputProperties: map[string]schema.PropertySpec{
						"cidrBlock": {TypeSpec: schema.TypeSpec{Type: "string"}},
					},
					ObjectTypeSpec: schema.ObjectTypeSpec{
						Properties: map[string]schema.PropertySpec{
							"cidrBlock": {TypeSpec: schema.TypeSpec{Type: "string"}},
						},
					},
				},
				"aws:index:Subnet": {
					InputProperties: map[string]schema.PropertySpec{
						"vpcId":     {TypeSpec: schema.TypeSpec{Type: "string"}},
						"cidrBlock": {TypeSpec: schema.TypeSpec{Type: "string"}},
					},
					ObjectTypeSpec: schema.ObjectTypeSpec{
						Properties: map[string]schema.PropertySpec{
							"vpcId":     {TypeSpec: schema.TypeSpec{Type: "string"}},
							"cidrBlock": {TypeSpec: schema.TypeSpec{Type: "string"}},
						},
					},
				},
			},
		}),
	})

	err := engine.Run(t.Context())
	if err != nil {
		t.Fatalf("run error: %v", err)
	}

	// Should have stack + 2 resources registered in dependency order
	if len(mock.RegisteredResources) != 3 {
		t.Fatalf("expected 3 registered resources (stack + 2 resources), got %d", len(mock.RegisteredResources))
	}

	// VPC should be registered first (after stack)
	if mock.RegisteredResources[1].Name != "main" {
		t.Errorf("expected main first, got %s", mock.RegisteredResources[1].Name)
	}

	// Subnet should be registered second
	if mock.RegisteredResources[2].Name != "main" {
		t.Errorf("expected main second, got %s", mock.RegisteredResources[2].Name)
	}
}

func TestValidate(t *testing.T) {
	t.Parallel()

	t.Run("valid config", func(t *testing.T) {
		t.Parallel()

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

		errs := run.Validate(config)
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

		errs := run.Validate(config)
		// Should have a warning about missing dependency
		if len(errs) == 0 {
			t.Error("expected validation errors for missing dependency")
		}
	})
}

func TestEngine_DependsOn(t *testing.T) {
	t.Parallel()

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

	mock := &testutil.MockResourceMonitor{}
	engine := run.NewEngine(config, &run.EngineOptions{
		ProjectName:     "test-project",
		StackName:       "dev",
		ResourceMonitor: mock,
		WorkDir:         t.TempDir(),
		RootDir:         t.TempDir(),
		SchemaLoader: schemaloader.New(t, schema.PackageSpec{
			Name: "aws",
			Resources: map[string]schema.ResourceSpec{
				"aws:index:Instance": {
					InputProperties: map[string]schema.PropertySpec{
						"ami": {TypeSpec: schema.TypeSpec{Type: "string"}},
					},
					ObjectTypeSpec: schema.ObjectTypeSpec{
						Properties: map[string]schema.PropertySpec{
							"ami": {TypeSpec: schema.TypeSpec{Type: "string"}},
						},
					},
				},
				"aws:s3:Bucket": {
					InputProperties: map[string]schema.PropertySpec{
						"bucket": {TypeSpec: schema.TypeSpec{Type: "string"}},
					},
					ObjectTypeSpec: schema.ObjectTypeSpec{
						Properties: map[string]schema.PropertySpec{
							"bucket": {TypeSpec: schema.TypeSpec{Type: "string"}},
						},
					},
				},
			},
		}),
	})

	err := engine.Run(t.Context())
	if err != nil {
		t.Fatalf("run error: %v", err)
	}

	// Should have stack + 2 resources, bucket first due to depends_on
	if len(mock.RegisteredResources) != 3 {
		t.Fatalf("expected 3 registered resources (stack + 2 resources), got %d", len(mock.RegisteredResources))
	}

	// Bucket should be first (after stack)
	if mock.RegisteredResources[1].Name != "mybucket" {
		t.Errorf("expected bucket first, got %s", mock.RegisteredResources[1].Name)
	}

	// Instance should have depends_on set
	if len(mock.RegisteredResources[2].Dependencies) == 0 {
		t.Error("expected instance to have dependencies from depends_on")
	}
}

func TestEngine_Lifecycle(t *testing.T) {
	t.Parallel()

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

	mock := &testutil.MockResourceMonitor{}
	engine := run.NewEngine(config, &run.EngineOptions{
		ProjectName:     "test-project",
		StackName:       "dev",
		ResourceMonitor: mock,
		WorkDir:         t.TempDir(),
		RootDir:         t.TempDir(),
		SchemaLoader: schemaloader.New(t, schema.PackageSpec{
			Name: "aws",
			Resources: map[string]schema.ResourceSpec{
				"aws:index:Instance": {
					InputProperties: map[string]schema.PropertySpec{
						"ami": {TypeSpec: schema.TypeSpec{Type: "string"}},
					},
					ObjectTypeSpec: schema.ObjectTypeSpec{
						Properties: map[string]schema.PropertySpec{
							"ami": {TypeSpec: schema.TypeSpec{Type: "string"}},
						},
					},
				},
			},
		}),
	})

	err := engine.Run(t.Context())
	if err != nil {
		t.Fatalf("run error: %v", err)
	}

	if len(mock.RegisteredResources) != 2 {
		t.Fatalf("expected 2 registered resources (stack + resource), got %d", len(mock.RegisteredResources))
	}

	req := mock.RegisteredResources[1]

	// prevent_destroy maps to Protect
	if !req.Protect {
		t.Error("expected Protect=true from prevent_destroy")
	}

	// ignore_changes should be set
	if len(req.IgnoreChanges) == 0 {
		t.Error("expected IgnoreChanges to be set")
	}
}

func TestEngine_CreateBeforeDestroy(t *testing.T) {
	t.Parallel()

	src := []byte(`
resource "aws_instance" "web" {
  ami = "ami-12345"

  lifecycle {
    create_before_destroy = true
  }
}
`)

	p := parser.NewParser()
	config, diags := p.ParseSource("test.hcl", src)
	if diags.HasErrors() {
		t.Fatalf("parse error: %s", diags.Error())
	}

	mock := &testutil.MockResourceMonitor{}
	engine := run.NewEngine(config, &run.EngineOptions{
		ProjectName:     "test-project",
		StackName:       "dev",
		ResourceMonitor: mock,
		WorkDir:         t.TempDir(),
		RootDir:         t.TempDir(),
		SchemaLoader: schemaloader.New(t, schema.PackageSpec{
			Name: "aws",
			Resources: map[string]schema.ResourceSpec{
				"aws:index:Instance": {
					InputProperties: map[string]schema.PropertySpec{
						"ami": {TypeSpec: schema.TypeSpec{Type: "string"}},
					},
					ObjectTypeSpec: schema.ObjectTypeSpec{
						Properties: map[string]schema.PropertySpec{
							"ami": {TypeSpec: schema.TypeSpec{Type: "string"}},
						},
					},
				},
			},
		}),
	})

	err := engine.Run(t.Context())
	if err != nil {
		t.Fatalf("run error: %v", err)
	}

	if len(mock.RegisteredResources) != 2 {
		t.Fatalf("expected 2 registered resources (stack + resource), got %d", len(mock.RegisteredResources))
	}

	req := mock.RegisteredResources[1]

	// create_before_destroy = true should map to DeleteBeforeReplace = false
	// (opposite semantics: TF "create before destroy" vs Pulumi "delete before replace")
	if !req.DeleteBeforeReplaceDef {
		t.Error("expected DeleteBeforeReplaceDef=true when create_before_destroy is set")
	}
	if req.DeleteBeforeReplace {
		t.Error("expected DeleteBeforeReplace=false from create_before_destroy=true")
	}
}

func TestEngine_CreateBeforeDestroyFalse(t *testing.T) {
	t.Parallel()

	// Explicit create_before_destroy = false should enable Terraform's default
	// behavior (delete-then-create), which maps to Pulumi's deleteBeforeReplace = true
	src := []byte(`
resource "aws_instance" "web" {
  ami = "ami-12345"

  lifecycle {
    create_before_destroy = false
  }
}
`)

	p := parser.NewParser()
	config, diags := p.ParseSource("test.hcl", src)
	if diags.HasErrors() {
		t.Fatalf("parse error: %s", diags.Error())
	}

	mock := &testutil.MockResourceMonitor{}
	engine := run.NewEngine(config, &run.EngineOptions{
		ProjectName:     "test-project",
		StackName:       "dev",
		ResourceMonitor: mock,
		WorkDir:         t.TempDir(),
		RootDir:         t.TempDir(),
		SchemaLoader: schemaloader.New(t, schema.PackageSpec{
			Name: "aws",
			Resources: map[string]schema.ResourceSpec{
				"aws:index:Instance": {
					InputProperties: map[string]schema.PropertySpec{
						"ami": {TypeSpec: schema.TypeSpec{Type: "string"}},
					},
					ObjectTypeSpec: schema.ObjectTypeSpec{
						Properties: map[string]schema.PropertySpec{
							"ami": {TypeSpec: schema.TypeSpec{Type: "string"}},
						},
					},
				},
			},
		}),
	})

	err := engine.Run(t.Context())
	if err != nil {
		t.Fatalf("run error: %v", err)
	}

	if len(mock.RegisteredResources) != 2 {
		t.Fatalf("expected 2 registered resources (stack + resource), got %d", len(mock.RegisteredResources))
	}

	req := mock.RegisteredResources[1]

	// create_before_destroy = false should map to DeleteBeforeReplace = true
	// (Terraform's default: delete old, then create new)
	if !req.DeleteBeforeReplaceDef {
		t.Error("expected DeleteBeforeReplaceDef=true when create_before_destroy is explicitly set")
	}
	if !req.DeleteBeforeReplace {
		t.Error("expected DeleteBeforeReplace=true from create_before_destroy=false")
	}
}

func TestEngine_VariableFromConfig(t *testing.T) {
	t.Parallel()

	src := []byte(`
variable "region" {
  type    = string
  default = "us-east-1"
}

output "region_value" {
  value = var.region
}
`)

	p := parser.NewParser()
	config, diags := p.ParseSource("test.hcl", src)
	if diags.HasErrors() {
		t.Fatalf("parse error: %s", diags.Error())
	}

	mock := &testutil.MockResourceMonitor{}
	engine := run.NewEngine(config, &run.EngineOptions{
		ProjectName:     "test-project",
		StackName:       "dev",
		ResourceMonitor: mock,
		WorkDir:         t.TempDir(),
		RootDir:         t.TempDir(),
		SchemaLoader: schemaloader.New(t, schema.PackageSpec{
			Name: "aws",
			Resources: map[string]schema.ResourceSpec{
				"aws:index:Instance": {
					InputProperties: map[string]schema.PropertySpec{
						"ami": {TypeSpec: schema.TypeSpec{Type: "string"}},
					},
					ObjectTypeSpec: schema.ObjectTypeSpec{
						Properties: map[string]schema.PropertySpec{
							"ami": {TypeSpec: schema.TypeSpec{Type: "string"}},
						},
					},
				},
			},
		}),
		Config: map[string]string{
			"test-project:region": "us-west-2",
		},
	})

	err := engine.Run(t.Context())
	if err != nil {
		t.Fatalf("run error: %v", err)
	}

	// Check the stack outputs - region should be us-west-2 from config, not default
	if mock.StackOutputs.Len() == 0 {
		t.Fatal("expected stack outputs")
	}
	regionOutput, ok := mock.StackOutputs.GetOk("region_value")
	if !ok {
		t.Fatal("expected region_value output")
	}
	if regionOutput.AsString() != "us-west-2" {
		t.Errorf("expected region_value=%q from config, got %q", "us-west-2", regionOutput.AsString())
	}
}

func TestEngine_VariableFromEnv(t *testing.T) {
	src := []byte(`
variable "region" {
  type    = string
  default = "us-east-1"
}

output "region_value" {
  value = var.region
}
`)

	p := parser.NewParser()
	config, diags := p.ParseSource("test.hcl", src)
	if diags.HasErrors() {
		t.Fatalf("parse error: %s", diags.Error())
	}

	// Set environment variable
	t.Setenv("TF_VAR_region", "eu-west-1")

	mock := &testutil.MockResourceMonitor{}
	engine := run.NewEngine(config, &run.EngineOptions{
		ProjectName:     "test-project",
		StackName:       "dev",
		ResourceMonitor: mock,
		WorkDir:         t.TempDir(),
		RootDir:         t.TempDir(),
		SchemaLoader: schemaloader.New(t, schema.PackageSpec{
			Name: "aws",
			Resources: map[string]schema.ResourceSpec{
				"aws:index:Instance": {
					InputProperties: map[string]schema.PropertySpec{
						"ami": {TypeSpec: schema.TypeSpec{Type: "string"}},
					},
					ObjectTypeSpec: schema.ObjectTypeSpec{
						Properties: map[string]schema.PropertySpec{
							"ami": {TypeSpec: schema.TypeSpec{Type: "string"}},
						},
					},
				},
			},
		}),
		Config: map[string]string{
			"test-project:region": "us-west-2", // This should be ignored
		},
	})

	err := engine.Run(t.Context())
	if err != nil {
		t.Fatalf("run error: %v", err)
	}

	// Check the stack outputs - region should be eu-west-1 from env (highest priority)
	if mock.StackOutputs.Len() == 0 {
		t.Fatal("expected stack outputs")
	}
	regionOutput, ok := mock.StackOutputs.GetOk("region_value")
	if !ok {
		t.Fatal("expected region_value output")
	}
	if regionOutput.AsString() != "eu-west-1" {
		t.Errorf("expected region_value=%q from env, got %q", "eu-west-1", regionOutput.AsString())
	}
}

func TestEngine_VariableRequired(t *testing.T) {
	t.Parallel()

	// A variable declared without a default is required, regardless of its
	// `nullable` setting. (The `nullable` attribute only governs whether a
	// provided value may be the null literal.)
	tests := []struct {
		name string
		src  string
	}{
		{
			name: "no_nullable_attribute",
			src: `
variable "required_var" {
  type = string
}
`,
		},
		{
			name: "nullable_true",
			src: `
variable "required_var" {
  type     = string
  nullable = true
}
`,
		},
		{
			name: "nullable_false",
			src: `
variable "required_var" {
  type     = string
  nullable = false
}
`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			p := parser.NewParser()
			config, diags := p.ParseSource("test.hcl", []byte(tc.src))
			require.False(t, diags.HasErrors(), "parse error: %s", diags.Error())

			mock := &testutil.MockResourceMonitor{}
			engine := run.NewEngine(config, &run.EngineOptions{
				ProjectName:     "test-project",
				StackName:       "dev",
				ResourceMonitor: mock,
				WorkDir:         t.TempDir(),
				RootDir:         t.TempDir(),
				SchemaLoader:    schemaloader.New(t, schema.PackageSpec{Name: "empty"}),
			})

			err := engine.Run(t.Context())
			assert.EqualError(t, err, `variable "required_var" is required but no value was provided. `+
				`Set it with TF_VAR_required_var environment variable or Pulumi config: `+
				`pulumi config set required_var <value>`+"\ncontext canceled")
		})
	}
}

func TestEngine_VariableValidationPass(t *testing.T) {
	t.Parallel()

	src := []byte(`
variable "instance_type" {
  type    = string
  default = "t3.micro"

  validation {
    condition     = startswith(var.instance_type, "t3.")
    error_message = "Must be a t3 instance type."
  }
}

output "instance_type" {
  value = var.instance_type
}
`)

	p := parser.NewParser()
	config, diags := p.ParseSource("test.hcl", src)
	if diags.HasErrors() {
		t.Fatalf("parse error: %s", diags.Error())
	}

	mock := &testutil.MockResourceMonitor{}
	engine := run.NewEngine(config, &run.EngineOptions{
		ProjectName:     "test-project",
		StackName:       "dev",
		ResourceMonitor: mock,
		WorkDir:         t.TempDir(),
		RootDir:         t.TempDir(),
		SchemaLoader: schemaloader.New(t, schema.PackageSpec{
			Name: "aws",
			Resources: map[string]schema.ResourceSpec{
				"aws:index:Instance": {
					InputProperties: map[string]schema.PropertySpec{
						"ami": {TypeSpec: schema.TypeSpec{Type: "string"}},
					},
					ObjectTypeSpec: schema.ObjectTypeSpec{
						Properties: map[string]schema.PropertySpec{
							"ami": {TypeSpec: schema.TypeSpec{Type: "string"}},
						},
					},
				},
			},
		}),
	})

	err := engine.Run(t.Context())
	if err != nil {
		t.Fatalf("run error: %v", err)
	}

	// Should pass validation
	output, ok := mock.StackOutputs.GetOk("instance_type")
	if !ok {
		t.Fatal("expected instance_type output")
	}
	if output.AsString() != "t3.micro" {
		t.Errorf("expected t3.micro, got %q", output.AsString())
	}
}

func TestEngine_VariableValidationFail(t *testing.T) {
	t.Parallel()

	src := []byte(`
variable "instance_type" {
  type    = string
  default = "m5.large"

  validation {
    condition     = startswith(var.instance_type, "t3.")
    error_message = "Must be a t3 instance type."
  }
}
`)

	p := parser.NewParser()
	config, diags := p.ParseSource("test.hcl", src)
	if diags.HasErrors() {
		t.Fatalf("parse error: %s", diags.Error())
	}

	mock := &testutil.MockResourceMonitor{}
	engine := run.NewEngine(config, &run.EngineOptions{
		ProjectName:     "test-project",
		StackName:       "dev",
		ResourceMonitor: mock,
		WorkDir:         t.TempDir(),
		RootDir:         t.TempDir(),
		SchemaLoader: schemaloader.New(t, schema.PackageSpec{
			Name: "aws",
			Resources: map[string]schema.ResourceSpec{
				"aws:index:Instance": {
					InputProperties: map[string]schema.PropertySpec{
						"ami": {TypeSpec: schema.TypeSpec{Type: "string"}},
					},
					ObjectTypeSpec: schema.ObjectTypeSpec{
						Properties: map[string]schema.PropertySpec{
							"ami": {TypeSpec: schema.TypeSpec{Type: "string"}},
						},
					},
				},
			},
		}),
	})

	err := engine.Run(t.Context())

	// Should error because validation fails
	if err == nil {
		t.Fatal("expected error for validation failure")
	}
	if !strings.Contains(err.Error(), "Must be a t3 instance type") {
		t.Errorf("expected error message from validation, got: %v", err)
	}
}

func TestEngine_Precondition(t *testing.T) {
	t.Parallel()

	// Precondition checks startswith on a variable that feeds into the resource
	// input. This validates that preconditions can reference resource inputs and
	// that they run before RegisterResource.
	hclTemplate := `
variable "field_value" {
  type    = string
  default = "%s"
}

resource "test_resource" "res" {
  field = var.field_value

  lifecycle {
    precondition {
      condition     = startswith(var.field_value, "valid-")
      error_message = "Field must start with 'valid-'."
    }
  }
}
`

	runWithField := func(t *testing.T, value string) (*testutil.MockResourceMonitor, error) {
		t.Helper()
		src := fmt.Appendf(nil, hclTemplate, value)
		p := parser.NewParser()
		config, diags := p.ParseSource("test.hcl", src)
		require.False(t, diags.HasErrors(), diags.Error())

		mock := &testutil.MockResourceMonitor{}
		engine := run.NewEngine(config, &run.EngineOptions{
			ProjectName:     "test-project",
			StackName:       "dev",
			ResourceMonitor: mock,
			WorkDir:         t.TempDir(),
			RootDir:         t.TempDir(),
			SchemaLoader:    schemaloader.New(t, testSchema()),
		})
		return mock, engine.Run(t.Context())
	}

	t.Run("true condition", func(t *testing.T) {
		t.Parallel()
		mock, err := runWithField(t, "valid-value")
		require.NoError(t, err)
		require.True(t, hasRegisteredResource(mock, "test:index:Resource"),
			"expected resource to be registered when precondition passes")
	})

	t.Run("false condition", func(t *testing.T) {
		t.Parallel()
		mock, err := runWithField(t, "bad")
		require.ErrorContains(t, err, "Field must start with 'valid-'.")
		require.False(t, hasRegisteredResource(mock, "test:index:Resource"),
			"resource must not be registered when precondition fails")
	})
}

func TestEngine_Postcondition(t *testing.T) {
	t.Parallel()

	// Postcondition checks self.field against a known value. The mock echoes
	// inputs as outputs, so self.field == the input value.
	// A failing postcondition does NOT undo the resource creation — the resource
	// is already registered with the engine. It only fails the current deploy.
	hclTemplate := `
resource "test_resource" "res" {
  field = "%s"

  lifecycle {
    postcondition {
      condition     = self.field == "expected"
      error_message = "Field must be 'expected'."
    }
  }
}
`

	runWithField := func(t *testing.T, value string) (*testutil.MockResourceMonitor, error) {
		t.Helper()
		src := fmt.Appendf(nil, hclTemplate, value)
		p := parser.NewParser()
		config, diags := p.ParseSource("test.hcl", src)
		require.False(t, diags.HasErrors(), diags.Error())

		mock := &testutil.MockResourceMonitor{}
		engine := run.NewEngine(config, &run.EngineOptions{
			ProjectName:     "test-project",
			StackName:       "dev",
			ResourceMonitor: mock,
			WorkDir:         t.TempDir(),
			RootDir:         t.TempDir(),
			SchemaLoader:    schemaloader.New(t, testSchema()),
		})
		return mock, engine.Run(t.Context())
	}

	t.Run("true condition", func(t *testing.T) {
		t.Parallel()
		mock, err := runWithField(t, "expected")
		require.NoError(t, err)
		require.True(t, hasRegisteredResource(mock, "test:index:Resource"),
			"expected resource to be registered when postcondition passes")
	})

	t.Run("false condition", func(t *testing.T) {
		t.Parallel()
		mock, err := runWithField(t, "wrong")
		require.ErrorContains(t, err, "Field must be 'expected'.")
		// The resource IS registered even though postcondition failed — postconditions
		// run after resource creation and only fail the deploy, they don't undo the
		// resource registration.
		require.True(t, hasRegisteredResource(mock, "test:index:Resource"),
			"resource should still be registered even when postcondition fails")
	})
}

func TestEngine_LocalExecProvisioner(t *testing.T) {
	t.Parallel()

	src := []byte(`
resource "aws_instance" "web" {
  ami = "ami-12345"

  provisioner "local-exec" {
    command = "echo 'Hello World'"
    working_dir = "/tmp"
  }
}
`)

	p := parser.NewParser()
	config, diags := p.ParseSource("test.hcl", src)
	if diags.HasErrors() {
		t.Fatalf("parse error: %s", diags.Error())
	}

	mock := &testutil.MockResourceMonitor{}
	engine := run.NewEngine(config, &run.EngineOptions{
		ProjectName:     "test-project",
		StackName:       "dev",
		ResourceMonitor: mock,
		WorkDir:         t.TempDir(),
		RootDir:         t.TempDir(),
		SchemaLoader: schemaloader.New(t, schema.PackageSpec{
			Name: "aws",
			Resources: map[string]schema.ResourceSpec{
				"aws:index:Instance": {
					InputProperties: map[string]schema.PropertySpec{
						"ami": {TypeSpec: schema.TypeSpec{Type: "string"}},
					},
					ObjectTypeSpec: schema.ObjectTypeSpec{
						Properties: map[string]schema.PropertySpec{
							"ami": {TypeSpec: schema.TypeSpec{Type: "string"}},
						},
					},
				},
			},
		}),
	})

	err := engine.Run(t.Context())

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have registered: stack + resource + provisioner
	if len(mock.RegisteredResources) < 3 {
		t.Fatalf("expected at least 3 registered resources, got %d", len(mock.RegisteredResources))
	}

	// Find the provisioner resource
	var provisionerReq *run.RegisterResourceRequest
	for i := range mock.RegisteredResources {
		if mock.RegisteredResources[i].Type == "command:local:Command" {
			provisionerReq = &mock.RegisteredResources[i]
			break
		}
	}

	require.NotNil(t, provisionerReq, "expected command:local:Command provisioner to be registered")

	// Check that the command was mapped to create
	if create, ok := provisionerReq.Inputs.GetOk("create"); ok {
		if create.AsString() != "echo 'Hello World'" {
			t.Errorf("expected create command 'echo 'Hello World'', got %s", create.AsString())
		}
	} else {
		t.Error("expected 'create' input to be set")
	}

	// Check working_dir was mapped to dir
	if dir, ok := provisionerReq.Inputs.GetOk("dir"); ok {
		if dir.AsString() != "/tmp" {
			t.Errorf("expected dir '/tmp', got %s", dir.AsString())
		}
	} else {
		t.Error("expected 'dir' input to be set")
	}
}

func TestEngine_MultipleProvisioners(t *testing.T) {
	t.Parallel()

	src := []byte(`
resource "aws_instance" "web" {
  ami = "ami-12345"

  provisioner "local-exec" {
    command = "echo 'First'"
  }

  provisioner "local-exec" {
    command = "echo 'Second'"
  }
}
`)

	p := parser.NewParser()
	config, diags := p.ParseSource("test.hcl", src)
	if diags.HasErrors() {
		t.Fatalf("parse error: %s", diags.Error())
	}

	mock := &testutil.MockResourceMonitor{}
	engine := run.NewEngine(config, &run.EngineOptions{
		ProjectName:     "test-project",
		StackName:       "dev",
		ResourceMonitor: mock,
		WorkDir:         t.TempDir(),
		RootDir:         t.TempDir(),
		SchemaLoader: schemaloader.New(t, schema.PackageSpec{
			Name: "aws",
			Resources: map[string]schema.ResourceSpec{
				"aws:index:Instance": {
					InputProperties: map[string]schema.PropertySpec{
						"ami": {TypeSpec: schema.TypeSpec{Type: "string"}},
					},
					ObjectTypeSpec: schema.ObjectTypeSpec{
						Properties: map[string]schema.PropertySpec{
							"ami": {TypeSpec: schema.TypeSpec{Type: "string"}},
						},
					},
				},
			},
		}),
	})

	err := engine.Run(t.Context())

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Count provisioner resources
	var provisionerCount int
	for _, r := range mock.RegisteredResources {
		if r.Type == "command:local:Command" {
			provisionerCount++
		}
	}

	if provisionerCount != 2 {
		t.Fatalf("expected 2 provisioner resources, got %d", provisionerCount)
	}
}

func TestEngine_ProvisionerWithSelf(t *testing.T) {
	t.Parallel()

	src := []byte(`
resource "aws_instance" "web" {
  ami = "ami-12345"

  provisioner "local-exec" {
    command = "echo ${self.id}"
  }
}
`)

	p := parser.NewParser()
	config, diags := p.ParseSource("test.hcl", src)
	if diags.HasErrors() {
		t.Fatalf("parse error: %s", diags.Error())
	}

	mock := &testutil.MockResourceMonitor{}
	engine := run.NewEngine(config, &run.EngineOptions{
		ProjectName:     "test-project",
		StackName:       "dev",
		ResourceMonitor: mock,
		WorkDir:         t.TempDir(),
		RootDir:         t.TempDir(),
		SchemaLoader: schemaloader.New(t, schema.PackageSpec{
			Name: "aws",
			Resources: map[string]schema.ResourceSpec{
				"aws:index:Instance": {
					InputProperties: map[string]schema.PropertySpec{
						"ami": {TypeSpec: schema.TypeSpec{Type: "string"}},
					},
					ObjectTypeSpec: schema.ObjectTypeSpec{
						Properties: map[string]schema.PropertySpec{
							"ami": {TypeSpec: schema.TypeSpec{Type: "string"}},
						},
					},
				},
			},
		}),
	})

	err := engine.Run(t.Context())

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Find the provisioner resource
	var provisionerReq *run.RegisterResourceRequest
	for i := range mock.RegisteredResources {
		if mock.RegisteredResources[i].Type == "command:local:Command" {
			provisionerReq = &mock.RegisteredResources[i]
			break
		}
	}

	require.NotNil(t, provisionerReq, "expected command:local:Command provisioner to be registered")

	// Check that self.id was resolved
	if create, ok := provisionerReq.Inputs.GetOk("create"); ok {
		// The id should be set to the resource name + "-id" by the mock
		if !strings.Contains(create.AsString(), "web-id") {
			t.Errorf("expected self.id to be resolved, got: %s", create.AsString())
		}
	} else {
		t.Error("expected 'create' input to be set")
	}
}

func TestEngine_SimpleModule(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	// Create module directory
	moduleDir := tmpDir + "/modules/vpc"
	if err := os.MkdirAll(moduleDir, 0755); err != nil {
		t.Fatalf("creating module dir: %v", err)
	}

	// Write module files
	moduleMain := `
variable "name" {
  type = string
}

variable "cidr" {
  type    = string
  default = "10.0.0.0/16"
}

resource "aws_vpc" "main" {
  cidr_block = var.cidr
  tags = {
    Name = var.name
  }
}

output "vpc_id" {
  value = aws_vpc.main.id
}

output "cidr_block" {
  value = var.cidr
}
`
	if err := os.WriteFile(moduleDir+"/main.hcl", []byte(moduleMain), 0644); err != nil {
		t.Fatalf("writing module file: %v", err)
	}

	// Write root configuration
	rootMain := `
module "vpc" {
  source = "./modules/vpc"
  name   = "my-vpc"
}

output "vpc_id" {
  value = module.vpc.vpc_id
}
`
	if err := os.WriteFile(tmpDir+"/main.hcl", []byte(rootMain), 0644); err != nil {
		t.Fatalf("writing root file: %v", err)
	}

	// Parse the root configuration
	p := parser.NewParser()
	config, diags := p.ParseDirectory(tmpDir)
	if diags.HasErrors() {
		t.Fatalf("parse error: %s", diags.Error())
	}

	mock := &testutil.MockResourceMonitor{}
	engine := run.NewEngine(config, &run.EngineOptions{
		ProjectName:     "test-project",
		StackName:       "dev",
		ResourceMonitor: mock,
		WorkDir:         tmpDir,
		RootDir:         tmpDir,
		SchemaLoader: schemaloader.New(t, schema.PackageSpec{
			Name: "aws",
			Resources: map[string]schema.ResourceSpec{
				"aws:index:Vpc": {
					InputProperties: map[string]schema.PropertySpec{
						"cidrBlock": {TypeSpec: schema.TypeSpec{Type: "string"}},
						"tags": {TypeSpec: schema.TypeSpec{
							Type:                 "object",
							AdditionalProperties: &schema.TypeSpec{Type: "string"},
						}},
					},
					ObjectTypeSpec: schema.ObjectTypeSpec{
						Properties: map[string]schema.PropertySpec{
							"cidrBlock": {TypeSpec: schema.TypeSpec{Type: "string"}},
							"tags": {TypeSpec: schema.TypeSpec{
								Type:                 "object",
								AdditionalProperties: &schema.TypeSpec{Type: "string"},
							}},
						},
					},
				},
			},
		}),
	})

	err := engine.Run(t.Context())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have registered: stack + module component + vpc resource
	if len(mock.RegisteredResources) < 3 {
		t.Fatalf("expected at least 3 registered resources, got %d", len(mock.RegisteredResources))
	}

	// Find the module component (type: components:index:{TypeName})
	var moduleComponent *run.RegisterResourceRequest
	for i := range mock.RegisteredResources {
		if strings.HasPrefix(mock.RegisteredResources[i].Type, "components:index:") {
			moduleComponent = &mock.RegisteredResources[i]
			break
		}
	}

	require.NotNil(t, moduleComponent, "expected module component to be registered")

	// Verify the component type token format
	expectedType := "components:index:Vpc"
	if moduleComponent.Type != expectedType {
		t.Errorf("expected module type %q, got %q", expectedType, moduleComponent.Type)
	}

	// Check that the module name is the logical module name (without "module." prefix)
	assert.Equal(t, "vpc", moduleComponent.Name)

	// Find the VPC resource
	var vpcResource *run.RegisterResourceRequest
	for i := range mock.RegisteredResources {
		if mock.RegisteredResources[i].Type == "aws:index:Vpc" {
			vpcResource = &mock.RegisteredResources[i]
			break
		}
	}

	require.NotNil(t, vpcResource, "expected aws:index:Vpc resource to be registered")

	// Check that the VPC has the correct cidr_block
	if cidr, ok := vpcResource.Inputs.GetOk("cidrBlock"); ok {
		if cidr.AsString() != "10.0.0.0/16" {
			t.Errorf("expected cidr_block '10.0.0.0/16', got %s", cidr.AsString())
		}
	} else {
		t.Error("expected 'cidr_block' input to be set")
	}
}

// TestEngine_ModuleNameWithDot verifies that module names containing a "."
// are preserved verbatim when computing the component's logical resource name,
// rather than being split apart by dot-based key parsing.
func TestEngine_ModuleNameWithDot(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	moduleDir := tmpDir + "/modules/vpc"
	require.NoError(t, os.MkdirAll(moduleDir, 0755))

	moduleMain := `
resource "aws_vpc" "main" {
  cidr_block = "10.0.0.0/16"
}
`
	require.NoError(t, os.WriteFile(moduleDir+"/main.hcl", []byte(moduleMain), 0644))

	rootMain := `
module "vpc.primary" {
  source = "./modules/vpc"
}
`
	require.NoError(t, os.WriteFile(tmpDir+"/main.hcl", []byte(rootMain), 0644))

	p := parser.NewParser()
	config, diags := p.ParseDirectory(tmpDir)
	require.False(t, diags.HasErrors(), "parse error: %s", diags.Error())

	mock := &testutil.MockResourceMonitor{}
	engine := run.NewEngine(config, &run.EngineOptions{
		ProjectName:     "test-project",
		StackName:       "dev",
		ResourceMonitor: mock,
		WorkDir:         tmpDir,
		RootDir:         tmpDir,
		SchemaLoader: schemaloader.New(t, schema.PackageSpec{
			Name: "aws",
			Resources: map[string]schema.ResourceSpec{
				"aws:index:Vpc": {
					InputProperties: map[string]schema.PropertySpec{
						"cidrBlock": {TypeSpec: schema.TypeSpec{Type: "string"}},
					},
					ObjectTypeSpec: schema.ObjectTypeSpec{
						Properties: map[string]schema.PropertySpec{
							"cidrBlock": {TypeSpec: schema.TypeSpec{Type: "string"}},
						},
					},
				},
			},
		}),
	})

	require.NoError(t, engine.Run(t.Context()))

	var moduleComponent *run.RegisterResourceRequest
	for i := range mock.RegisteredResources {
		if strings.HasPrefix(mock.RegisteredResources[i].Type, "components:index:") {
			moduleComponent = &mock.RegisteredResources[i]
			break
		}
	}
	require.NotNil(t, moduleComponent, "expected module component to be registered")

	assert.Equal(t, "vpc.primary", moduleComponent.Name)

	var vpcResource *run.RegisterResourceRequest
	for i := range mock.RegisteredResources {
		if mock.RegisteredResources[i].Type == "aws:index:Vpc" {
			vpcResource = &mock.RegisteredResources[i]
			break
		}
	}
	require.NotNil(t, vpcResource, "expected aws:index:Vpc resource to be registered")
	assert.Equal(t, "vpc.primary-main", vpcResource.Name)
}

// runSensitiveMetaArgTest is a shared driver for the four "sensitive value
// rejected by count/for_each" tests. It writes the given root.hcl into a
// temp dir, runs the engine, and returns the resulting error.
func runSensitiveMetaArgTest(t *testing.T, rootMain string) error {
	t.Helper()
	tmpDir := t.TempDir()

	moduleDir := tmpDir + "/modules/m"
	require.NoError(t, os.MkdirAll(moduleDir, 0755))
	require.NoError(t, os.WriteFile(moduleDir+"/main.hcl", []byte(`
output "name" { value = "leaf" }
`), 0644))
	require.NoError(t, os.WriteFile(tmpDir+"/main.hcl", []byte(rootMain), 0644))

	p := parser.NewParser()
	config, diags := p.ParseDirectory(tmpDir)
	require.False(t, diags.HasErrors(), "parse error: %s", diags.Error())

	mock := &testutil.MockResourceMonitor{}
	engine := run.NewEngine(config, &run.EngineOptions{
		ProjectName:     "test-project",
		StackName:       "dev",
		ResourceMonitor: mock,
		WorkDir:         tmpDir,
		RootDir:         tmpDir,
		SchemaLoader: schemaloader.New(t, schema.PackageSpec{
			Name: "aws",
			Resources: map[string]schema.ResourceSpec{
				"aws:index:Vpc": {
					InputProperties: map[string]schema.PropertySpec{
						"cidrBlock": {TypeSpec: schema.TypeSpec{Type: "string"}},
					},
					ObjectTypeSpec: schema.ObjectTypeSpec{
						Properties: map[string]schema.PropertySpec{
							"cidrBlock": {TypeSpec: schema.TypeSpec{Type: "string"}},
						},
					},
				},
			},
		}),
	})

	return engine.Run(t.Context())
}

// assertSensitiveArgumentError asserts that err is the specific diagnostic
// emitted when a sensitive value is supplied to a meta-argument.
func assertSensitiveArgumentError(t *testing.T, argName string, err error) {
	t.Helper()
	require.Error(t, err)
	msg := err.Error()
	assert.Contains(t, msg, "Invalid "+argName+" argument")
	assert.Contains(t, msg,
		"Sensitive values, or values derived from sensitive values, "+
			"cannot be used as "+argName+" arguments.")
}

// TestEngine_ModuleForEachSensitive verifies that supplying a sensitive
// value to a module's `for_each` produces a clean Terraform-style error
// rather than expanding the module with a leaked secret in the URN.
func TestEngine_ModuleForEachSensitive(t *testing.T) {
	t.Parallel()

	err := runSensitiveMetaArgTest(t, `
module "m" {
  source   = "./modules/m"
  for_each = sensitive(toset(["a", "b"]))
}
`)
	assertSensitiveArgumentError(t, "for_each", err)
}

// TestEngine_ModuleCountSensitive verifies the same behavior for module
// `count`.
func TestEngine_ModuleCountSensitive(t *testing.T) {
	t.Parallel()

	err := runSensitiveMetaArgTest(t, `
module "m" {
  source = "./modules/m"
  count  = sensitive(2)
}
`)
	assertSensitiveArgumentError(t, "count", err)
}

// TestEngine_ResourceForEachSensitive verifies the same behavior for a
// resource's `for_each` (handled via [eval.EvaluateForEach]).
func TestEngine_ResourceForEachSensitive(t *testing.T) {
	t.Parallel()

	err := runSensitiveMetaArgTest(t, `
resource "aws_vpc" "v" {
  for_each   = sensitive(toset(["a"]))
  cidr_block = "10.0.0.0/16"
}
`)
	assertSensitiveArgumentError(t, "for_each", err)
}

// TestEngine_ResourceCountSensitive verifies the same behavior for a
// resource's `count`.
func TestEngine_ResourceCountSensitive(t *testing.T) {
	t.Parallel()

	err := runSensitiveMetaArgTest(t, `
resource "aws_vpc" "v" {
  count      = sensitive(1)
  cidr_block = "10.0.0.0/16"
}
`)
	assertSensitiveArgumentError(t, "count", err)
}

// TestEngine_ModuleOutputRace verifies that concurrent processing of multiple
// module outputs does not trigger a data race on moduleInstance.Outputs.
// This is a regression test for https://github.com/pulumi-labs/pulumi-hcl/issues/60.
func TestEngine_ModuleOutputRace(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	// Create a module with many outputs to maximize scheduling contention.
	moduleDir := tmpDir + "/modules/multi"
	if err := os.MkdirAll(moduleDir, 0755); err != nil {
		t.Fatalf("creating module dir: %v", err)
	}

	moduleMain := `
variable "input" {
  type = string
}

output "out_a" { value = var.input }
output "out_b" { value = var.input }
output "out_c" { value = var.input }
output "out_d" { value = var.input }
output "out_e" { value = var.input }
output "out_f" { value = var.input }
output "out_g" { value = var.input }
output "out_h" { value = var.input }
`
	if err := os.WriteFile(moduleDir+"/main.hcl", []byte(moduleMain), 0644); err != nil {
		t.Fatalf("writing module file: %v", err)
	}

	rootMain := `
module "multi" {
  source = "./modules/multi"
  input  = "hello"
}
`
	if err := os.WriteFile(tmpDir+"/main.hcl", []byte(rootMain), 0644); err != nil {
		t.Fatalf("writing root file: %v", err)
	}

	p := parser.NewParser()
	config, diags := p.ParseDirectory(tmpDir)
	if diags.HasErrors() {
		t.Fatalf("parse error: %s", diags.Error())
	}

	mock := &testutil.MockResourceMonitor{}
	engine := run.NewEngine(config, &run.EngineOptions{
		ProjectName:     "test-project",
		StackName:       "dev",
		ResourceMonitor: mock,
		WorkDir:         tmpDir,
		RootDir:         tmpDir,
		SchemaLoader:    schemaloader.New(t, schema.PackageSpec{Name: "empty"}),
	})

	// Under -race this would fail before the fix due to concurrent map writes
	// in processModuleOutput.
	if err := engine.Run(t.Context()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestEngine_Timeouts(t *testing.T) {
	t.Parallel()

	src := []byte(`
resource "aws_instance" "web" {
  ami           = "ami-12345"
  instance_type = "t3.micro"

  timeouts {
    create = "60m"
    update = "30m"
    delete = "2h"
  }
}
`)

	p := parser.NewParser()
	config, diags := p.ParseSource("test.hcl", src)
	if diags.HasErrors() {
		t.Fatalf("parse error: %s", diags.Error())
	}

	mock := &testutil.MockResourceMonitor{}
	engine := run.NewEngine(config, &run.EngineOptions{
		ProjectName:     "test-project",
		StackName:       "dev",
		ResourceMonitor: mock,
		WorkDir:         t.TempDir(),
		RootDir:         t.TempDir(),
		SchemaLoader: schemaloader.New(t, schema.PackageSpec{
			Name: "aws",
			Resources: map[string]schema.ResourceSpec{
				"aws:index:Instance": {
					InputProperties: map[string]schema.PropertySpec{
						"ami":          {TypeSpec: schema.TypeSpec{Type: "string"}},
						"instanceType": {TypeSpec: schema.TypeSpec{Type: "string"}},
					},
					ObjectTypeSpec: schema.ObjectTypeSpec{
						Properties: map[string]schema.PropertySpec{
							"ami":          {TypeSpec: schema.TypeSpec{Type: "string"}},
							"instanceType": {TypeSpec: schema.TypeSpec{Type: "string"}},
						},
					},
				},
			},
		}),
	})

	err := engine.Run(t.Context())
	if err != nil {
		t.Fatalf("run error: %v", err)
	}

	// Find the instance resource
	var instanceReq *run.RegisterResourceRequest
	for i := range mock.RegisteredResources {
		if mock.RegisteredResources[i].Type == "aws:index:Instance" {
			instanceReq = &mock.RegisteredResources[i]
			break
		}
	}

	require.NotNil(t, instanceReq, "expected aws:index:Instance resource to be registered")

	// Check that timeouts were set
	require.NotNil(t, instanceReq.CustomTimeouts, "expected CustomTimeouts to be set")

	// 60m = 3600 seconds
	if instanceReq.CustomTimeouts.Create != 3600 {
		t.Errorf("expected Create timeout 3600, got %f", instanceReq.CustomTimeouts.Create)
	}

	// 30m = 1800 seconds
	if instanceReq.CustomTimeouts.Update != 1800 {
		t.Errorf("expected Update timeout 1800, got %f", instanceReq.CustomTimeouts.Update)
	}

	// 2h = 7200 seconds
	if instanceReq.CustomTimeouts.Delete != 7200 {
		t.Errorf("expected Delete timeout 7200, got %f", instanceReq.CustomTimeouts.Delete)
	}
}

func TestEngine_MovedBlock(t *testing.T) {
	t.Parallel()

	src := []byte(`
moved {
  from = aws_instance.old_server
  to   = aws_instance.web
}

resource "aws_instance" "web" {
  ami           = "ami-12345"
  instance_type = "t3.micro"
}
`)

	p := parser.NewParser()
	config, diags := p.ParseSource("test.hcl", src)
	if diags.HasErrors() {
		t.Fatalf("parse error: %s", diags.Error())
	}

	mock := &testutil.MockResourceMonitor{}
	engine := run.NewEngine(config, &run.EngineOptions{
		ProjectName:     "test-project",
		StackName:       "dev",
		ResourceMonitor: mock,
		WorkDir:         t.TempDir(),
		RootDir:         t.TempDir(),
		SchemaLoader: schemaloader.New(t, schema.PackageSpec{
			Name: "aws",
			Resources: map[string]schema.ResourceSpec{
				"aws:index:Instance": {
					InputProperties: map[string]schema.PropertySpec{
						"ami":          {TypeSpec: schema.TypeSpec{Type: "string"}},
						"instanceType": {TypeSpec: schema.TypeSpec{Type: "string"}},
					},
					ObjectTypeSpec: schema.ObjectTypeSpec{
						Properties: map[string]schema.PropertySpec{
							"ami":          {TypeSpec: schema.TypeSpec{Type: "string"}},
							"instanceType": {TypeSpec: schema.TypeSpec{Type: "string"}},
						},
					},
				},
			},
		}),
	})

	err := engine.Run(t.Context())
	if err != nil {
		t.Fatalf("run error: %v", err)
	}

	// Find the instance resource
	var instanceReq *run.RegisterResourceRequest
	for i := range mock.RegisteredResources {
		if mock.RegisteredResources[i].Type == "aws:index:Instance" {
			instanceReq = &mock.RegisteredResources[i]
			break
		}
	}

	require.NotNil(t, instanceReq, "expected aws:index:Instance resource to be registered")

	// Check that aliases include the old resource address
	if len(instanceReq.Aliases) == 0 {
		t.Fatal("expected Aliases to contain the moved 'from' address")
	}

	found := false
	for _, alias := range instanceReq.Aliases {
		if alias.Spec != nil && alias.Spec.Name == "aws_instance.old_server" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected alias with name 'aws_instance.old_server', got %v", instanceReq.Aliases)
	}
}

func TestEngine_ImportBlock(t *testing.T) {
	t.Parallel()

	src := []byte(`
import {
  to = aws_instance.imported
  id = "i-1234567890abcdef0"
}

resource "aws_instance" "imported" {
  ami           = "ami-12345"
  instance_type = "t3.micro"
}
`)

	p := parser.NewParser()
	config, diags := p.ParseSource("test.hcl", src)
	if diags.HasErrors() {
		t.Fatalf("parse error: %s", diags.Error())
	}

	mock := &testutil.MockResourceMonitor{}
	engine := run.NewEngine(config, &run.EngineOptions{
		ProjectName:     "test-project",
		StackName:       "dev",
		ResourceMonitor: mock,
		WorkDir:         t.TempDir(),
		RootDir:         t.TempDir(),
		SchemaLoader: schemaloader.New(t, schema.PackageSpec{
			Name: "aws",
			Resources: map[string]schema.ResourceSpec{
				"aws:index:Instance": {
					InputProperties: map[string]schema.PropertySpec{
						"ami":          {TypeSpec: schema.TypeSpec{Type: "string"}},
						"instanceType": {TypeSpec: schema.TypeSpec{Type: "string"}},
					},
					ObjectTypeSpec: schema.ObjectTypeSpec{
						Properties: map[string]schema.PropertySpec{
							"ami":          {TypeSpec: schema.TypeSpec{Type: "string"}},
							"instanceType": {TypeSpec: schema.TypeSpec{Type: "string"}},
						},
					},
				},
			},
		}),
	})

	err := engine.Run(t.Context())
	if err != nil {
		t.Fatalf("run error: %v", err)
	}

	// Find the instance resource
	var instanceReq *run.RegisterResourceRequest
	for i := range mock.RegisteredResources {
		if mock.RegisteredResources[i].Type == "aws:index:Instance" {
			instanceReq = &mock.RegisteredResources[i]
			break
		}
	}

	require.NotNil(t, instanceReq, "expected aws:index:Instance resource to be registered")

	// Check that ImportId was set
	if instanceReq.ImportId != "i-1234567890abcdef0" {
		t.Errorf("expected ImportId 'i-1234567890abcdef0', got %q", instanceReq.ImportId)
	}
}

// testSchema returns a minimal schema for a test_resource resource.
func testSchema() schema.PackageSpec {
	return schema.PackageSpec{
		Name: "test",
		Resources: map[string]schema.ResourceSpec{
			"test:index:Resource": {
				InputProperties: map[string]schema.PropertySpec{
					"field": {TypeSpec: schema.TypeSpec{Type: "string"}},
				},
				ObjectTypeSpec: schema.ObjectTypeSpec{
					Properties: map[string]schema.PropertySpec{
						"field": {TypeSpec: schema.TypeSpec{Type: "string"}},
					},
				},
			},
		},
	}
}

// hasRegisteredResource reports whether the mock has a registered resource with the given type.
func hasRegisteredResource(mock *testutil.MockResourceMonitor, typ string) bool {
	for _, r := range mock.RegisteredResources {
		if r.Type == typ {
			return true
		}
	}
	return false
}

func TestEngine_ReplaceTriggeredByErrors(t *testing.T) {
	t.Parallel()

	src := []byte(`
resource "test_resource" "res" {
  field = "value"

  lifecycle {
    replace_triggered_by = [test_resource.res.field]
  }
}
`)

	p := parser.NewParser()
	config, diags := p.ParseSource("test.hcl", src)
	if diags.HasErrors() {
		t.Fatalf("parse error: %s", diags.Error())
	}

	mock := &testutil.MockResourceMonitor{}
	engine := run.NewEngine(config, &run.EngineOptions{
		ProjectName:     "test-project",
		StackName:       "dev",
		ResourceMonitor: mock,
		WorkDir:         t.TempDir(),
		RootDir:         t.TempDir(),
		SchemaLoader:    schemaloader.New(t, testSchema()),
	})

	err := engine.Run(t.Context())
	require.ErrorContains(t, err, "replace_triggered_by")
	require.ErrorContains(t, err, "not supported")
}

// TestEngine_HetListOutputRoundTrip drives the engine against a mock provider whose
// resource output is a list of objects with a nested optional object populated in some
// elements and absent in others.
func TestEngine_HetListOutputRoundTrip(t *testing.T) {
	t.Parallel()

	src := []byte(`resource "test_het_list" "het" {}`)
	p := parser.NewParser()
	config, diags := p.ParseSource("test.hcl", src)
	require.False(t, diags.HasErrors(), diags.Error())

	hetListSchema := schema.PackageSpec{
		Name: "test",
		Types: map[string]schema.ComplexTypeSpec{
			"test:index:Token": {
				ObjectTypeSpec: schema.ObjectTypeSpec{
					Type: "object",
					Properties: map[string]schema.PropertySpec{
						"audience": {TypeSpec: schema.TypeSpec{Type: "string"}},
					},
				},
			},
			"test:index:Source": {
				ObjectTypeSpec: schema.ObjectTypeSpec{
					Type: "object",
					Properties: map[string]schema.PropertySpec{
						"token": {TypeSpec: schema.TypeSpec{Ref: "#/types/test:index:Token"}},
					},
				},
			},
		},
		Resources: map[string]schema.ResourceSpec{
			"test:index:HetList": {
				ObjectTypeSpec: schema.ObjectTypeSpec{
					Properties: map[string]schema.PropertySpec{
						"sources": {
							TypeSpec: schema.TypeSpec{
								Type:  "array",
								Items: &schema.TypeSpec{Ref: "#/types/test:index:Source"},
							},
						},
					},
				},
			},
		},
	}

	monitor := &testutil.MockResourceMonitor{
		RegisterResourceHandler: func(ctx context.Context, req run.RegisterResourceRequest) (*run.RegisterResourceResponse, error) {
			urn := "urn:pulumi:test::project::" + req.Type + "::" + req.Name
			if req.Type == "test:index:HetList" {
				return &run.RegisterResourceResponse{
					URN: urn,
					ID:  req.Name + "-id",
					Outputs: property.NewMap(map[string]property.Value{
						"sources": property.New([]property.Value{
							property.New(property.NewMap(map[string]property.Value{
								"token": property.New(property.NewMap(map[string]property.Value{})),
							})),
							property.New(property.NewMap(map[string]property.Value{})),
						}),
					}),
				}, nil
			}
			return &run.RegisterResourceResponse{
				URN:     urn,
				ID:      req.Name + "-id",
				Outputs: req.Inputs,
			}, nil
		},
	}
	engine := run.NewEngine(config, &run.EngineOptions{
		ProjectName:     "test-project",
		StackName:       "dev",
		ResourceMonitor: monitor,
		WorkDir:         t.TempDir(),
		RootDir:         t.TempDir(),
		SchemaLoader:    schemaloader.New(t, hetListSchema),
	})

	assert.NotPanics(t, func() {
		err := engine.Run(t.Context())
		require.NoError(t, err)
	})
}
