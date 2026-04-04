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
	"context"
	"testing"

	"github.com/pulumi-labs/pulumi-hcl/pkg/hcl/parser"
	"github.com/pulumi/pulumi/pkg/v3/util/pdag"
	"github.com/stretchr/testify/require"
	"github.com/zclconf/go-cty/cty"
)

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
	require.Empty(t, diags)

	g, err := BuildFromConfig(config, nil, "")
	require.NoError(t, err)

	nodes := g.seen
	require.Len(t, nodes, 7, "4 explicit nodes + 3 builtins")

	// Verify dependencies
	localNode := g.seen["local.greeting"].n
	if localNode == nil {
		t.Fatal("Expected local.greeting node")
	}

	// Verify topological sort works
	var sorted []string
	err = g.dag.Walk(t.Context(), func(_ context.Context, n dagNode) error {
		sorted = append(sorted, n.key)
		return nil
	}, pdag.MaxProcs(1))
	require.NoError(t, err)

	positions := make(map[string]int)
	for i, node := range sorted {
		positions[node] = i
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

func TestValidate(t *testing.T) {
	g := NewGraph()

	// Missing dependency
	_, i := g.newNode("NonExistent")
	err := g.AddNode(&Node{Key: "A", Type: NodeTypeLocal}, []pdag.Node{i})
	require.NoError(t, err)

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
			wantIndex: new(0),
		},
		{
			input:     "aws_instance.web[42]",
			wantBase:  "aws_instance.web",
			wantIndex: new(42),
		},
		{
			input:       `aws_instance.web["a"]`,
			wantBase:    "aws_instance.web",
			wantEachKey: new("a"),
		},
		{
			input:       `aws_instance.web["my-key"]`,
			wantBase:    "aws_instance.web",
			wantEachKey: new("my-key"),
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
