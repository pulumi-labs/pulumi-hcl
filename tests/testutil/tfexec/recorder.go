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
	"sort"
	"sync"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

// OpKind classifies a recorded provider operation.
type OpKind int

const (
	OpCreate OpKind = iota
	OpRead
	OpUpdate
	OpDelete
	OpDataSource
)

// Op is a single recorded provider operation, captured at the schema.Provider
// boundary. Inputs/Outputs are normalized via JSON so the recordings from the
// TF reattach path and the bridged-provider path compare directly.
type Op struct {
	Kind    OpKind
	Type    string
	Inputs  map[string]any
	Outputs map[string]any
}

// Recorder collects Ops appended by wrapped providers. Safe for concurrent use.
type Recorder struct {
	mu  sync.Mutex
	ops []Op
}

func (r *Recorder) add(op Op) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ops = append(r.ops, op)
}

// Ops returns a copy of recorded ops, sorted by (Kind, Type, serialized Inputs)
// so set-equality between two recorders is order-independent.
func (r *Recorder) Ops() []Op {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]Op, len(r.ops))
	copy(out, r.ops)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Kind != out[j].Kind {
			return out[i].Kind < out[j].Kind
		}
		if out[i].Type != out[j].Type {
			return out[i].Type < out[j].Type
		}
		ai, _ := json.Marshal(out[i].Inputs)
		aj, _ := json.Marshal(out[j].Inputs)
		return string(ai) < string(aj)
	})
	return out
}

// Wrap returns a *schema.Provider with cloned ResourcesMap and DataSourcesMap
// where each resource/data-source's Context CRUD funcs delegate to the original
// and append an Op to r. The original *schema.Provider is not modified.
func Wrap(p *schema.Provider, r *Recorder) *schema.Provider {
	wrapped := *p
	wrapped.ResourcesMap = make(map[string]*schema.Resource, len(p.ResourcesMap))
	for typeName, res := range p.ResourcesMap {
		wrapped.ResourcesMap[typeName] = wrapResource(typeName, res, r)
	}
	wrapped.DataSourcesMap = make(map[string]*schema.Resource, len(p.DataSourcesMap))
	for typeName, ds := range p.DataSourcesMap {
		wrapped.DataSourcesMap[typeName] = wrapDataSource(typeName, ds, r)
	}
	return &wrapped
}

func wrapResource(typeName string, res *schema.Resource, r *Recorder) *schema.Resource {
	clone := *res
	origCreate := res.CreateContext
	origRead := res.ReadContext
	origUpdate := res.UpdateContext
	origDelete := res.DeleteContext

	if origCreate != nil {
		clone.CreateContext = func(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
			inputs := snapshot(d, res.Schema)
			diags := origCreate(ctx, d, meta)
			outputs := snapshot(d, res.Schema)
			r.add(Op{Kind: OpCreate, Type: typeName, Inputs: inputs, Outputs: outputs})
			return diags
		}
	}
	if origRead != nil {
		clone.ReadContext = func(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
			inputs := snapshot(d, res.Schema)
			diags := origRead(ctx, d, meta)
			outputs := snapshot(d, res.Schema)
			r.add(Op{Kind: OpRead, Type: typeName, Inputs: inputs, Outputs: outputs})
			return diags
		}
	}
	if origUpdate != nil {
		clone.UpdateContext = func(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
			inputs := snapshot(d, res.Schema)
			diags := origUpdate(ctx, d, meta)
			outputs := snapshot(d, res.Schema)
			r.add(Op{Kind: OpUpdate, Type: typeName, Inputs: inputs, Outputs: outputs})
			return diags
		}
	}
	if origDelete != nil {
		clone.DeleteContext = func(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
			inputs := snapshot(d, res.Schema)
			diags := origDelete(ctx, d, meta)
			r.add(Op{Kind: OpDelete, Type: typeName, Inputs: inputs, Outputs: nil})
			return diags
		}
	}
	return &clone
}

func wrapDataSource(typeName string, ds *schema.Resource, r *Recorder) *schema.Resource {
	clone := *ds
	origRead := ds.ReadContext
	if origRead != nil {
		clone.ReadContext = func(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
			inputs := snapshot(d, ds.Schema)
			diags := origRead(ctx, d, meta)
			outputs := snapshot(d, ds.Schema)
			r.add(Op{Kind: OpDataSource, Type: typeName, Inputs: inputs, Outputs: outputs})
			return diags
		}
	}
	return &clone
}

// snapshot copies every value the resource schema knows about into a plain
// map[string]any. JSON-roundtripping yields a representation that is identical
// regardless of whether the caller is the TF gRPC layer or the bridged provider.
func snapshot(d *schema.ResourceData, sch map[string]*schema.Schema) map[string]any {
	out := make(map[string]any, len(sch))
	for k := range sch {
		v := d.Get(k)
		// Roundtrip through JSON to canonicalize types (TF's set/map shapes
		// can otherwise compare unequal across paths).
		b, err := json.Marshal(v)
		if err != nil {
			out[k] = v
			continue
		}
		var n any
		if err := json.Unmarshal(b, &n); err != nil {
			out[k] = v
			continue
		}
		out[k] = n
	}
	return out
}
