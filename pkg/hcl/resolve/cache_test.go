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
	"errors"
	"testing"

	pulumirpc "github.com/pulumi/pulumi/sdk/v3/proto/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// countingResolver records how many times ResolvePackage is invoked per source.
type countingResolver struct {
	fakeResolver
	calls map[string]int
}

func newCountingResolver() *countingResolver {
	c := &countingResolver{calls: map[string]int{}}
	c.respond = func(spec *pulumirpc.PackageSpec) (*pulumirpc.PackageDependency, error) {
		c.calls[spec.Source]++
		if spec.Source == "boom" {
			return nil, errors.New("boom")
		}
		return &pulumirpc.PackageDependency{Name: spec.Source, Kind: "resource", Version: "1.0.0"}, nil
	}
	return c
}

func TestCacheResolvesEachSpecOnce(t *testing.T) {
	t.Parallel()

	inner := newCountingResolver()
	cache := NewCache(inner)

	aws := &pulumirpc.PackageSpec{Source: "terraform-provider", Parameters: []string{"hashicorp/aws", "~> 6.0"}}
	for range 3 {
		dep, err := cache.ResolvePackage(t.Context(), aws)
		require.NoError(t, err)
		assert.Equal(t, &pulumirpc.PackageDependency{
			Name: "terraform-provider", Kind: "resource", Version: "1.0.0",
		}, dep)
	}
	assert.Equal(t, 1, inner.calls["terraform-provider"])

	// A spec differing only in its parameters is a distinct cache entry.
	gcp := &pulumirpc.PackageSpec{Source: "terraform-provider", Parameters: []string{"hashicorp/google", "~> 6.0"}}
	_, err := cache.ResolvePackage(t.Context(), gcp)
	require.NoError(t, err)
	assert.Equal(t, 2, inner.calls["terraform-provider"])
}

func TestCacheCachesErrors(t *testing.T) {
	t.Parallel()

	inner := newCountingResolver()
	cache := NewCache(inner)

	for range 2 {
		_, err := cache.ResolvePackage(t.Context(), &pulumirpc.PackageSpec{Source: "boom"})
		assert.EqualError(t, err, "boom")
	}
	assert.Equal(t, 1, inner.calls["boom"])
}
