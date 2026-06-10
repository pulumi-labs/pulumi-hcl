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

package tfexec

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"sort"
	"sync"

	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// unknownSentinel stands in for unknown values in recorded ops. Both runtimes
// must agree on where unknowns appear for their recordings to compare equal.
const unknownSentinel = "<unknown>"

// WrapServer returns a tfprotov6.ProviderServer that delegates to srv and
// appends an Op to r for every resource CRUD operation and data-source read.
// It records at the protocol boundary — wire values are decoded with the
// provider's own schema — so it works for any provider implementation (e.g.
// terraform-plugin-framework) and, unlike a per-resource wrapper, preserves
// optional protocol capabilities such as MoveResourceState.
//
// Protocol recordings are stricter than Wrap's helper/schema snapshots: null
// and unknown values are preserved instead of being flattened to zero values.
func WrapServer(srv tfprotov6.ProviderServer, r *Recorder) tfprotov6.ProviderServer {
	return &recordingServer{ProviderServer: srv, rec: r}
}

type recordingServer struct {
	tfprotov6.ProviderServer
	rec *Recorder

	once        sync.Once
	schemaErr   error
	resources   map[string]tftypes.Type
	dataSources map[string]tftypes.Type
}

// objectTypes returns the object type of every resource and data source,
// fetched once from the underlying server's GetProviderSchema.
func (s *recordingServer) objectTypes(ctx context.Context) (resources, dataSources map[string]tftypes.Type, err error) {
	s.once.Do(func() {
		resp, err := s.GetProviderSchema(ctx, &tfprotov6.GetProviderSchemaRequest{})
		if err != nil {
			s.schemaErr = err
			return
		}
		s.resources = make(map[string]tftypes.Type, len(resp.ResourceSchemas))
		for name, sch := range resp.ResourceSchemas {
			s.resources[name] = sch.ValueType()
		}
		s.dataSources = make(map[string]tftypes.Type, len(resp.DataSourceSchemas))
		for name, sch := range resp.DataSourceSchemas {
			s.dataSources[name] = sch.ValueType()
		}
	})
	return s.resources, s.dataSources, s.schemaErr
}

func (s *recordingServer) ApplyResourceChange(
	ctx context.Context, req *tfprotov6.ApplyResourceChangeRequest,
) (*tfprotov6.ApplyResourceChangeResponse, error) {
	resources, _, err := s.objectTypes(ctx)
	if err != nil {
		return nil, err
	}
	typ, ok := resources[req.TypeName]
	if !ok {
		return nil, fmt.Errorf("recording: no schema for resource %q", req.TypeName)
	}
	prior, err := decodeObject(req.PriorState, typ)
	if err != nil {
		return nil, fmt.Errorf("recording: decode %s prior state: %w", req.TypeName, err)
	}
	planned, err := decodeObject(req.PlannedState, typ)
	if err != nil {
		return nil, fmt.Errorf("recording: decode %s planned state: %w", req.TypeName, err)
	}

	resp, err := s.ProviderServer.ApplyResourceChange(ctx, req)
	if err != nil {
		return resp, err
	}

	newState, err := decodeObject(resp.NewState, typ)
	if err != nil {
		return nil, fmt.Errorf("recording: decode %s new state: %w", req.TypeName, err)
	}

	// The protocol encodes the operation in the null-ness of the states: a
	// null planned state destroys, a null prior state creates.
	switch {
	case planned == nil:
		s.rec.add(Op{Kind: OpDelete, Type: req.TypeName, Inputs: prior, Outputs: nil})
	case prior == nil:
		s.rec.add(Op{Kind: OpCreate, Type: req.TypeName, Inputs: planned, Outputs: newState})
	default:
		s.rec.add(Op{Kind: OpUpdate, Type: req.TypeName, Inputs: planned, Outputs: newState})
	}
	return resp, nil
}

func (s *recordingServer) ReadResource(
	ctx context.Context, req *tfprotov6.ReadResourceRequest,
) (*tfprotov6.ReadResourceResponse, error) {
	resources, _, err := s.objectTypes(ctx)
	if err != nil {
		return nil, err
	}
	typ, ok := resources[req.TypeName]
	if !ok {
		return nil, fmt.Errorf("recording: no schema for resource %q", req.TypeName)
	}
	current, err := decodeObject(req.CurrentState, typ)
	if err != nil {
		return nil, fmt.Errorf("recording: decode %s current state: %w", req.TypeName, err)
	}

	resp, err := s.ProviderServer.ReadResource(ctx, req)
	if err != nil {
		return resp, err
	}

	newState, err := decodeObject(resp.NewState, typ)
	if err != nil {
		return nil, fmt.Errorf("recording: decode %s new state: %w", req.TypeName, err)
	}
	s.rec.add(Op{Kind: OpRead, Type: req.TypeName, Inputs: current, Outputs: newState})
	return resp, nil
}

func (s *recordingServer) ReadDataSource(
	ctx context.Context, req *tfprotov6.ReadDataSourceRequest,
) (*tfprotov6.ReadDataSourceResponse, error) {
	_, dataSources, err := s.objectTypes(ctx)
	if err != nil {
		return nil, err
	}
	typ, ok := dataSources[req.TypeName]
	if !ok {
		return nil, fmt.Errorf("recording: no schema for data source %q", req.TypeName)
	}
	config, err := decodeObject(req.Config, typ)
	if err != nil {
		return nil, fmt.Errorf("recording: decode %s config: %w", req.TypeName, err)
	}

	resp, err := s.ProviderServer.ReadDataSource(ctx, req)
	if err != nil {
		return resp, err
	}

	state, err := decodeObject(resp.State, typ)
	if err != nil {
		return nil, fmt.Errorf("recording: decode %s state: %w", req.TypeName, err)
	}
	s.rec.add(Op{Kind: OpDataSource, Type: req.TypeName, Inputs: config, Outputs: state})
	return resp, nil
}

// decodeObject unmarshals a wire value with the given object type and converts
// it to a JSON-comparable map. A nil or null value decodes to a nil map.
func decodeObject(dv *tfprotov6.DynamicValue, typ tftypes.Type) (map[string]any, error) {
	if dv == nil {
		return nil, nil
	}
	val, err := dv.Unmarshal(typ)
	if err != nil {
		return nil, err
	}
	if val.IsNull() {
		return nil, nil
	}
	var attrs map[string]tftypes.Value
	if err := val.As(&attrs); err != nil {
		return nil, err
	}
	out := make(map[string]any, len(attrs))
	for k, av := range attrs {
		conv, err := valueToAny(av)
		if err != nil {
			return nil, fmt.Errorf("attribute %q: %w", k, err)
		}
		out[k] = conv
	}
	return out, nil
}

// valueToAny converts a tftypes.Value to plain JSON-comparable Go values:
// nil for null, unknownSentinel for unknown, and float64/string/bool/[]any/
// map[string]any otherwise.
func valueToAny(v tftypes.Value) (any, error) {
	if v.IsNull() {
		return nil, nil
	}
	if !v.IsKnown() {
		return unknownSentinel, nil
	}
	switch typ := v.Type(); {
	case typ.Is(tftypes.String):
		var s string
		if err := v.As(&s); err != nil {
			return nil, err
		}
		return s, nil
	case typ.Is(tftypes.Bool):
		var b bool
		if err := v.As(&b); err != nil {
			return nil, err
		}
		return b, nil
	case typ.Is(tftypes.Number):
		f := new(big.Float)
		if err := v.As(&f); err != nil {
			return nil, err
		}
		n, _ := f.Float64()
		return n, nil
	case typ.Is(tftypes.Set{}):
		elems, err := elementsToAny(v)
		if err != nil {
			return nil, err
		}
		return sortByJSON(elems)
	case typ.Is(tftypes.List{}), typ.Is(tftypes.Tuple{}):
		return elementsToAny(v)
	case typ.Is(tftypes.Map{}), typ.Is(tftypes.Object{}):
		var attrs map[string]tftypes.Value
		if err := v.As(&attrs); err != nil {
			return nil, err
		}
		out := make(map[string]any, len(attrs))
		for k, av := range attrs {
			conv, err := valueToAny(av)
			if err != nil {
				return nil, fmt.Errorf("%q: %w", k, err)
			}
			out[k] = conv
		}
		return out, nil
	default:
		return nil, fmt.Errorf("unsupported value type %s", typ)
	}
}

func elementsToAny(v tftypes.Value) ([]any, error) {
	var elems []tftypes.Value
	if err := v.As(&elems); err != nil {
		return nil, err
	}
	out := make([]any, len(elems))
	for i, e := range elems {
		conv, err := valueToAny(e)
		if err != nil {
			return nil, fmt.Errorf("element %d: %w", i, err)
		}
		out[i] = conv
	}
	return out, nil
}

// sortByJSON orders set elements by their JSON encoding. Set element order on
// the wire is implementation-defined, so recordings from the two paths would
// otherwise spuriously diverge — the protocol-level analog of canonicalizeSets.
func sortByJSON(elems []any) ([]any, error) {
	type keyed struct {
		key  string
		elem any
	}
	pairs := make([]keyed, len(elems))
	for i, e := range elems {
		b, err := json.Marshal(e)
		if err != nil {
			return nil, err
		}
		pairs[i] = keyed{key: string(b), elem: e}
	}
	sort.Slice(pairs, func(i, j int) bool { return pairs[i].key < pairs[j].key })
	out := make([]any, len(pairs))
	for i, p := range pairs {
		out[i] = p.elem
	}
	return out, nil
}
