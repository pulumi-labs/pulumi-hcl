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
	"testing"

	"github.com/pulumi/pulumi/sdk/v3/go/property"
	"github.com/stretchr/testify/assert"

	"github.com/pulumi-labs/pulumi-hcl/pkg/hcl/packages"
)

func TestLowerTerraformDataInputs(t *testing.T) {
	t.Parallel()

	null := property.New(property.Null)

	tests := []struct {
		name        string
		inputs      property.Map
		opts        ResourceOptions
		wantInputs  property.Map
		wantTrigger property.Value
	}{
		{
			name: "triggers_replace present",
			inputs: property.NewMap(map[string]property.Value{
				"input":            property.New("a"),
				"triggers_replace": property.New("x"),
			}),
			wantInputs: property.NewMap(map[string]property.Value{
				"input": property.New("a"),
			}),
			wantTrigger: property.New([]property.Value{property.New("x"), null}),
		},
		{
			name: "triggers_replace absent still registers a non-null trigger",
			inputs: property.NewMap(map[string]property.Value{
				"input": property.New("a"),
			}),
			wantInputs: property.NewMap(map[string]property.Value{
				"input": property.New("a"),
			}),
			wantTrigger: property.New([]property.Value{null, null}),
		},
		{
			name:   "omitted input is materialized as null",
			inputs: property.Map{},
			wantInputs: property.NewMap(map[string]property.Value{
				"input": null,
			}),
			wantTrigger: property.New([]property.Value{null, null}),
		},
		{
			name: "replace_triggered_by trigger is preserved alongside triggers_replace",
			inputs: property.NewMap(map[string]property.Value{
				"input":            property.New("a"),
				"triggers_replace": property.New("x"),
			}),
			opts: ResourceOptions{ReplacementTrigger: property.New("lifecycle")},
			wantInputs: property.NewMap(map[string]property.Value{
				"input": property.New("a"),
			}),
			wantTrigger: property.New([]property.Value{
				property.New("x"), property.New("lifecycle"),
			}),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := lowerTerraformDataInputs(packages.TerraformDataType, tt.inputs, &tt.opts)
			assert.Equal(t, tt.wantInputs, got)
			assert.Equal(t, tt.wantTrigger, tt.opts.ReplacementTrigger)
		})
	}

	t.Run("other types are untouched", func(t *testing.T) {
		t.Parallel()

		inputs := property.NewMap(map[string]property.Value{
			"triggers_replace": property.New("x"),
		})
		opts := ResourceOptions{}
		got := lowerTerraformDataInputs("random_pet", inputs, &opts)
		assert.Equal(t, inputs, got)
		assert.Equal(t, ResourceOptions{}, opts)
	})
}

func TestLowerTerraformDataOutputs(t *testing.T) {
	t.Parallel()

	null := property.New(property.Null)
	inputs := property.NewMap(map[string]property.Value{
		"input":            property.New("a"),
		"triggers_replace": property.New("x"),
	})
	opts := ResourceOptions{}
	lowerTerraformDataInputs(packages.TerraformDataType, inputs, &opts)

	outputs := property.NewMap(map[string]property.Value{
		"input": property.New("a"),
	})
	got := lowerTerraformDataOutputs(packages.TerraformDataType, outputs, &opts)
	assert.Equal(t, property.NewMap(map[string]property.Value{
		"input":            property.New("a"),
		"output":           property.New("a"),
		"triggers_replace": property.New("x"),
	}), got)

	t.Run("absent triggers_replace echoes null", func(t *testing.T) {
		t.Parallel()

		opts := ResourceOptions{}
		lowerTerraformDataInputs(packages.TerraformDataType, property.Map{}, &opts)
		got := lowerTerraformDataOutputs(packages.TerraformDataType, property.Map{}, &opts)
		assert.Equal(t, property.NewMap(map[string]property.Value{
			"output":           null,
			"triggers_replace": null,
		}), got)
	})
}
