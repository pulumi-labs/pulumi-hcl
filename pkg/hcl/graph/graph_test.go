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

package graph

import (
	"testing"

	"github.com/pulumi/pulumi-language-hcl/pkg/hcl/parser"
	"github.com/zclconf/go-cty/cty"
)

func TestTopologicalSort(t *testing.T) {
	g := NewGraph()

	// Add nodes: A -> B -> C (A depends on nothing, B depends on A, C depends on B)
	g.AddNode(&Node{Key: "A", Type: NodeTypeVariable})
	g.AddNode(&Node{Key: "B", Type: NodeTypeLocal, Dependencies: []string{"A"}})
	g.AddNode(&Node{Key: "C", Type: NodeTypeResource, Dependencies: []string{"B"}})

	sorted, err := g.TopologicalSort()
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if len(sorted) != 3 {
		t.Fatalf("Expected 3 nodes, got %d", len(sorted))
	}

	// Find positions
	positions := make(map[string]int)
	for i, node := range sorted {
		positions[node.Key] = i
	}

	// Verify order: A before B, B before C
	if positions["A"] >= positions["B"] {
		t.Errorf("A should come before B")
	}
	if positions["B"] >= positions["C"] {
		t.Errorf("B should come before C")
	}
}

func TestTopologicalSortCycle(t *testing.T) {
	g := NewGraph()

	// Create a cycle: A -> B -> C -> A
	g.AddNode(&Node{Key: "A", Type: NodeTypeLocal, Dependencies: []string{"C"}})
	g.AddNode(&Node{Key: "B", Type: NodeTypeLocal, Dependencies: []string{"A"}})
	g.AddNode(&Node{Key: "C", Type: NodeTypeLocal, Dependencies: []string{"B"}})

	_, err := g.TopologicalSort()
	if err == nil {
		t.Fatal("Expected cycle error, got none")
	}
}

func TestTopologicalSortDiamondDependency(t *testing.T) {
	g := NewGraph()

	// Diamond: A is depended on by B and C, D depends on both B and C
	//     A
	//    / \
	//   B   C
	//    \ /
	//     D
	g.AddNode(&Node{Key: "A", Type: NodeTypeVariable})
	g.AddNode(&Node{Key: "B", Type: NodeTypeLocal, Dependencies: []string{"A"}})
	g.AddNode(&Node{Key: "C", Type: NodeTypeLocal, Dependencies: []string{"A"}})
	g.AddNode(&Node{Key: "D", Type: NodeTypeResource, Dependencies: []string{"B", "C"}})

	sorted, err := g.TopologicalSort()
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if len(sorted) != 4 {
		t.Fatalf("Expected 4 nodes, got %d", len(sorted))
	}

	positions := make(map[string]int)
	for i, node := range sorted {
		positions[node.Key] = i
	}

	// A must be first
	if positions["A"] != 0 {
		t.Errorf("A should be first, was at position %d", positions["A"])
	}

	// B and C must come after A
	if positions["B"] <= positions["A"] {
		t.Errorf("B should come after A")
	}
	if positions["C"] <= positions["A"] {
		t.Errorf("C should come after A")
	}

	// D must be last
	if positions["D"] <= positions["B"] || positions["D"] <= positions["C"] {
		t.Errorf("D should come after B and C")
	}
}

func TestBuildFromConfig(t *testing.T) {
	src := []byte(`
variable "name" {
  type = string
}

locals {
  greeting = "Hello, ${var.name}"
}

resource "aws_instance" "web" {
  ami = local.greeting
}

output "instance_id" {
  value = aws_instance.web.id
}
`)

	p := parser.NewParser()
	config, diags := p.ParseSource("test.hcl", src)
	if diags.HasErrors() {
		t.Fatalf("Parse error: %s", diags.Error())
	}

	g, err := BuildFromConfig(config)
	if err != nil {
		t.Fatalf("BuildFromConfig error: %v", err)
	}

	// Should have 4 nodes
	nodes := g.Nodes()
	if len(nodes) != 4 {
		t.Errorf("Expected 4 nodes, got %d", len(nodes))
	}

	// Verify dependencies
	localNode := g.GetNode("local.greeting")
	if localNode == nil {
		t.Fatal("Expected local.greeting node")
	}
	if len(localNode.Dependencies) != 1 || localNode.Dependencies[0] != "var.name" {
		t.Errorf("local.greeting should depend on var.name, got %v", localNode.Dependencies)
	}

	resourceNode := g.GetNode("aws_instance.web")
	if resourceNode == nil {
		t.Fatal("Expected aws_instance.web node")
	}
	if len(resourceNode.Dependencies) != 1 || resourceNode.Dependencies[0] != "local.greeting" {
		t.Errorf("aws_instance.web should depend on local.greeting, got %v", resourceNode.Dependencies)
	}

	outputNode := g.GetNode("output.instance_id")
	if outputNode == nil {
		t.Fatal("Expected output.instance_id node")
	}
	if len(outputNode.Dependencies) != 1 || outputNode.Dependencies[0] != "aws_instance.web" {
		t.Errorf("output.instance_id should depend on aws_instance.web, got %v", outputNode.Dependencies)
	}

	// Verify topological sort works
	sorted, err := g.TopologicalSort()
	if err != nil {
		t.Fatalf("TopologicalSort error: %v", err)
	}

	positions := make(map[string]int)
	for i, node := range sorted {
		positions[node.Key] = i
	}

	if positions["var.name"] >= positions["local.greeting"] {
		t.Error("var.name should come before local.greeting")
	}
	if positions["local.greeting"] >= positions["aws_instance.web"] {
		t.Error("local.greeting should come before aws_instance.web")
	}
	if positions["aws_instance.web"] >= positions["output.instance_id"] {
		t.Error("aws_instance.web should come before output.instance_id")
	}
}

func TestGetDependents(t *testing.T) {
	g := NewGraph()

	g.AddNode(&Node{Key: "A", Type: NodeTypeVariable})
	g.AddNode(&Node{Key: "B", Type: NodeTypeLocal, Dependencies: []string{"A"}})
	g.AddNode(&Node{Key: "C", Type: NodeTypeLocal, Dependencies: []string{"A"}})
	g.AddNode(&Node{Key: "D", Type: NodeTypeResource, Dependencies: []string{"B"}})

	dependents := g.GetDependents("A")
	if len(dependents) != 2 {
		t.Errorf("A should have 2 dependents, got %d", len(dependents))
	}

	dependents = g.GetDependents("B")
	if len(dependents) != 1 {
		t.Errorf("B should have 1 dependent, got %d", len(dependents))
	}

	dependents = g.GetDependents("D")
	if len(dependents) != 0 {
		t.Errorf("D should have 0 dependents, got %d", len(dependents))
	}
}

func TestGetTransitiveDependencies(t *testing.T) {
	g := NewGraph()

	g.AddNode(&Node{Key: "A", Type: NodeTypeVariable})
	g.AddNode(&Node{Key: "B", Type: NodeTypeLocal, Dependencies: []string{"A"}})
	g.AddNode(&Node{Key: "C", Type: NodeTypeResource, Dependencies: []string{"B"}})

	deps := g.GetTransitiveDependencies("C")
	if len(deps) != 2 {
		t.Errorf("C should have 2 transitive dependencies, got %d", len(deps))
	}

	// Check that both A and B are in deps
	found := make(map[string]bool)
	for _, dep := range deps {
		found[dep.Key] = true
	}
	if !found["A"] || !found["B"] {
		t.Errorf("Expected A and B in dependencies, got %v", found)
	}
}

func TestFilterByType(t *testing.T) {
	g := NewGraph()

	g.AddNode(&Node{Key: "var.a", Type: NodeTypeVariable})
	g.AddNode(&Node{Key: "var.b", Type: NodeTypeVariable})
	g.AddNode(&Node{Key: "local.x", Type: NodeTypeLocal})
	g.AddNode(&Node{Key: "aws_instance.web", Type: NodeTypeResource})

	vars := g.FilterByType(NodeTypeVariable)
	if len(vars) != 2 {
		t.Errorf("Expected 2 variables, got %d", len(vars))
	}

	locals := g.FilterByType(NodeTypeLocal)
	if len(locals) != 1 {
		t.Errorf("Expected 1 local, got %d", len(locals))
	}

	resources := g.FilterByType(NodeTypeResource)
	if len(resources) != 1 {
		t.Errorf("Expected 1 resource, got %d", len(resources))
	}
}

func TestValidate(t *testing.T) {
	g := NewGraph()

	// Missing dependency
	g.AddNode(&Node{Key: "A", Type: NodeTypeLocal, Dependencies: []string{"NonExistent"}})

	errors := g.Validate()
	if len(errors) != 1 {
		t.Errorf("Expected 1 error, got %d", len(errors))
	}
}

func TestResourceExpander(t *testing.T) {
	expander := NewResourceExpander()

	node := &Node{
		Key:  "aws_instance.web",
		Type: NodeTypeResource,
	}

	t.Run("single instance", func(t *testing.T) {
		result := expander.Expand(node)
		if !result.IsSingle {
			t.Error("Expected single instance")
		}
		if len(result.Instances) != 1 {
			t.Errorf("Expected 1 instance, got %d", len(result.Instances))
		}
		if result.Instances[0].Key != "aws_instance.web" {
			t.Errorf("Unexpected key: %s", result.Instances[0].Key)
		}
	})

	t.Run("count expansion", func(t *testing.T) {
		expander.SetCount("aws_instance.web", 3)
		result := expander.Expand(node)
		if result.IsSingle {
			t.Error("Should not be single instance")
		}
		if len(result.Instances) != 3 {
			t.Errorf("Expected 3 instances, got %d", len(result.Instances))
		}
		for i, inst := range result.Instances {
			expectedKey := "aws_instance.web[" + string(rune('0'+i)) + "]"
			if inst.Key != expectedKey {
				t.Errorf("Instance %d: expected key %s, got %s", i, expectedKey, inst.Key)
			}
			if inst.Index == nil || *inst.Index != i {
				t.Errorf("Instance %d: expected index %d", i, i)
			}
		}
	})

	t.Run("count zero", func(t *testing.T) {
		expander.SetCount("aws_instance.zero", 0)
		zeroNode := &Node{Key: "aws_instance.zero", Type: NodeTypeResource}
		result := expander.Expand(zeroNode)
		if result.IsSingle {
			t.Error("Should not be single instance")
		}
		if len(result.Instances) != 0 {
			t.Errorf("Expected 0 instances, got %d", len(result.Instances))
		}
	})

	t.Run("for_each expansion", func(t *testing.T) {
		expander2 := NewResourceExpander()
		expander2.SetForEach("aws_instance.each", map[string]cty.Value{
			"a": cty.StringVal("value_a"),
			"b": cty.StringVal("value_b"),
		})
		eachNode := &Node{Key: "aws_instance.each", Type: NodeTypeResource}
		result := expander2.Expand(eachNode)
		if result.IsSingle {
			t.Error("Should not be single instance")
		}
		if len(result.Instances) != 2 {
			t.Errorf("Expected 2 instances, got %d", len(result.Instances))
		}

		// Verify keys are set
		for _, inst := range result.Instances {
			if inst.EachKey == nil {
				t.Error("EachKey should be set")
			}
			if inst.EachValue == nil {
				t.Error("EachValue should be set")
			}
		}
	})
}

func TestParseInstanceKey(t *testing.T) {
	tests := []struct {
		input       string
		wantBase    string
		wantIndex   *int
		wantEachKey *string
	}{
		{
			input:    "aws_instance.web",
			wantBase: "aws_instance.web",
		},
		{
			input:     "aws_instance.web[0]",
			wantBase:  "aws_instance.web",
			wantIndex: intPtr(0),
		},
		{
			input:     "aws_instance.web[42]",
			wantBase:  "aws_instance.web",
			wantIndex: intPtr(42),
		},
		{
			input:       `aws_instance.web["a"]`,
			wantBase:    "aws_instance.web",
			wantEachKey: strPtr("a"),
		},
		{
			input:       `aws_instance.web["my-key"]`,
			wantBase:    "aws_instance.web",
			wantEachKey: strPtr("my-key"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			base, idx, key := ParseInstanceKey(tt.input)
			if base != tt.wantBase {
				t.Errorf("base: got %q, want %q", base, tt.wantBase)
			}
			if (idx == nil) != (tt.wantIndex == nil) || (idx != nil && *idx != *tt.wantIndex) {
				t.Errorf("index: got %v, want %v", idx, tt.wantIndex)
			}
			if (key == nil) != (tt.wantEachKey == nil) || (key != nil && *key != *tt.wantEachKey) {
				t.Errorf("eachKey: got %v, want %v", key, tt.wantEachKey)
			}
		})
	}
}

func intPtr(i int) *int    { return &i }
func strPtr(s string) *string { return &s }
