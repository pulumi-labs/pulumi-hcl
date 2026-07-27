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
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/pulumi/pulumi/pkg/v3/codegen/schema"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource/urn"
	"github.com/zclconf/go-cty/cty"

	"github.com/pulumi-labs/pulumi-hcl/pkg/hcl/ast"
	"github.com/pulumi-labs/pulumi-hcl/pkg/hcl/bridge"
	"github.com/pulumi-labs/pulumi-hcl/pkg/hcl/eval"
	"github.com/pulumi-labs/pulumi-hcl/pkg/hcl/packages"
	"github.com/pulumi-labs/pulumi-hcl/pkg/provisioner/runtime"
)

// destroyProvisionerHook is the single BeforeDelete hook name bound on every
// resource instance that declares a destroy-time provisioner. The name that
// lands in state must still resolve on every later run — including runs where
// the instance (or its whole block) is gone from the program — so it carries
// no per-instance, per-block, or per-component qualifier: any of those change
// out from under the state and brick the delete. One constant name, registered
// unconditionally every run, dispatched by URN.
const destroyProvisionerHook = "pulumi-hcl:provisioner:before-delete"

// destroyDispatch is the process-global dispatch table behind
// [destroyProvisionerHook]. It is process-global because one plugin process
// serves several engines (one per Construct) and several deployments (the
// preview and update halves of one `pulumi up`); entries are scoped by
// deployment key so those never see each other's snapshots.
var destroyDispatch = &dispatchTable{
	registered: map[string]bool{},
	instances:  map[dispatchKey]ResourceHookFunction{},
	blocks:     map[string][]*blockEntry{},
}

type dispatchKey struct{ deployment, urn string }

type dispatchTable struct {
	mu sync.RWMutex
	// registered marks deployments whose engine-side hook registry already
	// holds destroyProvisionerHook (registering a name twice errors).
	registered map[string]bool
	// instances maps a registered instance's URN to its destroy-provisioner
	// runner. Insertion overwrites: within one deployment a URN re-registered
	// later carries the fresher snapshot.
	instances map[dispatchKey]ResourceHookFunction
	// blocks holds one entry per resource block × live module instance that
	// declares destroy provisioners, recorded even when the block expands to
	// zero instances. They serve orphans whose block is still in config —
	// count = 0, a dropped for_each key, a module count shrink.
	blocks map[string][]*blockEntry
}

func (t *dispatchTable) claimRegistration(deployment string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.registered[deployment] {
		return false
	}
	t.registered[deployment] = true
	return true
}

func (t *dispatchTable) putInstance(deployment, urn string, run ResourceHookFunction) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.instances[dispatchKey{deployment, urn}] = run
}

func (t *dispatchTable) putBlock(deployment string, entry *blockEntry) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.blocks[deployment] = append(t.blocks[deployment], entry)
}

// dispatch resolves a BeforeDelete invocation to a resource instance. An
// instance registered this run matches by URN. Otherwise the instance is an
// orphan: if exactly one block entry matches its static config address, that
// block's destroy provisioners run; no match means the block is gone from the
// program and the delete proceeds without provisioners — TF's semantics for a
// removed block. Ambiguity no-ops with a warning: running the wrong command
// is worse than running none.
func (t *dispatchTable) dispatch(ctx context.Context, deployment string, args *ResourceHookArgs) error {
	t.mu.RLock()
	inst := t.instances[dispatchKey{deployment, args.URN}]
	blocks := t.blocks[deployment]
	t.mu.RUnlock()

	if inst != nil {
		return inst(ctx, args)
	}

	var matches []*blockEntry
	for _, b := range blocks {
		if b.matches(args) {
			matches = append(matches, b)
		}
	}
	switch len(matches) {
	case 0:
		return nil
	case 1:
		return matches[0].run(ctx, args)
	default:
		_ = matches[0].eng.resmon.LogWarning(ctx, fmt.Sprintf(
			"%s: destroy-time provisioners skipped: %d configuration blocks match this orphaned instance",
			args.URN, len(matches)))
		return nil
	}
}

// engineCounter hands out fallback deployment keys for engines constructed
// without one (unit tests), isolating each engine's dispatch entries.
var engineCounter atomic.Int64

// registerDestroyDispatcher registers the constant BeforeDelete hook, once per
// deployment (several engines can share one deployment on the MLC path).
// Registration failure warns and continues: it can only mean another
// pulumi-hcl process already registered the name, and those resources
// dispatch into that process's table — a bounded silent no-op, unlike a
// qualified name that bricks the delete outright.
func (e *Engine) registerDestroyDispatcher(ctx context.Context) {
	if e.resmon == nil {
		return
	}
	if !destroyDispatch.claimRegistration(e.deploymentKey) {
		return
	}
	deployment := e.deploymentKey
	callback := func(ctx context.Context, args *ResourceHookArgs) error {
		return destroyDispatch.dispatch(ctx, deployment, args)
	}
	if err := e.resmon.RegisterResourceHook(ctx, destroyProvisionerHook, callback, ResourceHookOptions{
		OnDryRun: false, // TF doesn't run provisioners during plan.
	}); err != nil {
		_ = e.resmon.LogWarning(ctx, fmt.Sprintf(
			"registering the destroy-time provisioner hook: %v", err))
	}
}

// blockEntry lets an orphaned instance whose resource block is still in the
// program run that block's destroy provisioners, keyed by the static config
// address (module block chain + resource block) rather than any instance name.
type blockEntry struct {
	eng       *Engine
	res       *ast.Resource
	resSchema *schema.Resource
	mapping   *bridge.BodyMapping

	// token is the Pulumi type token the block registers under; matched
	// against the orphan URN's type. A name shape alone collides across
	// types: aws_instance.web and aws_eip.web both register the name "web".
	token string
	// parentChain is the qualified parent-type chain of a URN registered by
	// this block, matched against the orphan URN's chain: two dynamically
	// loaded modules share one component token, so the chain (not the name)
	// is what a URN can still attest.
	parentChain string
	// prefix is the adapter's name prefix ("" on the langhost path, the
	// component name + "-" on the MLC path); stripped from args.Name before
	// shape matching, since pkg/server rewrites names after this package
	// sees them.
	prefix string
	// modPath is the static module block-name chain enclosing the resource
	// block, root → leaf. Matching accepts any instance key at each step: a
	// module count shrink orphans m[1].x while only m[0] is live.
	modPath []string
	// modInstName and evalCtx identify the live module instance that
	// recorded the entry; an orphan whose module prefix matches it exactly
	// (count = 0 inside a live module) evaluates in that instance's scope.
	modInstName string
	evalCtx     *eval.Context
	// overridden marks a live `pulumi { name = ... }` override on the block
	// or an enclosing module call. Overridden names are not invertible, so
	// the entry never matches by derived shape; the instance path (URN
	// equality) still serves the overridden resources that are registered.
	overridden bool
}

// logicalSeg is one dot-joined segment of a derived Pulumi logical name:
// `name`, `name[3]`, or `name["key"]`.
type logicalSeg struct {
	name  string
	index *int
	key   *string
}

// parseLogicalName splits a derived logical name into its segments. Keys are
// strconv.Quote-escaped and may contain dots, so the parse is a quote-aware
// left-to-right scan, not a split.
func parseLogicalName(s string) ([]logicalSeg, error) {
	var segs []logicalSeg
	for {
		i := 0
		for i < len(s) && s[i] != '[' && s[i] != '.' {
			i++
		}
		if i == 0 {
			return nil, fmt.Errorf("empty name segment")
		}
		seg := logicalSeg{name: s[:i]}
		s = s[i:]
		if strings.HasPrefix(s, "[") {
			rest := s[1:]
			switch {
			case strings.HasPrefix(rest, `"`):
				q, err := strconv.QuotedPrefix(rest)
				if err != nil {
					return nil, fmt.Errorf("malformed key segment: %w", err)
				}
				key, err := strconv.Unquote(q)
				if err != nil {
					return nil, fmt.Errorf("malformed key segment: %w", err)
				}
				seg.key = &key
				rest = rest[len(q):]
			default:
				j := 0
				for j < len(rest) && rest[j] >= '0' && rest[j] <= '9' {
					j++
				}
				if j == 0 {
					return nil, fmt.Errorf("malformed index segment")
				}
				n, err := strconv.Atoi(rest[:j])
				if err != nil {
					return nil, fmt.Errorf("malformed index segment: %w", err)
				}
				seg.index = &n
				rest = rest[j:]
			}
			if !strings.HasPrefix(rest, "]") {
				return nil, fmt.Errorf("unterminated key segment")
			}
			s = rest[1:]
		}
		segs = append(segs, seg)
		switch {
		case s == "":
			return segs, nil
		case strings.HasPrefix(s, "."):
			s = s[1:]
		default:
			return nil, fmt.Errorf("trailing characters in name segment")
		}
	}
}

// renderSegs is the inverse of parseLogicalName over a prefix of segments,
// re-encoding them the way buildResourceName/joinModuleName do.
func renderSegs(segs []logicalSeg) string {
	var b strings.Builder
	for i, s := range segs {
		if i > 0 {
			b.WriteByte('.')
		}
		b.WriteString(s.name)
		switch {
		case s.index != nil:
			b.WriteString("[" + strconv.Itoa(*s.index) + "]")
		case s.key != nil:
			b.WriteString("[" + strconv.Quote(*s.key) + "]")
		}
	}
	return b.String()
}

// urnParentChain returns the qualified type chain of a URN minus its own
// (final) type — the part contributed by the parent chain.
func urnParentChain(u string) string {
	qualified := urnQualifiedType(u)
	if i := strings.LastIndexByte(qualified, '$'); i >= 0 {
		return qualified[:i+1]
	}
	return ""
}

// urnQualifiedType extracts the qualified type portion of a URN string.
func urnQualifiedType(u string) string {
	parts := strings.Split(u, "::")
	if len(parts) < 3 {
		return ""
	}
	return parts[2]
}

func urnType(u string) string {
	qualified := urnQualifiedType(u)
	if i := strings.LastIndexByte(qualified, '$'); i >= 0 {
		return qualified[i+1:]
	}
	return qualified
}

func (b *blockEntry) matches(args *ResourceHookArgs) bool {
	if b.overridden {
		return false
	}
	if urnType(args.URN) != b.token || urnParentChain(args.URN) != b.parentChain {
		return false
	}
	name := args.Name
	if b.prefix != "" {
		var ok bool
		if name, ok = strings.CutPrefix(name, b.prefix); !ok {
			return false
		}
	}
	segs, err := parseLogicalName(name)
	if err != nil || len(segs) != len(b.modPath)+1 {
		return false
	}
	for i, mod := range b.modPath {
		if segs[i].name != mod {
			return false
		}
	}
	return segs[len(segs)-1].name == b.res.Name
}

// run evaluates the block's destroy provisioners for an orphaned instance.
// Scope comes in two rungs: the recording module instance's full evaluation
// context when the orphan's module prefix names it exactly (count = 0 inside
// a live module), else TF's own destroy scope — self, count.index, each.key,
// path.*, terraform.* — synthesized from the parsed instance keys. A rung-2
// body referencing anything else warns and no-ops rather than erroring: a
// BeforeDelete error fails the delete outright, recreating the undeletable
// resource this dispatcher exists to remove.
func (b *blockEntry) run(ctx context.Context, args *ResourceHookArgs) error {
	name := strings.TrimPrefix(args.Name, b.prefix)
	segs, err := parseLogicalName(name)
	if err != nil {
		return nil
	}
	leaf := segs[len(segs)-1]

	index := leaf.index
	// A bool-derived count yields one instance with no index in its name,
	// where TF supplies count.index = 0.
	if index == nil && b.res.Count != nil {
		zero := 0
		index = &zero
	}

	var hclCtx *hcl.EvalContext
	strict := false
	if renderSegs(segs[:len(segs)-1]) == b.modInstName {
		var keyVal *cty.Value
		if leaf.key != nil {
			v := cty.StringVal(*leaf.key)
			keyVal = &v
		}
		hclCtx = b.evalCtx.HCLContextWithIteration(index, keyVal, nil)
	} else {
		hclCtx = b.tfDestroyScope(index, leaf.key)
		strict = true
	}

	outputs := args.OldOutputs
	// terraform_data's output mirrors input; the registration-time trigger
	// tuple is gone with the instance, so triggers_replace stays absent and a
	// reference to it fails scope preflight rather than reporting a
	// fabricated value.
	if b.res.Type == packages.TerraformDataType {
		outputs = outputs.Set("output", outputs.Get("input"))
	}
	selfCtx, err := selfBoundEvalCtx(hclCtx, outputs, args.ID, args.URN, b.res.Type, b.resSchema, b.mapping, b.eng.dryRun)
	if err != nil {
		return fmt.Errorf("destroy provisioners for %s.%s: %w", b.res.Type, b.res.Name, err)
	}

	for i, prov := range b.res.Provisioners {
		if prov.When != "destroy" {
			continue
		}
		spec := &runtime.Spec{
			Type:   prov.Type,
			Config: prov.Config,
			Conn:   effectiveConnectionBody(prov, b.res),
		}
		if strict {
			if refErr := checkBodyReferences(selfCtx, spec.Config, spec.Conn); refErr != nil {
				_ = b.eng.resmon.LogWarning(ctx, fmt.Sprintf(
					"%s: destroy-time provisioner %d skipped: %v (only self, count.index, each.key, "+
						"path.* and terraform.* are in scope for an instance of a no-longer-live module instance)",
					args.URN, i+1, refErr))
				continue
			}
		}
		if runErr := runtime.Run(ctx, spec, selfCtx); runErr != nil {
			if prov.OnFailure == "continue" {
				continue
			}
			return fmt.Errorf("provisioner %d (%s) for %s.%s: %w", i+1, prov.Type, b.res.Type, b.res.Name, runErr)
		}
	}
	return nil
}

// tfDestroyScope builds TF's destroy-time evaluation scope: path.* and
// terraform.* copied from the recording context (both are static properties
// of the block, not of an instance), plus count.index/each.key synthesized
// from the orphan's parsed name.
func (b *blockEntry) tfDestroyScope(index *int, key *string) *hcl.EvalContext {
	vars := map[string]cty.Value{}
	parent := b.evalCtx.HCLContext()
	for _, name := range []string{"path", "terraform"} {
		if v, ok := lookupContextVariable(parent, name); ok {
			vars[name] = v
		}
	}
	if index != nil {
		vars["count"] = cty.ObjectVal(map[string]cty.Value{"index": cty.NumberIntVal(int64(*index))})
	}
	if key != nil {
		vars["each"] = cty.ObjectVal(map[string]cty.Value{"key": cty.StringVal(*key)})
	}
	return &hcl.EvalContext{Variables: vars, Functions: parent.Functions}
}

func lookupContextVariable(ctx *hcl.EvalContext, name string) (cty.Value, bool) {
	for c := ctx; c != nil; c = c.Parent() {
		if v, ok := c.Variables[name]; ok {
			return v, true
		}
	}
	return cty.NilVal, false
}

// checkBodyReferences resolves every variable reference in the given bodies
// against ctx, so an out-of-scope reference is detected before any command in
// the provisioner runs.
func checkBodyReferences(ctx *hcl.EvalContext, bodies ...hcl.Body) error {
	var walk func(body hcl.Body) error
	walk = func(body hcl.Body) error {
		if body == nil {
			return nil
		}
		if eb, ok := body.(*ast.EscapedBody); ok {
			if err := walk(eb.Base); err != nil {
				return err
			}
			return walk(eb.Escape)
		}
		attrs, _ := body.JustAttributes()
		for _, attr := range attrs {
			for _, traversal := range attr.Expr.Variables() {
				if _, diags := traversal.TraverseAbs(ctx); diags.HasErrors() {
					return fmt.Errorf("%s", diags.Error())
				}
			}
		}
		if syntaxBody, ok := body.(*hclsyntax.Body); ok {
			for _, block := range syntaxBody.Blocks {
				if err := walk(block.Body); err != nil {
					return err
				}
			}
		}
		return nil
	}
	for _, body := range bodies {
		if err := walk(body); err != nil {
			return err
		}
	}
	return nil
}

// recordBlockEntry records the dispatch entry for one resource block × live
// module instance, when the block declares destroy provisioners. Called from
// cell expansion whether or not the block expands to any instances; not
// called when the cell body never runs (unknown count/for_each, failed
// dependency), whose absence can only turn a concurrent orphan delete into a
// no-op on an already-failing deployment.
func (e *Engine) recordBlockEntry(
	ctx context.Context, res *ast.Resource, resSchema *schema.Resource,
	evalCtx *eval.Context, parentURN urn.URN, modInst *moduleInstance,
) {
	if e.resmon == nil || !hasDestroyProvisioners(res) {
		return
	}

	probeURN, probeName := e.resmon.ResolveURN(parentURN, resSchema.Token, "probe")
	prefix := strings.TrimSuffix(probeName, "probe")

	entry := &blockEntry{
		eng:         e,
		res:         res,
		resSchema:   resSchema,
		mapping:     e.resolver.ResourceBodyMapping(ctx, res.Type),
		token:       resSchema.Token,
		parentChain: urnParentChain(string(probeURN)),
		prefix:      prefix,
		evalCtx:     evalCtx,
	}
	if modInst != nil {
		entry.modInstName = modInst.Name
		for s := range modInst.Path.Steps {
			entry.modPath = append(entry.modPath, s.Name())
		}
		// An overridden enclosing module name is not derived from the static
		// path, so shape matching cannot attribute an orphan to this block.
		if modInst.Name != modInst.Path.LogicalName() {
			entry.overridden = true
		}
	}
	if res.PulumiName != nil {
		_, live, err := evaluatePulumiName(res.PulumiName, evalCtx.HCLContext(), res.Type+"."+res.Name)
		// An override that needs per-instance scope (count.index) cannot be
		// judged here; treat it as live rather than guess.
		if err != nil || live {
			entry.overridden = true
		}
	}

	destroyDispatch.putBlock(e.deploymentKey, entry)
}

func hasDestroyProvisioners(res *ast.Resource) bool {
	for _, prov := range res.Provisioners {
		if prov.When == "destroy" {
			return true
		}
	}
	return false
}
