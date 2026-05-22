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
	"fmt"
	"sync"

	"github.com/pulumi-labs/pulumi-hcl/pkg/hcl/run"
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
	hooks               map[string]registeredHook

	// DryRun mirrors engine preview mode: hooks with OnDryRun=false are skipped.
	DryRun bool

	// InvokeHandler, if set, is called for each Invoke instead of the default behavior.
	InvokeHandler func(ctx context.Context, req run.InvokeRequest) (*run.InvokeResponse, error)

	// RegisterResourceHandler, if  set, is  called for each  RegisterResource instead of the default behavior.
	RegisterResourceHandler func(ctx context.Context, req run.RegisterResourceRequest) (*run.RegisterResourceResponse, error)
}

type registeredHook struct {
	callback run.ResourceHookFunction
	opts     run.ResourceHookOptions
}

func (m *MockResourceMonitor) RegisterResource(ctx context.Context, req run.RegisterResourceRequest) (*run.RegisterResourceResponse, error) {
	m.mu.Lock()
	urn := "urn:pulumi:test::project::" + req.Type + "::" + req.Name
	if req.Type == "pulumi:pulumi:Stack" {
		m.stackURN = urn
	}
	hooks := m.hooks
	m.mu.Unlock()

	if req.Hooks != nil {
		args := &run.ResourceHookArgs{
			URN:       urn,
			Name:      req.Name,
			Type:      req.Type,
			NewInputs: req.Inputs,
		}
		// The mock has no state, so every registration is treated as a create.
		for _, name := range req.Hooks.BeforeCreate {
			h, ok := hooks[name]
			if !ok {
				return nil, fmt.Errorf("hook %q not registered", name)
			}
			if m.DryRun && !h.opts.OnDryRun {
				continue
			}
			if err := h.callback(ctx, args); err != nil {
				return nil, fmt.Errorf("hook %q failed: %w", name, err)
			}
		}
	}

	m.mu.Lock()
	m.RegisteredResources = append(m.RegisteredResources, req)
	handler := m.RegisterResourceHandler
	m.mu.Unlock()

	if req.Type != "pulumi:pulumi:Stack" && handler != nil {
		return handler(ctx, req)
	}
	return &run.RegisterResourceResponse{
		URN:     urn,
		ID:      req.Name + "-id",
		Outputs: req.Inputs,
	}, nil
}

func (m *MockResourceMonitor) Invoke(ctx context.Context, req run.InvokeRequest) (*run.InvokeResponse, error) {
	m.mu.Lock()
	m.InvokedFunctions = append(m.InvokedFunctions, req)
	handler := m.InvokeHandler
	m.mu.Unlock()

	if handler != nil {
		return handler(ctx, req)
	}
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

func (m *MockResourceMonitor) RegisterResourceHook(
	ctx context.Context, name string, callback run.ResourceHookFunction, opts run.ResourceHookOptions,
) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.hooks == nil {
		m.hooks = make(map[string]registeredHook)
	}
	if _, exists := m.hooks[name]; exists {
		return fmt.Errorf("hook %q already registered", name)
	}
	m.hooks[name] = registeredHook{callback: callback, opts: opts}
	return nil
}
