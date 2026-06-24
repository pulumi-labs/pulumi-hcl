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

package resolve

import (
	"context"
	"hash/maphash"
	"sync"

	pulumirpc "github.com/pulumi/pulumi/sdk/v3/proto/go"
	"google.golang.org/grpc"
)

type Cache struct {
	inner pulumirpc.PackageResolverClient

	mu      sync.Mutex
	entries map[specHash]cacheEntry
}

type cacheEntry struct {
	dep *pulumirpc.PackageDependency
	err error
}

func NewCache(resolver pulumirpc.PackageResolverClient) *Cache {
	return &Cache{
		inner:   resolver,
		entries: map[specHash]cacheEntry{},
	}
}

func (c *Cache) ResolvePackage(
	ctx context.Context, spec *pulumirpc.PackageSpec, opts ...grpc.CallOption,
) (*pulumirpc.PackageDependency, error) {
	key := hashSpec(spec)

	c.mu.Lock()
	defer c.mu.Unlock()
	if e, ok := c.entries[key]; ok {
		return e.dep, e.err
	}
	dep, err := c.inner.ResolvePackage(ctx, spec, opts...)
	c.entries[key] = cacheEntry{dep: dep, err: err}
	return dep, err
}

type specHash uint64

var specHashSeed = maphash.MakeSeed()

func hashSpec(spec *pulumirpc.PackageSpec) specHash {
	var h maphash.Hash
	h.SetSeed(specHashSeed)
	maphash.WriteComparable(&h, spec.GetSource())
	maphash.WriteComparable(&h, spec.GetVersion())
	maphash.WriteComparable(&h, len(spec.GetParameters()))
	for _, p := range spec.GetParameters() {
		maphash.WriteComparable(&h, p)
	}
	return specHash(h.Sum64())
}
