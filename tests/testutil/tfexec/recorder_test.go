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

package tfexec_test

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/pulumi-labs/pulumi-hcl/tests/testutil/tfexec"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// trivialProvider exposes one resource with two scalar inputs and one
// computed output. CreateContext writes a fixed result. Used to drive
// the recorder via direct CreateContext invocation, no gRPC involved.
func trivialProvider(result string) *schema.Provider {
	return &schema.Provider{
		ResourcesMap: map[string]*schema.Resource{
			"t_resource": {
				Schema: map[string]*schema.Schema{
					"input":  {Type: schema.TypeString, Optional: true},
					"result": {Type: schema.TypeString, Computed: true},
				},
				CreateContext: func(_ context.Context, d *schema.ResourceData, _ any) diag.Diagnostics {
					d.SetId("id-1")
					if err := d.Set("result", result); err != nil {
						return diag.FromErr(err)
					}
					return nil
				},
			},
		},
	}
}

// invokeCreate dives into the wrapped *schema.Resource and invokes its
// CreateContext with a freshly built ResourceData populated from inputs.
// Returns the diagnostics from the underlying call.
func invokeCreate(t *testing.T, p *schema.Provider, typeName string, inputs map[string]any) diag.Diagnostics {
	t.Helper()
	res := p.ResourcesMap[typeName]
	require.NotNil(t, res, "resource type not found: %s", typeName)
	d := res.Data(nil)
	for k, v := range inputs {
		require.NoError(t, d.Set(k, v))
	}
	return res.CreateContext(t.Context(), d, nil)
}

// TestWrap_RecordsCreate confirms the recorder captures a Create op with the
// resource type, the inputs (snapshot at boundary), and the outputs after the
// underlying CreateContext sets computed fields.
func TestWrap_RecordsCreate(t *testing.T) {
	t.Parallel()
	r := &tfexec.Recorder{}
	wrapped := tfexec.Wrap(trivialProvider("computed-value"), r)
	diags := invokeCreate(t, wrapped, "t_resource", map[string]any{"input": "in"})
	require.False(t, diags.HasError(), diags)

	ops := r.Ops()
	require.Len(t, ops, 1)
	assert.Equal(t, tfexec.Op{
		Kind:    tfexec.OpCreate,
		Type:    "t_resource",
		Inputs:  map[string]any{"input": "in", "result": ""},
		Outputs: map[string]any{"input": "in", "result": "computed-value"},
	}, ops[0])
}

// TestRecorder_EqualWhenSameOps verifies that two recorders fed identical
// calls compare equal regardless of the call order.
func TestRecorder_EqualWhenSameOps(t *testing.T) {
	t.Parallel()
	a, b := &tfexec.Recorder{}, &tfexec.Recorder{}
	pA := tfexec.Wrap(trivialProvider("v"), a)
	pB := tfexec.Wrap(trivialProvider("v"), b)

	require.False(t, invokeCreate(t, pA, "t_resource", map[string]any{"input": "1"}).HasError())
	require.False(t, invokeCreate(t, pA, "t_resource", map[string]any{"input": "2"}).HasError())
	// Reverse order in B.
	require.False(t, invokeCreate(t, pB, "t_resource", map[string]any{"input": "2"}).HasError())
	require.False(t, invokeCreate(t, pB, "t_resource", map[string]any{"input": "1"}).HasError())

	assert.Equal(t, a.Ops(), b.Ops())
}

// TestRecorder_OrderedOpsDifferWhenOrderDiffers proves the failure
// Case.OrderDeterministic exists to cause: two recorders fed the same calls
// in different orders compare equal under the default sorted comparison
// (Ops), so an ordering divergence between the runtimes passes silently —
// while the arrival-order comparison (OrderedOps) sees the divergence and
// fails. Each recorder's sequence is asserted in full to pin that OrderedOps
// reports true arrival order.
func TestRecorder_OrderedOpsDifferWhenOrderDiffers(t *testing.T) {
	t.Parallel()
	a, b := &tfexec.Recorder{}, &tfexec.Recorder{}
	pA := tfexec.Wrap(trivialProvider("v"), a)
	pB := tfexec.Wrap(trivialProvider("v"), b)

	require.False(t, invokeCreate(t, pA, "t_resource", map[string]any{"input": "1"}).HasError())
	require.False(t, invokeCreate(t, pA, "t_resource", map[string]any{"input": "2"}).HasError())
	// Reverse order in B.
	require.False(t, invokeCreate(t, pB, "t_resource", map[string]any{"input": "2"}).HasError())
	require.False(t, invokeCreate(t, pB, "t_resource", map[string]any{"input": "1"}).HasError())

	op := func(input string) tfexec.Op {
		return tfexec.Op{
			Kind:    tfexec.OpCreate,
			Type:    "t_resource",
			Inputs:  map[string]any{"input": input, "result": ""},
			Outputs: map[string]any{"input": input, "result": "v"},
		}
	}
	assert.Equal(t, []tfexec.Op{op("1"), op("2")}, a.OrderedOps())
	assert.Equal(t, []tfexec.Op{op("2"), op("1")}, b.OrderedOps())

	assert.Equal(t, a.Ops(), b.Ops(), "sorted comparison must tolerate the order divergence")
	assert.NotEqual(t, a.OrderedOps(), b.OrderedOps(), "ordered comparison must fail on the order divergence")
}

// TestRecorder_DifferentWhenInputsDiffer confirms inputs are part of the
// comparison surface: same operation kind + type but different inputs must
// produce unequal recordings.
func TestRecorder_DifferentWhenInputsDiffer(t *testing.T) {
	t.Parallel()
	a, b := &tfexec.Recorder{}, &tfexec.Recorder{}
	pA := tfexec.Wrap(trivialProvider("v"), a)
	pB := tfexec.Wrap(trivialProvider("v"), b)

	require.False(t, invokeCreate(t, pA, "t_resource", map[string]any{"input": "x"}).HasError())
	require.False(t, invokeCreate(t, pB, "t_resource", map[string]any{"input": "y"}).HasError())

	assert.NotEqual(t, a.Ops(), b.Ops())
}

// TestRecorder_DifferentWhenOutputsDiffer confirms outputs are also part of
// the comparison surface: same inputs but different computed result must
// produce unequal recordings.
func TestRecorder_DifferentWhenOutputsDiffer(t *testing.T) {
	t.Parallel()
	a, b := &tfexec.Recorder{}, &tfexec.Recorder{}
	pA := tfexec.Wrap(trivialProvider("alpha"), a)
	pB := tfexec.Wrap(trivialProvider("beta"), b)

	require.False(t, invokeCreate(t, pA, "t_resource", map[string]any{"input": "x"}).HasError())
	require.False(t, invokeCreate(t, pB, "t_resource", map[string]any{"input": "x"}).HasError())

	assert.NotEqual(t, a.Ops(), b.Ops())
}
