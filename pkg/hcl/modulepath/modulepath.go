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

// Package modulepath identifies a particular module instance in the
// nesting tree of an HCL configuration.
//
// A [Path] is composed of [Step]s, one per module call traversed from the
// root config down to the instance in question. The empty path is the
// root configuration. A [Step] carries the module call's block label plus
// the optional disambiguator (count index or for_each key) that identifies
// which instance of that call we mean.
//
// The internal byte layout of a Path is an implementation detail and is
// explicitly NOT a stable encoding: do not persist values of this type or
// rely on the bytes returned by any private method to round-trip across
// process boundaries. The package's value comes from giving callers a
// comparable, hashable handle that supports unambiguous derivation of
// Pulumi logical names regardless of label content.
package modulepath

import (
	"cmp"
	"encoding/binary"
	"fmt"
	"strconv"
	"strings"
)

// stepKind tags the disambiguator carried by a [Step].
type stepKind uint8

const (
	stepKindBare    stepKind = 0
	stepKindIndex   stepKind = 1
	stepKindForEach stepKind = 2
)

// Step is one element of a [Path]: a module call's block label plus the
// optional count index or for_each key that selects a particular instance
// of that call.
//
// Construct values via [NewStep], [NewIndexedStep], or [NewKeyedStep].
type Step struct {
	name string
	kind stepKind
	idx  uint64
	key  string
}

// NewStep returns a Step for a module call with no count or for_each
// expansion.
func NewStep(name string) Step {
	return Step{name: name, kind: stepKindBare}
}

// NewIndexedStep returns a Step for a particular instance of a count-expanded
// module call. index must be non-negative.
func NewIndexedStep(name string, index int) Step {
	if index < 0 {
		panic(fmt.Sprintf("modulepath: index must be non-negative, got %d", index))
	}
	return Step{name: name, kind: stepKindIndex, idx: uint64(index)}
}

// NewKeyedStep returns a Step for a particular instance of a for_each-expanded
// module call.
func NewKeyedStep(name string, key string) Step {
	return Step{name: name, kind: stepKindForEach, key: key}
}

// Name returns the block label of this step (e.g. "vpc", "vpc.primary").
func (s Step) Name() string { return s.name }

// Key returns the for_each key of this step. ok is false if the step is not
// from a for_each expansion.
func (s Step) Key() (key string, ok bool) {
	if s.kind != stepKindForEach {
		return "", false
	}
	return s.key, true
}

// Index returns the count index of this step. ok is false if the step is not
// from a count expansion.
func (s Step) Index() (index int, ok bool) {
	if s.kind != stepKindIndex {
		return 0, false
	}
	return int(s.idx), true
}

// Config returns s with its instance disambiguator stripped: the step that
// names the static config block rather than one of its instances.
func (s Step) Config() Step {
	return Step{name: s.name, kind: stepKindBare}
}

// LogicalName returns the Pulumi logical name component for this single
// step, using Terraform-style instance addressing:
//
//   - `<name>`          if the step has no count/for_each.
//   - `<name>[<index>]` for count instances.
//   - `<name>["<key>"]` for for_each instances, with the key quoted via
//     [strconv.Quote] so keys containing `"` or `\` stay unambiguous.
//
// Because `[` and `"` cannot appear in an HCL block label, distinct
// (name, key) pairs always produce distinct logical names.
func (s Step) LogicalName() string {
	switch s.kind {
	case stepKindIndex:
		return s.name + "[" + strconv.FormatUint(s.idx, 10) + "]"
	case stepKindForEach:
		return s.name + "[" + strconv.Quote(s.key) + "]"
	default:
		return s.name
	}
}

// String returns a diagnostic representation of this step using
// Terraform-style addressing (module["name"], module["name"][0],
// module["name"]["key"]).
//
// Don't parse this. Use the typed accessors instead.
func (s Step) String() string {
	switch s.kind {
	case stepKindIndex:
		return fmt.Sprintf("module[%q][%d]", s.name, s.idx)
	case stepKindForEach:
		return fmt.Sprintf("module[%q][%q]", s.name, s.key)
	default:
		return fmt.Sprintf("module[%q]", s.name)
	}
}

// Path identifies a particular module instance in the nesting tree
// rooted at the top-level config.
//
// The zero value [Path] is the root (no enclosing module). Paths are
// comparable in Go, hashable as map keys, and immutable.
type Path struct {
	// The internal representation is a length-prefixed binary string. There
	// is no marshal API because there is no stable on-disk format. Use
	// [Path.String] for human-readable diagnostics; use the typed methods
	// for everything else.

	repr string
}

// Root returns the empty Path (the root configuration).
func Root() Path { return Path{} }

// Len returns the number of steps in p.
func (p Path) Len() int {
	n := 0
	for range p.Steps {
		n++
	}
	return n
}

// IsRoot reports whether p has zero steps.
func (p Path) IsRoot() bool { return p == Path{} }

// Append returns a new Path with s added to the end. p is not mutated.
func (p Path) Append(s Step) Path {
	var b strings.Builder
	b.Grow(len(p.repr) + estimateStepSize(s))
	b.WriteString(p.repr)
	encodeStep(&b, s)
	return Path{repr: b.String()}
}

// Config returns p with every step's instance disambiguator stripped: the
// static config address of the block, shared by all of its instances.
func (p Path) Config() Path {
	config := Root()
	for s := range p.Steps {
		config = config.Append(s.Config())
	}
	return config
}

// Compare returns a deterministic total order over paths for stable sorting:
// step-wise by (name, kind, index, key), with a shorter path ordering before
// its extensions. Compare(p, q) == 0 iff p == q.
func Compare(p, q Path) int {
	pSteps, qSteps := p.stepSlice(), q.stepSlice()
	for i := range min(len(pSteps), len(qSteps)) {
		a, b := pSteps[i], qSteps[i]
		if c := strings.Compare(a.name, b.name); c != 0 {
			return c
		}
		if c := cmp.Compare(a.kind, b.kind); c != 0 {
			return c
		}
		if c := cmp.Compare(a.idx, b.idx); c != 0 {
			return c
		}
		if c := strings.Compare(a.key, b.key); c != 0 {
			return c
		}
	}
	return cmp.Compare(len(pSteps), len(qSteps))
}

func (p Path) stepSlice() []Step {
	steps := make([]Step, 0, p.Len())
	for s := range p.Steps {
		steps = append(steps, s)
	}
	return steps
}

// Parent returns the path with the last step removed plus that last step.
// ok is false if p is the root.
func (p Path) Parent() (parent Path, last Step, ok bool) {
	if p.repr == "" {
		return Path{}, Step{}, false
	}
	bytes := []byte(p.repr)
	lastStart := 0
	cursor := 0
	var lastStep Step
	for cursor < len(bytes) {
		lastStart = cursor
		next, s := decodeStep(bytes, cursor)
		lastStep = s
		cursor = next
	}
	return Path{repr: string(bytes[:lastStart])}, lastStep, true
}

// Steps yields the steps of p in order. Designed for `for s := range p.Steps`.
func (p Path) Steps(yield func(Step) bool) {
	bytes := []byte(p.repr)
	cursor := 0
	for cursor < len(bytes) {
		next, s := decodeStep(bytes, cursor)
		if !yield(s) {
			return
		}
		cursor = next
	}
}

// LogicalName returns the canonical derived Pulumi resource name for this
// path (joining each step's [Step.LogicalName] with ".").
//
//	Root                              -> ""
//	["outer"]                         -> "outer"
//	["outer", "inner"]                -> "outer.inner"
//	["outer"[0], "inner"]             -> "outer[0].inner"
//	["outer"["a"], "inner"]           -> `outer["a"].inner`
//
// HCL block labels cannot contain ".", "[", or `"`, and instance keys are
// bracket-quoted, so distinct paths built from valid HCL labels always
// produce distinct logical names. Labels that themselves contain "." (which
// HCL cannot express) are not escaped; for such pathological labels this is
// a one-way derivation, not a round-trippable encoding of the path.
func (p Path) LogicalName() string {
	var b strings.Builder
	first := true
	for s := range p.Steps {
		if !first {
			b.WriteByte('.')
		}
		b.WriteString(s.LogicalName())
		first = false
	}
	return b.String()
}

// ParseLogicalName parses the [Path.LogicalName] encoding back into a Path.
// The scan is quote-aware rather than a split on ".": for_each keys are
// [strconv.Quote]-escaped and may contain dots and brackets.
func ParseLogicalName(s string) (Path, error) {
	p := Root()
	for {
		i := 0
		for i < len(s) && s[i] != '[' && s[i] != '.' {
			i++
		}
		if i == 0 {
			return Path{}, fmt.Errorf("modulepath: empty name segment")
		}
		name := s[:i]
		s = s[i:]
		step := NewStep(name)
		if strings.HasPrefix(s, "[") {
			rest := s[1:]
			switch {
			case strings.HasPrefix(rest, `"`):
				q, err := strconv.QuotedPrefix(rest)
				if err != nil {
					return Path{}, fmt.Errorf("modulepath: malformed key segment after %q: %w", name, err)
				}
				key, err := strconv.Unquote(q)
				if err != nil {
					return Path{}, fmt.Errorf("modulepath: malformed key segment after %q: %w", name, err)
				}
				// Reject non-canonical quoting so parse and render stay
				// exact inverses.
				if strconv.Quote(key) != q {
					return Path{}, fmt.Errorf("modulepath: non-canonical key segment after %q", name)
				}
				step = NewKeyedStep(name, key)
				rest = rest[len(q):]
			default:
				j := 0
				for j < len(rest) && rest[j] >= '0' && rest[j] <= '9' {
					j++
				}
				index, err := strconv.Atoi(rest[:j])
				if err != nil {
					return Path{}, fmt.Errorf("modulepath: malformed index segment after %q: %w", name, err)
				}
				if j > 1 && rest[0] == '0' {
					return Path{}, fmt.Errorf("modulepath: non-canonical index segment after %q", name)
				}
				step = NewIndexedStep(name, index)
				rest = rest[j:]
			}
			if !strings.HasPrefix(rest, "]") {
				return Path{}, fmt.Errorf("modulepath: unterminated instance-key segment after %q", name)
			}
			s = rest[1:]
		}
		p = p.Append(step)
		switch {
		case s == "":
			return p, nil
		case strings.HasPrefix(s, "."):
			s = s[1:]
		default:
			return Path{}, fmt.Errorf("modulepath: unexpected character after segment %q", name)
		}
	}
}

// String returns a diagnostic representation joining each step with ".".
//
// Don't parse this. Use [Path.Steps] or the typed accessors instead.
func (p Path) String() string {
	if p.IsRoot() {
		return ""
	}
	parts := make([]string, 0, p.Len())
	for s := range p.Steps {
		parts = append(parts, s.String())
	}
	return strings.Join(parts, ".")
}

// Address identifies a resource within the module tree: the enclosing module
// path plus the leaf's own step. Like [Path] it is an immutable, comparable
// value; keyless steps make it a config address, keyed steps an instance.
type Address struct {
	module   Path
	resource Step
}

// NewAddress returns the address of resource within the module instance (or
// config block) at module.
func NewAddress(module Path, resource Step) Address {
	return Address{module: module, resource: resource}
}

// Module returns the enclosing module path.
func (a Address) Module() Path { return a.module }

// Resource returns the leaf step.
func (a Address) Resource() Step { return a.resource }

// Config returns a with every instance disambiguator stripped: the static
// config address shared by all of the block's instances.
func (a Address) Config() Address {
	return Address{module: a.module.Config(), resource: a.resource.Config()}
}

// InstanceOf reports whether a addresses an instance of the static config
// block at config. Equivalent to a.Config() == config.
func (a Address) InstanceOf(config Address) bool {
	return a.Config() == config
}

// LogicalName renders a in the same injective encoding as
// [Path.LogicalName]; ParseAddress is its inverse.
func (a Address) LogicalName() string {
	return a.module.Append(a.resource).LogicalName()
}

// ParseAddress parses the [Address.LogicalName] encoding back into an
// Address: the last segment is the resource step, everything before it the
// module path.
func ParseAddress(s string) (Address, error) {
	p, err := ParseLogicalName(s)
	if err != nil {
		return Address{}, err
	}
	module, resource, _ := p.Parent()
	return Address{module: module, resource: resource}, nil
}

// --- internal encoding ---

// encodeStep writes a single step into b using a length-prefixed layout:
//
//	u16 BE  name length
//	bytes   name
//	u8      kind tag
//	if kind == stepKindIndex:   u64 BE index
//	if kind == stepKindForEach: u16 BE key length, key bytes
func encodeStep(b *strings.Builder, s Step) {
	writeUint16(b, len(s.name))
	b.WriteString(s.name)
	b.WriteByte(byte(s.kind))
	switch s.kind {
	case stepKindIndex:
		var buf [8]byte
		binary.BigEndian.PutUint64(buf[:], s.idx)
		b.Write(buf[:])
	case stepKindForEach:
		writeUint16(b, len(s.key))
		b.WriteString(s.key)
	}
}

// decodeStep returns the cursor position after the step starting at start
// and the decoded Step. It panics if the bytes are malformed; callers must
// only feed it bytes produced by encodeStep.
func decodeStep(bytes []byte, start int) (int, Step) {
	mustHave(bytes, start, 2)
	nameLen := int(binary.BigEndian.Uint16(bytes[start : start+2]))
	start += 2
	mustHave(bytes, start, nameLen)
	name := string(bytes[start : start+nameLen])
	start += nameLen
	mustHave(bytes, start, 1)
	kind := stepKind(bytes[start])
	start++
	switch kind {
	case stepKindBare:
		return start, Step{name: name, kind: kind}
	case stepKindIndex:
		mustHave(bytes, start, 8)
		idx := binary.BigEndian.Uint64(bytes[start : start+8])
		start += 8
		return start, Step{name: name, kind: kind, idx: idx}
	case stepKindForEach:
		mustHave(bytes, start, 2)
		keyLen := int(binary.BigEndian.Uint16(bytes[start : start+2]))
		start += 2
		mustHave(bytes, start, keyLen)
		key := string(bytes[start : start+keyLen])
		start += keyLen
		return start, Step{name: name, kind: kind, key: key}
	default:
		panic(fmt.Sprintf("modulepath: corrupt path repr (unknown kind %d)", kind))
	}
}

func writeUint16(b *strings.Builder, n int) {
	if n < 0 || n > 0xFFFF {
		panic(fmt.Sprintf("modulepath: value %d does not fit in uint16", n))
	}
	var buf [2]byte
	binary.BigEndian.PutUint16(buf[:], uint16(n)) //nolint:gosec // checked above
	b.Write(buf[:])
}

func estimateStepSize(s Step) int {
	size := 2 + len(s.name) + 1
	switch s.kind {
	case stepKindIndex:
		size += 8
	case stepKindForEach:
		size += 2 + len(s.key)
	}
	return size
}

func mustHave(bytes []byte, start, n int) {
	if start+n > len(bytes) {
		panic("modulepath: corrupt path repr (truncated)")
	}
}
