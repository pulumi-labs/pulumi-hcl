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

package bridge

import (
	"github.com/pulumi/pulumi-terraform-bridge/v3/pkg/tfbridge"
	"github.com/pulumi/pulumi-terraform-bridge/v3/pkg/tfbridge/info"
	shim "github.com/pulumi/pulumi-terraform-bridge/v3/pkg/tfshim"
)

// FieldMapping captures one TF attribute's TF→Pulumi naming and block-vs-attr
// shape. A nil *FieldMapping means "field absent from this resource"; a
// non-nil value with TFBlock == false means a plain scalar/list/map attribute.
type FieldMapping struct {
	// TFName is the snake_case name as written in HCL.
	TFName string
	// PulumiName is the camelCase name expected by the Pulumi engine.
	PulumiName string
	// TFBlock is true when this field would be expressed in TF as a block
	// (TypeList or TypeSet with a Resource element).
	TFBlock bool
	// MaxItemsOne is true when a TF block is projected to Pulumi as a single
	// object rather than a list of objects. Only meaningful when TFBlock.
	MaxItemsOne bool
	// Nested is the inner-body mapping for fields with named nested fields:
	// blocks (TFBlock) and object-typed attributes (nested attributes).
	Nested *BodyMapping
}

// BodyMapping describes one TF resource / data source / provider config body:
// which names are valid, whether each is a block, and how to rename to Pulumi.
type BodyMapping struct {
	// Fields is keyed by TF (snake_case) name.
	Fields map[string]*FieldMapping
}

// Lookup returns the mapping for tfName, or nil if no such field exists.
func (m *BodyMapping) Lookup(tfName string) *FieldMapping {
	if m == nil {
		return nil
	}
	return m.Fields[tfName]
}

// TFBlockNames returns the TF names of all block-shaped fields. Order is not
// stable; callers should not rely on it.
func (m *BodyMapping) TFBlockNames() []string {
	if m == nil {
		return nil
	}
	out := make([]string, 0, len(m.Fields))
	for k, f := range m.Fields {
		if f.TFBlock {
			out = append(out, k)
		}
	}
	return out
}

// PulumiNameOf returns the Pulumi attribute name for tfName, or "" if no
// mapping exists.
func (m *BodyMapping) PulumiNameOf(tfName string) string {
	f := m.Lookup(tfName)
	if f == nil {
		return ""
	}
	return f.PulumiName
}

// ResourceBodyMapping builds a BodyMapping for the given TF resource type.
// Returns nil when info has no schema for tfType.
func ResourceBodyMapping(info *tfbridge.ProviderInfo, tfType string) *BodyMapping {
	if info == nil || info.P == nil {
		return nil
	}
	res, ok := info.P.ResourcesMap().GetOk(tfType)
	if !ok || res == nil {
		return nil
	}
	var fieldOverrides map[string]*tfbridge.SchemaInfo
	if r := info.Resources[tfType]; r != nil {
		fieldOverrides = r.Fields
	}
	return bodyMappingFromSchema(res.Schema(), fieldOverrides)
}

// DataSourceBodyMapping builds a BodyMapping for the given TF data source.
func DataSourceBodyMapping(info *tfbridge.ProviderInfo, tfType string) *BodyMapping {
	if info == nil || info.P == nil {
		return nil
	}
	ds, ok := info.P.DataSourcesMap().GetOk(tfType)
	if !ok || ds == nil {
		return nil
	}
	var fieldOverrides map[string]*tfbridge.SchemaInfo
	if d := info.DataSources[tfType]; d != nil {
		fieldOverrides = d.Fields
	}
	return bodyMappingFromSchema(ds.Schema(), fieldOverrides)
}

// ProviderConfigBodyMapping builds a BodyMapping for the provider config block.
func ProviderConfigBodyMapping(info *tfbridge.ProviderInfo) *BodyMapping {
	if info == nil || info.P == nil {
		return nil
	}
	return bodyMappingFromSchema(info.P.Schema(), info.Config)
}

func bodyMappingFromSchema(sm shim.SchemaMap, overrides map[string]*tfbridge.SchemaInfo) *BodyMapping {
	if sm == nil {
		return nil
	}
	m := &BodyMapping{Fields: map[string]*FieldMapping{}}
	sm.Range(func(tfName string, sch shim.Schema) bool {
		ov := overrides[tfName]
		if ov != nil && ov.Omit {
			return true
		}
		fm := &FieldMapping{
			TFName:     tfName,
			PulumiName: tfbridge.TerraformToPulumiNameV2(tfName, sm, overrides),
		}
		if elemRes, isBlock := elemAsResource(sch); isBlock {
			fm.TFBlock = true
			fm.MaxItemsOne = computeMaxItemsOne(sch, ov)
			fm.Nested = bodyMappingFromSchema(elemRes.Schema(), nestedOverrides(ov))
		} else if objRes, isObject := elemAsObject(sch); isObject {
			fm.Nested = bodyMappingFromSchema(objRes.Schema(), nestedOverrides(ov))
		}
		m.Fields[tfName] = fm
		return true
	})
	return m
}

// nestedOverrides returns the SchemaInfo field overrides that apply to a
// field's nested body.
func nestedOverrides(ov *tfbridge.SchemaInfo) map[string]*tfbridge.SchemaInfo {
	if ov == nil {
		return nil
	}
	if ov.Elem != nil && len(ov.Elem.Fields) > 0 {
		return ov.Elem.Fields
	}
	return ov.Fields
}

// elemAsResource returns the nested-block element shim if sch is a TF block
// (TypeList/TypeSet with a Resource Elem).
func elemAsResource(sch shim.Schema) (shim.Resource, bool) {
	switch sch.Type() {
	case shim.TypeList, shim.TypeSet:
		if res, ok := sch.Elem().(shim.Resource); ok {
			return res, true
		}
	}
	return nil, false
}

// elemAsObject returns the pseudo-resource carrying an object-typed
// attribute's named fields. Object types use the tfshim convention
// Schema{Type: Map, Elem: Resource}: collection-nested attributes carry that
// schema as their Elem, and a single nested attribute is that schema directly.
func elemAsObject(sch shim.Schema) (shim.Resource, bool) {
	switch sch.Type() {
	case shim.TypeList, shim.TypeSet:
		if e, ok := sch.Elem().(shim.Schema); ok {
			return objectSchemaResource(e)
		}
	case shim.TypeMap:
		switch e := sch.Elem().(type) {
		case shim.Resource:
			return e, true
		case shim.Schema:
			return objectSchemaResource(e)
		}
	}
	return nil, false
}

// objectSchemaResource unwraps the tfshim object-type convention
// (Schema{Type: Map, Elem: Resource}) to its pseudo-resource.
func objectSchemaResource(sch shim.Schema) (shim.Resource, bool) {
	if sch.Type() == shim.TypeMap {
		if res, ok := sch.Elem().(shim.Resource); ok {
			return res, true
		}
	}
	return nil, false
}

// computeMaxItemsOne mirrors the bridge default: an explicit override beats
// MaxItems(); otherwise List/Set with MaxItems==1 projects to a single object.
func computeMaxItemsOne(sch shim.Schema, ov *info.Schema) bool {
	if ov != nil && ov.MaxItemsOne != nil {
		return *ov.MaxItemsOne
	}
	return sch.MaxItems() == 1
}
