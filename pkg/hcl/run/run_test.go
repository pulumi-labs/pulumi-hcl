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
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/blang/semver"
	"github.com/pulumi/pulumi-language-hcl/pkg/hcl/packages"
	"github.com/pulumi/pulumi-language-hcl/pkg/hcl/parser"
	"github.com/pulumi/pulumi/pkg/v3/codegen/schema"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/stretchr/testify/require"
)

// mockResourceMonitor is a mock implementation of ResourceMonitor for testing.
type mockResourceMonitor struct {
	mu                  sync.Mutex
	registeredResources []RegisterResourceRequest
	invokedFunctions    []InvokeRequest
	stackOutputs        resource.PropertyMap
	stackURN            string
}

func (m *mockResourceMonitor) RegisterResource(ctx context.Context, req RegisterResourceRequest) (*RegisterResourceResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.registeredResources = append(m.registeredResources, req)
	urn := "urn:pulumi:test::project::" + req.Type + "::" + req.Name
	if req.Type == "pulumi:pulumi:Stack" {
		m.stackURN = urn
	}
	return &RegisterResourceResponse{
		URN:     urn,
		ID:      req.Name + "-id",
		Outputs: req.Inputs,
	}, nil
}

func (m *mockResourceMonitor) Invoke(ctx context.Context, req InvokeRequest) (*InvokeResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.invokedFunctions = append(m.invokedFunctions, req)
	return &InvokeResponse{
		Return: resource.PropertyMap{
			"id": resource.NewStringProperty("mock-id"),
		},
	}, nil
}

func (m *mockResourceMonitor) RegisterResourceOutputs(ctx context.Context, urn string, outputs resource.PropertyMap) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if urn == m.stackURN {
		m.stackOutputs = outputs
	}
	return nil
}

func (m *mockResourceMonitor) CheckPulumiVersion(ctx context.Context, versionRange string) error {
	return nil
}

var _ schema.ReferenceLoader = mockReferenceLoader{}

type mockReferenceLoader map[string]schema.Package

func (m mockReferenceLoader) LoadPackage(pkg string, version *semver.Version) (*schema.Package, error) {
	return m.LoadPackageV2(context.Background(), &schema.PackageDescriptor{
		Name:    pkg,
		Version: version,
	})
}

func (m mockReferenceLoader) LoadPackageV2(ctx context.Context, descriptor *schema.PackageDescriptor) (*schema.Package, error) {
	p, ok := m[descriptor.String()]
	if ok {
		return &p, nil
	}
	return nil, packages.ErrNotFound
}

func (m mockReferenceLoader) LoadPackageReference(pkg string, version *semver.Version) (schema.PackageReference, error) {
	return m.LoadPackageReferenceV2(context.Background(), &schema.PackageDescriptor{
		Name:    pkg,
		Version: version,
	})
}

func (m mockReferenceLoader) LoadPackageReferenceV2(ctx context.Context, descriptor *schema.PackageDescriptor) (schema.PackageReference, error) {
	p, ok := m[descriptor.String()]
	if ok {
		return p.Reference(), nil
	}
	fmt.Printf("Looking for %s\n", descriptor.String())
	for k := range m {
		fmt.Printf("Found: %s\n", k)
	}
	return nil, packages.ErrNotFound
}

func newMockReferenceLoader(t testing.TB, schemas ...schema.PackageSpec) schema.ReferenceLoader {
	loader := mockReferenceLoader{}
	for _, spec := range schemas {
		pkg, diag, err := schema.BindSpec(spec, loader, schema.ValidationOptions{})
		require.NoError(t, err)
		require.Len(t, diag, 0)
		d, err := pkg.Descriptor(t.Context())
		require.NoError(t, err)

		params := func() *schema.ParameterizationDescriptor {
			if d.Parameterization == nil {
				return nil
			}
			return &schema.ParameterizationDescriptor{
				Name:    d.Parameterization.Name,
				Version: d.Parameterization.Version,
				Value:   d.Parameterization.Value,
			}
		}
		loader[(&schema.PackageDescriptor{
			Name:             d.Name,
			Version:          d.Version,
			DownloadURL:      d.PluginDownloadURL,
			Parameterization: params(),
		}).String()] = *pkg
	}
	return loader
}

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

	mock := &mockResourceMonitor{}
	engine := NewEngine(config, &EngineOptions{
		ProjectName:     "test-project",
		StackName:       "dev",
		ResourceMonitor: mock,
		WorkDir:         t.TempDir(),
		RootDir:         t.TempDir(),
		SchemaLoader: newMockReferenceLoader(t, schema.PackageSpec{
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
	if len(mock.registeredResources) != 2 {
		t.Fatalf("expected 2 registered resources (stack + resource), got %d", len(mock.registeredResources))
	}

	// First resource should be the stack
	if mock.registeredResources[0].Type != "pulumi:pulumi:Stack" {
		t.Errorf("expected first resource to be stack, got %s", mock.registeredResources[0].Type)
	}

	req := mock.registeredResources[1]
	if req.Name != "web" {
		t.Errorf("expected resource name 'web', got %s", req.Name)
	}
	if req.Inputs["ami"].StringValue() != "ami-12345" {
		t.Errorf("expected ami 'ami-12345', got %v", req.Inputs["ami"])
	}
	if req.Inputs["instanceType"].StringValue() != "test" {
		t.Errorf("expected instanceType 'test', got %v", req.Inputs["instanceType"])
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

	mock := &mockResourceMonitor{}
	engine := NewEngine(config, &EngineOptions{
		ProjectName:     "test-project",
		StackName:       "dev",
		ResourceMonitor: mock,
		WorkDir:         t.TempDir(),
		RootDir:         t.TempDir(),
		SchemaLoader: newMockReferenceLoader(t, schema.PackageSpec{
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

	mock := &mockResourceMonitor{}
	engine := NewEngine(config, &EngineOptions{
		ProjectName:     "test-project",
		StackName:       "dev",
		ResourceMonitor: mock,
		WorkDir:         t.TempDir(),
		RootDir:         t.TempDir(),
		SchemaLoader: newMockReferenceLoader(t, schema.PackageSpec{
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
	if len(mock.registeredResources) != 3 {
		t.Fatalf("expected 3 registered resources (stack + 2 resources), got %d", len(mock.registeredResources))
	}

	// VPC should be registered first (after stack)
	if mock.registeredResources[1].Name != "main" {
		t.Errorf("expected main first, got %s", mock.registeredResources[1].Name)
	}

	// Subnet should be registered second
	if mock.registeredResources[2].Name != "main" {
		t.Errorf("expected main second, got %s", mock.registeredResources[2].Name)
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

	mock := &mockResourceMonitor{}
	engine := NewEngine(config, &EngineOptions{
		ProjectName:     "test-project",
		StackName:       "dev",
		ResourceMonitor: mock,
		WorkDir:         t.TempDir(),
		RootDir:         t.TempDir(),
		SchemaLoader: newMockReferenceLoader(t, schema.PackageSpec{
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
	if len(mock.registeredResources) != 3 {
		t.Fatalf("expected 3 registered resources (stack + 2 resources), got %d", len(mock.registeredResources))
	}

	// Bucket should be first (after stack)
	if mock.registeredResources[1].Name != "mybucket" {
		t.Errorf("expected bucket first, got %s", mock.registeredResources[1].Name)
	}

	// Instance should have depends_on set
	if len(mock.registeredResources[2].Dependencies) == 0 {
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

	mock := &mockResourceMonitor{}
	engine := NewEngine(config, &EngineOptions{
		ProjectName:     "test-project",
		StackName:       "dev",
		ResourceMonitor: mock,
		WorkDir:         t.TempDir(),
		RootDir:         t.TempDir(),
		SchemaLoader: newMockReferenceLoader(t, schema.PackageSpec{
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

	mock := &mockResourceMonitor{}
	engine := NewEngine(config, &EngineOptions{
		ProjectName:     "test-project",
		StackName:       "dev",
		ResourceMonitor: mock,
		WorkDir:         t.TempDir(),
		RootDir:         t.TempDir(),
		SchemaLoader: newMockReferenceLoader(t, schema.PackageSpec{
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

	if len(mock.registeredResources) != 2 {
		t.Fatalf("expected 2 registered resources (stack + resource), got %d", len(mock.registeredResources))
	}

	req := mock.registeredResources[1]

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

	mock := &mockResourceMonitor{}
	engine := NewEngine(config, &EngineOptions{
		ProjectName:     "test-project",
		StackName:       "dev",
		ResourceMonitor: mock,
		WorkDir:         t.TempDir(),
		RootDir:         t.TempDir(),
		SchemaLoader: newMockReferenceLoader(t, schema.PackageSpec{
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

	if len(mock.registeredResources) != 2 {
		t.Fatalf("expected 2 registered resources (stack + resource), got %d", len(mock.registeredResources))
	}

	req := mock.registeredResources[1]

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

	mock := &mockResourceMonitor{}
	engine := NewEngine(config, &EngineOptions{
		ProjectName:     "test-project",
		StackName:       "dev",
		ResourceMonitor: mock,
		WorkDir:         t.TempDir(),
		RootDir:         t.TempDir(),
		SchemaLoader: newMockReferenceLoader(t, schema.PackageSpec{
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
	if mock.stackOutputs == nil {
		t.Fatal("expected stack outputs")
	}
	regionOutput, ok := mock.stackOutputs["region_value"]
	if !ok {
		t.Fatal("expected region_value output")
	}
	if regionOutput.StringValue() != "us-west-2" {
		t.Errorf("expected region_value=%q from config, got %q", "us-west-2", regionOutput.StringValue())
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

	mock := &mockResourceMonitor{}
	engine := NewEngine(config, &EngineOptions{
		ProjectName:     "test-project",
		StackName:       "dev",
		ResourceMonitor: mock,
		WorkDir:         t.TempDir(),
		RootDir:         t.TempDir(),
		SchemaLoader: newMockReferenceLoader(t, schema.PackageSpec{
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
	if mock.stackOutputs == nil {
		t.Fatal("expected stack outputs")
	}
	regionOutput, ok := mock.stackOutputs["region_value"]
	if !ok {
		t.Fatal("expected region_value output")
	}
	if regionOutput.StringValue() != "eu-west-1" {
		t.Errorf("expected region_value=%q from env, got %q", "eu-west-1", regionOutput.StringValue())
	}
}

func TestEngine_VariableRequired(t *testing.T) {
	t.Parallel()

	src := []byte(`
variable "required_var" {
  type     = string
  nullable = false
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
		WorkDir:         t.TempDir(),
		RootDir:         t.TempDir(),
		SchemaLoader: newMockReferenceLoader(t, schema.PackageSpec{
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

	// Should error because required_var has no value
	if err == nil {
		t.Fatal("expected error for missing required variable")
	}
	if !strings.Contains(err.Error(), "required_var") {
		t.Errorf("expected error to mention required_var, got: %v", err)
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

	mock := &mockResourceMonitor{}
	engine := NewEngine(config, &EngineOptions{
		ProjectName:     "test-project",
		StackName:       "dev",
		ResourceMonitor: mock,
		WorkDir:         t.TempDir(),
		RootDir:         t.TempDir(),
		SchemaLoader: newMockReferenceLoader(t, schema.PackageSpec{
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
	output, ok := mock.stackOutputs["instance_type"]
	if !ok {
		t.Fatal("expected instance_type output")
	}
	if output.StringValue() != "t3.micro" {
		t.Errorf("expected t3.micro, got %q", output.StringValue())
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

	mock := &mockResourceMonitor{}
	engine := NewEngine(config, &EngineOptions{
		ProjectName:     "test-project",
		StackName:       "dev",
		ResourceMonitor: mock,
		WorkDir:         t.TempDir(),
		RootDir:         t.TempDir(),
		SchemaLoader: newMockReferenceLoader(t, schema.PackageSpec{
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

func TestEngine_PreconditionPass(t *testing.T) {
	t.Parallel()

	src := []byte(`
variable "ami_id" {
  type    = string
  default = "ami-12345"
}

resource "aws_instance" "web" {
  ami = var.ami_id

  lifecycle {
    precondition {
      condition     = startswith(var.ami_id, "ami-")
      error_message = "AMI ID must start with 'ami-'."
    }
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
		WorkDir:         t.TempDir(),
		RootDir:         t.TempDir(),
		SchemaLoader: newMockReferenceLoader(t, schema.PackageSpec{
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

	// Should pass - precondition is satisfied
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Resource should be registered
	found := false
	for _, r := range mock.registeredResources {
		if r.Name == "web" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected resource web to be registered")
	}
}

func TestEngine_PreconditionFail(t *testing.T) {
	t.Parallel()

	src := []byte(`
variable "ami_id" {
  type    = string
  default = "invalid-ami"
}

resource "aws_instance" "web" {
  ami = var.ami_id

  lifecycle {
    precondition {
      condition     = startswith(var.ami_id, "ami-")
      error_message = "AMI ID must start with 'ami-'."
    }
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
		WorkDir:         t.TempDir(),
		RootDir:         t.TempDir(),
		SchemaLoader: newMockReferenceLoader(t, schema.PackageSpec{
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

	// Should error because precondition fails
	if err == nil {
		t.Fatal("expected error for precondition failure")
	}
	if !strings.Contains(err.Error(), "AMI ID must start with 'ami-'") {
		t.Errorf("expected error message from precondition, got: %v", err)
	}

	// Resource should not be registered (only stack)
	for _, r := range mock.registeredResources {
		if r.Name == "web" {
			t.Fatal("resource should not be registered when precondition fails")
		}
	}
}

func TestEngine_PostconditionPass(t *testing.T) {
	t.Parallel()

	src := []byte(`
resource "aws_instance" "web" {
  ami = "ami-12345"

  lifecycle {
    postcondition {
      condition     = self.id != ""
      error_message = "Instance ID must not be empty."
    }
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
		WorkDir:         t.TempDir(),
		RootDir:         t.TempDir(),
		SchemaLoader: newMockReferenceLoader(t, schema.PackageSpec{
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

	// Should pass - postcondition is satisfied (id is set by mock)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestEngine_PostconditionFail(t *testing.T) {
	t.Parallel()

	src := []byte(`
resource "aws_instance" "web" {
  ami = "ami-12345"

  lifecycle {
    postcondition {
      condition     = self.ami == "ami-67890"
      error_message = "AMI must be ami-67890."
    }
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
		WorkDir:         t.TempDir(),
		RootDir:         t.TempDir(),
		SchemaLoader: newMockReferenceLoader(t, schema.PackageSpec{
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

	// Should error because postcondition fails
	if err == nil {
		t.Fatal("expected error for postcondition failure")
	}
	if !strings.Contains(err.Error(), "AMI must be ami-67890") {
		t.Errorf("expected error message from postcondition, got: %v", err)
	}
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

	mock := &mockResourceMonitor{}
	engine := NewEngine(config, &EngineOptions{
		ProjectName:     "test-project",
		StackName:       "dev",
		ResourceMonitor: mock,
		WorkDir:         t.TempDir(),
		RootDir:         t.TempDir(),
		SchemaLoader: newMockReferenceLoader(t, schema.PackageSpec{
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
	if len(mock.registeredResources) < 3 {
		t.Fatalf("expected at least 3 registered resources, got %d", len(mock.registeredResources))
	}

	// Find the provisioner resource
	var provisionerReq *RegisterResourceRequest
	for i := range mock.registeredResources {
		if mock.registeredResources[i].Type == "command:local:Command" {
			provisionerReq = &mock.registeredResources[i]
			break
		}
	}

	if provisionerReq == nil {
		t.Fatal("expected command:local:Command provisioner to be registered")
	}

	// Check that the command was mapped to create
	if create, ok := provisionerReq.Inputs["create"]; ok {
		if create.StringValue() != "echo 'Hello World'" {
			t.Errorf("expected create command 'echo 'Hello World'', got %s", create.StringValue())
		}
	} else {
		t.Error("expected 'create' input to be set")
	}

	// Check working_dir was mapped to dir
	if dir, ok := provisionerReq.Inputs["dir"]; ok {
		if dir.StringValue() != "/tmp" {
			t.Errorf("expected dir '/tmp', got %s", dir.StringValue())
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

	mock := &mockResourceMonitor{}
	engine := NewEngine(config, &EngineOptions{
		ProjectName:     "test-project",
		StackName:       "dev",
		ResourceMonitor: mock,
		WorkDir:         t.TempDir(),
		RootDir:         t.TempDir(),
		SchemaLoader: newMockReferenceLoader(t, schema.PackageSpec{
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
	for _, r := range mock.registeredResources {
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

	mock := &mockResourceMonitor{}
	engine := NewEngine(config, &EngineOptions{
		ProjectName:     "test-project",
		StackName:       "dev",
		ResourceMonitor: mock,
		WorkDir:         t.TempDir(),
		RootDir:         t.TempDir(),
		SchemaLoader: newMockReferenceLoader(t, schema.PackageSpec{
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
	var provisionerReq *RegisterResourceRequest
	for i := range mock.registeredResources {
		if mock.registeredResources[i].Type == "command:local:Command" {
			provisionerReq = &mock.registeredResources[i]
			break
		}
	}

	if provisionerReq == nil {
		t.Fatal("expected command:local:Command provisioner to be registered")
	}

	// Check that self.id was resolved
	if create, ok := provisionerReq.Inputs["create"]; ok {
		// The id should be set to the resource name + "-id" by the mock
		if !strings.Contains(create.StringValue(), "web-id") {
			t.Errorf("expected self.id to be resolved, got: %s", create.StringValue())
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

	mock := &mockResourceMonitor{}
	engine := NewEngine(config, &EngineOptions{
		ProjectName:     "test-project",
		StackName:       "dev",
		ResourceMonitor: mock,
		WorkDir:         tmpDir,
		RootDir:         tmpDir,
		SchemaLoader: newMockReferenceLoader(t, schema.PackageSpec{
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
	if len(mock.registeredResources) < 3 {
		t.Fatalf("expected at least 3 registered resources, got %d", len(mock.registeredResources))
	}

	// Find the module component (type is now dynamic: {project}:modules:{name})
	var moduleComponent *RegisterResourceRequest
	for i := range mock.registeredResources {
		if strings.Contains(mock.registeredResources[i].Type, ":modules:") {
			moduleComponent = &mock.registeredResources[i]
			break
		}
	}

	if moduleComponent == nil {
		t.Fatal("expected module component to be registered")
	}

	// Verify the dynamic component type token format
	expectedType := "test-project:modules:vpc"
	if moduleComponent.Type != expectedType {
		t.Errorf("expected module type %q, got %q", expectedType, moduleComponent.Type)
	}

	// Check that the module name includes the module name
	if !strings.Contains(moduleComponent.Name, "module.vpc") {
		t.Errorf("expected module name to contain 'module.vpc', got: %s", moduleComponent.Name)
	}

	// Find the VPC resource
	var vpcResource *RegisterResourceRequest
	for i := range mock.registeredResources {
		if mock.registeredResources[i].Type == "aws:index:Vpc" {
			vpcResource = &mock.registeredResources[i]
			break
		}
	}

	if vpcResource == nil {
		t.Fatal("expected aws:index:Vpc resource to be registered")
	}

	// Check that the VPC has the correct cidr_block
	if cidr, ok := vpcResource.Inputs["cidrBlock"]; ok {
		if cidr.StringValue() != "10.0.0.0/16" {
			t.Errorf("expected cidr_block '10.0.0.0/16', got %s", cidr.StringValue())
		}
	} else {
		t.Error("expected 'cidr_block' input to be set")
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

	mock := &mockResourceMonitor{}
	engine := NewEngine(config, &EngineOptions{
		ProjectName:     "test-project",
		StackName:       "dev",
		ResourceMonitor: mock,
		WorkDir:         t.TempDir(),
		RootDir:         t.TempDir(),
		SchemaLoader: newMockReferenceLoader(t, schema.PackageSpec{
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
	var instanceReq *RegisterResourceRequest
	for i := range mock.registeredResources {
		if mock.registeredResources[i].Type == "aws:index:Instance" {
			instanceReq = &mock.registeredResources[i]
			break
		}
	}

	if instanceReq == nil {
		t.Fatal("expected aws:index:Instance resource to be registered")
	}

	// Check that timeouts were set
	if instanceReq.CustomTimeouts == nil {
		t.Fatal("expected CustomTimeouts to be set")
	}

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

	mock := &mockResourceMonitor{}
	engine := NewEngine(config, &EngineOptions{
		ProjectName:     "test-project",
		StackName:       "dev",
		ResourceMonitor: mock,
		WorkDir:         t.TempDir(),
		RootDir:         t.TempDir(),
		SchemaLoader: newMockReferenceLoader(t, schema.PackageSpec{
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
	var instanceReq *RegisterResourceRequest
	for i := range mock.registeredResources {
		if mock.registeredResources[i].Type == "aws:index:Instance" {
			instanceReq = &mock.registeredResources[i]
			break
		}
	}

	if instanceReq == nil {
		t.Fatal("expected aws:index:Instance resource to be registered")
	}

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

	mock := &mockResourceMonitor{}
	engine := NewEngine(config, &EngineOptions{
		ProjectName:     "test-project",
		StackName:       "dev",
		ResourceMonitor: mock,
		WorkDir:         t.TempDir(),
		RootDir:         t.TempDir(),
		SchemaLoader: newMockReferenceLoader(t, schema.PackageSpec{
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
	var instanceReq *RegisterResourceRequest
	for i := range mock.registeredResources {
		if mock.registeredResources[i].Type == "aws:index:Instance" {
			instanceReq = &mock.registeredResources[i]
			break
		}
	}

	if instanceReq == nil {
		t.Fatal("expected aws:index:Instance resource to be registered")
	}

	// Check that ImportId was set
	if instanceReq.ImportId != "i-1234567890abcdef0" {
		t.Errorf("expected ImportId 'i-1234567890abcdef0', got %q", instanceReq.ImportId)
	}
}
