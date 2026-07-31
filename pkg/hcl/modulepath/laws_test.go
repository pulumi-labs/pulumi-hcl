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

// Property tests for the modulepath laws. They hold for step names that are
// valid HCL labels; instance keys are arbitrary strings.
package modulepath_test

import (
	"testing"

	"pgregory.net/rapid"

	"github.com/pulumi/pulumi-hcl/pkg/hcl/modulepath"
)

func labelGen() *rapid.Generator[string] {
	return rapid.StringMatching(`[a-zA-Z_][a-zA-Z0-9_-]{0,8}`)
}

func stepGen() *rapid.Generator[modulepath.Step] {
	return rapid.Custom(func(t *rapid.T) modulepath.Step {
		name := labelGen().Draw(t, "name")
		switch rapid.IntRange(0, 2).Draw(t, "kind") {
		case 0:
			return modulepath.NewStep(name)
		case 1:
			return modulepath.NewIndexedStep(name, rapid.IntRange(0, 1<<20).Draw(t, "index"))
		default:
			return modulepath.NewKeyedStep(name, rapid.String().Draw(t, "key"))
		}
	})
}

func pathGen(minSteps int) *rapid.Generator[modulepath.Path] {
	return rapid.Custom(func(t *rapid.T) modulepath.Path {
		p := modulepath.Root()
		for _, s := range rapid.SliceOfN(stepGen(), minSteps, 5).Draw(t, "steps") {
			p = p.Append(s)
		}
		return p
	})
}

func addressGen() *rapid.Generator[modulepath.Address] {
	return rapid.Custom(func(t *rapid.T) modulepath.Address {
		return modulepath.NewAddress(pathGen(0).Draw(t, "module"), stepGen().Draw(t, "resource"))
	})
}

func TestLaw_LogicalNameRoundTrip(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		p := pathGen(1).Draw(t, "p")
		rendered := p.LogicalName()
		parsed, err := modulepath.ParseLogicalName(rendered)
		if err != nil {
			t.Fatalf("parse of rendered %q: %v", rendered, err)
		}
		if parsed != p {
			t.Fatalf("round trip of %q changed the path", rendered)
		}
		if parsed.LogicalName() != rendered {
			t.Fatalf("re-render of %q differs", rendered)
		}
	})
}

func TestLaw_LogicalNameInjective(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		p := pathGen(1).Draw(t, "p")
		q := pathGen(1).Draw(t, "q")
		if p != q && p.LogicalName() == q.LogicalName() {
			t.Fatalf("distinct paths render identically: %q", p.LogicalName())
		}
	})
}

func TestLaw_Config(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		p := pathGen(0).Draw(t, "p")
		config := p.Config()
		if config.Config() != config {
			t.Fatalf("Config() is not idempotent")
		}
		for s := range config.Steps {
			if _, ok := s.Index(); ok {
				t.Fatalf("Config() kept a count index")
			}
			if _, ok := s.Key(); ok {
				t.Fatalf("Config() kept a for_each key")
			}
		}
	})
}

func TestLaw_AppendParent(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		p := pathGen(0).Draw(t, "p")
		s := stepGen().Draw(t, "s")
		parent, last, ok := p.Append(s).Parent()
		if !ok || parent != p || last != s {
			t.Fatalf("Parent() did not invert Append()")
		}
	})
}

func TestLaw_CompareTotalOrder(t *testing.T) {
	t.Parallel()
	sign := func(n int) int {
		switch {
		case n < 0:
			return -1
		case n > 0:
			return 1
		default:
			return 0
		}
	}
	rapid.Check(t, func(t *rapid.T) {
		a := pathGen(0).Draw(t, "a")
		b := pathGen(0).Draw(t, "b")
		c := pathGen(0).Draw(t, "c")
		if (modulepath.Compare(a, b) == 0) != (a == b) {
			t.Fatalf("Compare zero disagrees with ==")
		}
		if sign(modulepath.Compare(a, b)) != -sign(modulepath.Compare(b, a)) {
			t.Fatalf("Compare is not antisymmetric")
		}
		if modulepath.Compare(a, b) <= 0 && modulepath.Compare(b, c) <= 0 &&
			modulepath.Compare(a, c) > 0 {
			t.Fatalf("Compare is not transitive")
		}
	})
}

func TestLaw_AddressRoundTrip(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		a := addressGen().Draw(t, "a")
		rendered := a.LogicalName()
		parsed, err := modulepath.ParseAddress(rendered)
		if err != nil {
			t.Fatalf("parse of rendered %q: %v", rendered, err)
		}
		if parsed != a {
			t.Fatalf("round trip of %q changed the address", rendered)
		}
		if !a.InstanceOf(a.Config()) {
			t.Fatalf("address is not an instance of its own config address")
		}
		c := addressGen().Draw(t, "c").Config()
		if a.InstanceOf(c) != (a.Config() == c) {
			t.Fatalf("InstanceOf disagrees with Config() equality")
		}
	})
}
