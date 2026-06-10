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

	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/pulumi-labs/pulumi-hcl/tests/testutil/tfexec"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var thingSchema = &tfprotov6.Schema{
	Block: &tfprotov6.SchemaBlock{
		Attributes: []*tfprotov6.SchemaAttribute{
			{Name: "input", Type: tftypes.String, Optional: true},
			{Name: "result", Type: tftypes.String, Computed: true},
		},
	},
}

// fakeServer is a minimal tfprotov6.ProviderServer with one resource:
// ApplyResourceChange echoes the planned state with `result` set to the
// configured value. Only the methods the recorder uses are implemented; the
// embedded nil interface panics on anything else.
type fakeServer struct {
	tfprotov6.ProviderServer
	result string
}

func (s *fakeServer) GetProviderSchema(
	context.Context, *tfprotov6.GetProviderSchemaRequest,
) (*tfprotov6.GetProviderSchemaResponse, error) {
	return &tfprotov6.GetProviderSchemaResponse{
		ResourceSchemas: map[string]*tfprotov6.Schema{"t_resource": thingSchema},
	}, nil
}

func (s *fakeServer) ApplyResourceChange(
	_ context.Context, req *tfprotov6.ApplyResourceChangeRequest,
) (*tfprotov6.ApplyResourceChangeResponse, error) {
	typ := thingSchema.ValueType()
	planned, err := req.PlannedState.Unmarshal(typ)
	if err != nil {
		return nil, err
	}
	if planned.IsNull() {
		state, err := tfprotov6.NewDynamicValue(typ, tftypes.NewValue(typ, nil))
		if err != nil {
			return nil, err
		}
		return &tfprotov6.ApplyResourceChangeResponse{NewState: &state}, nil
	}
	var attrs map[string]tftypes.Value
	if err := planned.As(&attrs); err != nil {
		return nil, err
	}
	attrs["result"] = tftypes.NewValue(tftypes.String, s.result)
	state, err := tfprotov6.NewDynamicValue(typ, tftypes.NewValue(typ, attrs))
	if err != nil {
		return nil, err
	}
	return &tfprotov6.ApplyResourceChangeResponse{NewState: &state}, nil
}

// thingState builds a wire value for t_resource. nil means a null state.
func thingState(t *testing.T, attrs map[string]tftypes.Value) *tfprotov6.DynamicValue {
	t.Helper()
	typ := thingSchema.ValueType()
	var val tftypes.Value
	if attrs == nil {
		val = tftypes.NewValue(typ, nil)
	} else {
		val = tftypes.NewValue(typ, attrs)
	}
	dv, err := tfprotov6.NewDynamicValue(typ, val)
	require.NoError(t, err)
	return &dv
}

func apply(t *testing.T, srv tfprotov6.ProviderServer, prior, planned *tfprotov6.DynamicValue) {
	t.Helper()
	_, err := srv.ApplyResourceChange(t.Context(), &tfprotov6.ApplyResourceChangeRequest{
		TypeName:     "t_resource",
		PriorState:   prior,
		PlannedState: planned,
	})
	require.NoError(t, err)
}

func strVal(s string) tftypes.Value { return tftypes.NewValue(tftypes.String, s) }

// TestWrapServer_ClassifiesOps drives a create (null prior), an update (both
// states set), and a delete (null planned) through the wrapper and asserts
// the recorded ops in full: kind classification, decoded inputs, and outputs.
func TestWrapServer_ClassifiesOps(t *testing.T) {
	t.Parallel()
	r := &tfexec.Recorder{}
	srv := tfexec.WrapServer(&fakeServer{result: "computed"}, r)

	created := map[string]tftypes.Value{"input": strVal("a"), "result": strVal("computed")}
	apply(t, srv, thingState(t, nil),
		thingState(t, map[string]tftypes.Value{"input": strVal("a"), "result": tftypes.NewValue(tftypes.String, nil)}))
	apply(t, srv, thingState(t, created),
		thingState(t, map[string]tftypes.Value{"input": strVal("b"), "result": strVal("computed")}))
	apply(t, srv, thingState(t, created), thingState(t, nil))

	assert.Equal(t, []tfexec.Op{
		{
			Kind:    tfexec.OpCreate,
			Type:    "t_resource",
			Inputs:  map[string]any{"input": "a", "result": nil},
			Outputs: map[string]any{"input": "a", "result": "computed"},
		},
		{
			Kind:    tfexec.OpUpdate,
			Type:    "t_resource",
			Inputs:  map[string]any{"input": "b", "result": "computed"},
			Outputs: map[string]any{"input": "b", "result": "computed"},
		},
		{
			Kind:    tfexec.OpDelete,
			Type:    "t_resource",
			Inputs:  map[string]any{"input": "a", "result": "computed"},
			Outputs: nil,
		},
	}, r.Ops())
}

// TestWrapServer_EqualWhenSameBehavior verifies that two wrapped servers with
// identical behavior produce equal recordings.
func TestWrapServer_EqualWhenSameBehavior(t *testing.T) {
	t.Parallel()
	a, b := &tfexec.Recorder{}, &tfexec.Recorder{}
	srvA := tfexec.WrapServer(&fakeServer{result: "v"}, a)
	srvB := tfexec.WrapServer(&fakeServer{result: "v"}, b)

	planned := map[string]tftypes.Value{"input": strVal("x"), "result": tftypes.NewValue(tftypes.String, nil)}
	apply(t, srvA, thingState(t, nil), thingState(t, planned))
	apply(t, srvB, thingState(t, nil), thingState(t, planned))

	assert.Equal(t, a.Ops(), b.Ops())
}

// TestWrapServer_DifferentWhenOutputsDiffer confirms a behavioral divergence
// between the two paths is observed: same planned state, different computed
// result, unequal recordings.
func TestWrapServer_DifferentWhenOutputsDiffer(t *testing.T) {
	t.Parallel()
	a, b := &tfexec.Recorder{}, &tfexec.Recorder{}
	srvA := tfexec.WrapServer(&fakeServer{result: "alpha"}, a)
	srvB := tfexec.WrapServer(&fakeServer{result: "beta"}, b)

	planned := map[string]tftypes.Value{"input": strVal("x"), "result": tftypes.NewValue(tftypes.String, nil)}
	apply(t, srvA, thingState(t, nil), thingState(t, planned))
	apply(t, srvB, thingState(t, nil), thingState(t, planned))

	assert.NotEqual(t, a.Ops(), b.Ops())
}

// TestWrapServer_DifferentWhenUnknownsDiffer confirms the recorder preserves
// the null/unknown distinction: planned states that differ only in an unknown
// versus a null computed attribute produce unequal recordings.
func TestWrapServer_DifferentWhenUnknownsDiffer(t *testing.T) {
	t.Parallel()
	a, b := &tfexec.Recorder{}, &tfexec.Recorder{}
	srvA := tfexec.WrapServer(&fakeServer{result: "v"}, a)
	srvB := tfexec.WrapServer(&fakeServer{result: "v"}, b)

	apply(t, srvA, thingState(t, nil), thingState(t, map[string]tftypes.Value{
		"input": strVal("x"), "result": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
	}))
	apply(t, srvB, thingState(t, nil), thingState(t, map[string]tftypes.Value{
		"input": strVal("x"), "result": tftypes.NewValue(tftypes.String, nil),
	}))

	assert.NotEqual(t, a.Ops(), b.Ops())
}
