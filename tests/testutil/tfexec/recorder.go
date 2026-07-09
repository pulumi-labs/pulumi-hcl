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
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"slices"
	"sort"
	"sync"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/pulumi/pulumi/sdk/v3/go/common/util/contract"
)

// OpKind classifies a recorded provider operation.
type OpKind int

const (
	OpCreate OpKind = iota
	OpRead
	OpUpdate
	OpDelete
	OpDataSource
	OpCallFunction
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

// OrderedOps returns a copy of recorded ops in arrival order.
func (r *Recorder) OrderedOps() []Op {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.ops)
}

// Ops returns a copy of recorded ops, sorted by (Kind, Type, serialized
// Inputs, serialized Outputs) so set-equality between two recorders is
// order-independent. Outputs participate in the key because ops can tie on
// inputs (e.g. several imports of one resource type read distinct ids, with no
// inputs set before the Read) while execution order differs across paths.
func (r *Recorder) Ops() []Op {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := slices.Clone(r.ops)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Kind != out[j].Kind {
			return out[i].Kind < out[j].Kind
		}
		if out[i].Type != out[j].Type {
			return out[i].Type < out[j].Type
		}
		ai, _ := json.Marshal(out[i].Inputs)
		aj, _ := json.Marshal(out[j].Inputs)
		if string(ai) != string(aj) {
			return string(ai) < string(aj)
		}
		bi, _ := json.Marshal(out[i].Outputs)
		bj, _ := json.Marshal(out[j].Outputs)
		return string(bi) < string(bj)
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

// WithStore returns a *schema.Provider with cloned ResourcesMap where every
// resource gains import support: a passthrough importer, Create/Update
// snapshot attributes into store, and Read hydrates from that snapshot (see
// ImportStore). Compose with Wrap as WithStore(Wrap(p, r), store) so
// hydration happens before the recorder snapshots a Read's inputs.
func WithStore(p *schema.Provider, store *ImportStore) *schema.Provider {
	wrapped := *p
	wrapped.ResourcesMap = make(map[string]*schema.Resource, len(p.ResourcesMap))
	for typeName, res := range p.ResourcesMap {
		wrapped.ResourcesMap[typeName] = withStoreResource(typeName, res, store)
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

func withStoreResource(typeName string, res *schema.Resource, store *ImportStore) *schema.Resource {
	clone := *res
	origCreate := res.CreateContext
	origRead := res.ReadContext
	origUpdate := res.UpdateContext
	origDelete := res.DeleteContext

	if clone.Importer == nil {
		clone.Importer = &schema.ResourceImporter{StateContext: schema.ImportStatePassthroughContext}
	}

	if origCreate != nil {
		clone.CreateContext = func(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
			inputs := snapshot(d, res.Schema)
			diags := origCreate(ctx, d, meta)
			if !diags.HasError() && d.Id() != "" {
				// Test providers use fixed ids ("simple-id"), which would
				// collide in the store across instances of one type. Both
				// driver paths derive the same suffix, so cross-path
				// comparisons stay equal.
				d.SetId(d.Id() + "-" + fingerprint(inputs))
				store.put(typeName, d.Id(), snapshot(d, res.Schema))
			}
			return diags
		}
	}
	if origRead != nil {
		clone.ReadContext = func(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
			// A Read reached via import passthrough sees only the resource
			// id; on refresh Reads the snapshot matches current state and
			// hydration is a no-op.
			if snap := store.get(typeName, d.Id()); snap != nil {
				for k, v := range snap {
					if _, ok := res.Schema[k]; ok && v != nil {
						contract.IgnoreError(d.Set(k, v))
					}
				}
			}
			return origRead(ctx, d, meta)
		}
	}
	if origUpdate != nil {
		clone.UpdateContext = func(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
			diags := origUpdate(ctx, d, meta)
			if !diags.HasError() {
				store.put(typeName, d.Id(), snapshot(d, res.Schema))
			}
			return diags
		}
	}
	if origDelete != nil {
		clone.DeleteContext = func(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
			diags := origDelete(ctx, d, meta)
			if !diags.HasError() {
				store.delete(typeName, d.Id())
			}
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

// fingerprint hashes a snapshot deterministically (json.Marshal sorts map
// keys, so both driver paths agree). Instances with identical inputs share an
// id and store entry — harmless, as their snapshots are identical too.
func fingerprint(snapshot map[string]any) string {
	b, err := json.Marshal(snapshot)
	// Snapshots already survived a JSON roundtrip; failing here means the
	// wrapper is broken, so don't degrade into silent id collisions.
	contract.AssertNoErrorf(err, "marshaling snapshot for id fingerprint")
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:4])
}

// snapshot copies every value the resource schema knows about into a plain
// map[string]any. JSON-roundtripping yields a representation that is identical
// regardless of whether the caller is the TF gRPC layer or the bridged provider.
func snapshot(d *schema.ResourceData, sch map[string]*schema.Schema) map[string]any {
	out := make(map[string]any, len(sch))
	for k := range sch {
		v := canonicalizeSets(d.Get(k))
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

// canonicalizeSets replaces TF's *schema.Set values with the equivalent
// ordered []any, recursing through lists and maps so nested sets are handled
// too. json.Marshal can't encode a *schema.Set (its hash func field), and two
// equal-content sets compare unequal across paths because those func fields
// differ — so without this every set-typed attribute spuriously diverges.
func canonicalizeSets(v any) any {
	switch t := v.(type) {
	case *schema.Set:
		list := t.List()
		out := make([]any, len(list))
		for i, e := range list {
			out[i] = canonicalizeSets(e)
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, e := range t {
			out[i] = canonicalizeSets(e)
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, e := range t {
			out[k] = canonicalizeSets(e)
		}
		return out
	default:
		return v
	}
}
