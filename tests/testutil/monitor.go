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

package testutil

import (
	"context"
	"sync"

	"github.com/pulumi/pulumi-language-hcl/pkg/hcl/run"
	"github.com/pulumi/pulumi/sdk/v3/go/common/workspace"
	"github.com/pulumi/pulumi/sdk/v3/go/property"
)

// MockResourceMonitor is a mock implementation of run.ResourceMonitor for testing.
type MockResourceMonitor struct {
	mu                  sync.Mutex
	RegisteredResources []run.RegisterResourceRequest
	InvokedFunctions    []run.InvokeRequest
	StackOutputs        property.Map
	stackURN            string
}

func (m *MockResourceMonitor) RegisterResource(ctx context.Context, req run.RegisterResourceRequest) (*run.RegisterResourceResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.RegisteredResources = append(m.RegisteredResources, req)
	urn := "urn:pulumi:test::project::" + req.Type + "::" + req.Name
	if req.Type == "pulumi:pulumi:Stack" {
		m.stackURN = urn
	}
	return &run.RegisterResourceResponse{
		URN:     urn,
		ID:      req.Name + "-id",
		Outputs: req.Inputs,
	}, nil
}

func (m *MockResourceMonitor) Invoke(ctx context.Context, req run.InvokeRequest) (*run.InvokeResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.InvokedFunctions = append(m.InvokedFunctions, req)
	return &run.InvokeResponse{
		Return: property.NewMap(map[string]property.Value{
			"id": property.New("mock-id"),
		}),
	}, nil
}

func (m *MockResourceMonitor) RegisterResourceOutputs(ctx context.Context, urn string, outputs property.Map) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if urn == m.stackURN {
		m.StackOutputs = outputs
	}
	return nil
}

func (m *MockResourceMonitor) Call(ctx context.Context, req run.CallRequest) (*run.CallResponse, error) {
	return &run.CallResponse{
		Return: property.NewMap(nil),
	}, nil
}

func (m *MockResourceMonitor) CheckPulumiVersion(ctx context.Context, versionRange string) error {
	return nil
}

func (m *MockResourceMonitor) RegisterPackage(ctx context.Context, pkg workspace.PackageDescriptor) (run.PackageRef, error) {
	return "", nil
}
