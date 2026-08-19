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

package transform

import (
	"github.com/pulumi/inflector"
	"github.com/pulumi/pulumi-hcl/pkg/hcl/bridge"
	"github.com/pulumi/pulumi/pkg/v3/codegen"
	"github.com/pulumi/pulumi/pkg/v3/codegen/schema"
)

// AlignBodyMapping returns a copy of mapping reconciled with the bound Pulumi
// schema. The raw provider mapping remains authoritative for HCL names and
// block classification, while the bound schema supplies the Pulumi property
// names and wire shapes that the engine accepts.
func AlignBodyMapping(mapping *bridge.BodyMapping, props []*schema.Property) *bridge.BodyMapping {
	if mapping == nil {
		return nil
	}

	aligned := &bridge.BodyMapping{Fields: make(map[string]*bridge.FieldMapping, len(mapping.Fields))}
	for tfName, field := range mapping.Fields {
		if field == nil {
			continue
		}

		fieldCopy := *field
		if prop := propertyForField(tfName, field, props); prop != nil {
			fieldCopy.PulumiName = prop.Name
			if field.TFBlock {
				if repeated, ok := repeatedBlockShape(prop.Type); ok {
					fieldCopy.MaxItemsOne = !repeated
				}
			}
			fieldCopy.Nested = AlignBodyMapping(field.Nested, nestedPropertiesOf(prop.Type))
		} else {
			// Keep unmatched fields so HCL validation can recognize the original
			// provider field and evaluation can report a source-ranged diagnostic.
			fieldCopy.Nested = AlignBodyMapping(field.Nested, nil)
		}
		aligned.Fields[tfName] = &fieldCopy
	}
	return aligned
}

func propertyForField(
	tfName string, field *bridge.FieldMapping, props []*schema.Property,
) *schema.Property {
	for _, prop := range props {
		if prop.Name == field.PulumiName && fieldShapeCompatible(field, prop) {
			return prop
		}
	}

	if _, prop := camelCaseFromSnakeCase(tfName, props); prop != nil && fieldShapeCompatible(field, prop) {
		return prop
	}

	// Some dynamically-bound schemas retain a collection where the raw
	// mapping projects MaxItemsOne to a scalar (or vice versa). In that case
	// the generated Pulumi name differs only by singular/plural inflection.
	variants := map[string]struct{}{
		inflector.Pluralize(field.PulumiName):   {},
		inflector.Singularize(field.PulumiName): {},
	}
	var match *schema.Property
	for _, prop := range props {
		if _, ok := variants[prop.Name]; !ok {
			continue
		}
		if !fieldShapeCompatible(field, prop) {
			continue
		}
		if match != nil {
			return nil
		}
		match = prop
	}
	return match
}

func fieldShapeCompatible(field *bridge.FieldMapping, prop *schema.Property) bool {
	if !field.TFBlock {
		return true
	}
	_, ok := repeatedBlockShape(prop.Type)
	return ok
}

func repeatedBlockShape(typ schema.Type) (repeated bool, ok bool) {
	if _, ok := AsHCLBlockType(typ); ok {
		return true, true
	}
	_, ok = codegen.UnwrapType(typ).(*schema.ObjectType)
	return false, ok
}

func nestedPropertiesOf(typ schema.Type) []*schema.Property {
	switch typ := codegen.UnwrapType(typ).(type) {
	case *schema.ObjectType:
		return typ.Properties
	case *schema.ArrayType:
		return nestedPropertiesOf(typ.ElementType)
	case *schema.MapType:
		return nestedPropertiesOf(typ.ElementType)
	default:
		return nil
	}
}
