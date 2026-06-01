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

// Package bridge resolves Terraform provider names to bridged
// tfbridge.ProviderInfo instances via the Pulumi convert.Mapper, so the engine
// can use original TF block/attribute layout when interpreting HCL.
package bridge

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"github.com/pulumi/pulumi-terraform-bridge/v3/pkg/tfbridge"
	"github.com/pulumi/pulumi/pkg/v3/codegen/convert"
	"github.com/pulumi/pulumi/sdk/v3/go/common/workspace"
)

// ProviderInfoSource resolves a TF provider name to its bridged
// tfbridge.ProviderInfo. desc, when non-nil, carries the on-disk SDK
// package descriptor (written by `pulumi install` via GeneratePackage) and
// routes the mapper request through that base provider's parameterization.
type ProviderInfoSource interface {
	GetProviderInfo(ctx context.Context, tfProvider string, desc *workspace.PackageDescriptor) (*tfbridge.ProviderInfo, error)
}

// Cache wraps another ProviderInfoSource with per-process memoisation,
// keyed by the TF provider name. Negative results are cached as nil entries.
type Cache struct {
	inner ProviderInfoSource

	mu      sync.RWMutex
	entries map[string]cacheEntry
}

type cacheEntry struct {
	info *tfbridge.ProviderInfo
	err  error
}

// NewCache wraps source with read-through caching.
func NewCache(source ProviderInfoSource) *Cache {
	return &Cache{
		inner:   source,
		entries: map[string]cacheEntry{},
	}
}

// GetProviderInfo returns the bridged info for tfProvider; subsequent calls
// for the same name reuse the first result (including errors).
func (c *Cache) GetProviderInfo(
	ctx context.Context, tfProvider string, desc *workspace.PackageDescriptor,
) (*tfbridge.ProviderInfo, error) {
	c.mu.RLock()
	if e, ok := c.entries[tfProvider]; ok {
		c.mu.RUnlock()
		return e.info, e.err
	}
	c.mu.RUnlock()

	c.mu.Lock()
	defer c.mu.Unlock()
	if e, ok := c.entries[tfProvider]; ok {
		return e.info, e.err
	}
	info, err := c.inner.GetProviderInfo(ctx, tfProvider, desc)
	c.entries[tfProvider] = cacheEntry{info: info, err: err}
	return info, err
}

// MapperSource queries a convert.Mapper for the bridge mapping JSON and
// decodes it into a tfbridge.ProviderInfo.
type MapperSource struct {
	mapper convert.Mapper
}

// NewMapperSource wraps mapper as a ProviderInfoSource.
func NewMapperSource(mapper convert.Mapper) *MapperSource {
	return &MapperSource{mapper: mapper}
}

// GetProviderInfo asks the mapper for the bridge mapping of tfProvider. When
// desc carries a parameterization (the SDK on disk was written by
// `pulumi install` for a bridged provider), the request routes via that
// base provider's plugin with the parameterization attached. Otherwise the
// matching Pulumi plugin is hinted by tfProvider name.
func (s *MapperSource) GetProviderInfo(
	ctx context.Context, tfProvider string, desc *workspace.PackageDescriptor,
) (*tfbridge.ProviderInfo, error) {
	if s.mapper == nil {
		return nil, errors.New("no mapper available")
	}

	mapperHint := &convert.MapperPackageHint{PluginName: tfProvider}
	if desc != nil && desc.Parameterization != nil {
		mapperHint = &convert.MapperPackageHint{
			PluginName:       desc.Name,
			Parameterization: desc.Parameterization,
		}
	}

	data, err := s.mapper.GetMapping(ctx, tfProvider, mapperHint)
	if err != nil {
		return nil, fmt.Errorf("mapper GetMapping(%q): %w", tfProvider, err)
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("no mapping available for terraform provider %q", tfProvider)
	}

	var marshalled tfbridge.MarshallableProviderInfo
	if err := json.Unmarshal(data, &marshalled); err != nil {
		return nil, fmt.Errorf("decoding mapping for %q: %w", tfProvider, err)
	}
	return marshalled.Unmarshal(), nil
}
