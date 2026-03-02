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

package graph

import (
	"fmt"

	"github.com/zclconf/go-cty/cty"
)

// ExpandedResource represents a single instance of a resource after count/for_each expansion.
type ExpandedResource struct {
	// Key is the unique identifier for this instance (e.g., "aws_instance.web[0]" or "aws_instance.web[\"a\"]")
	Key string

	// OriginalKey is the key of the original resource before expansion
	OriginalKey string

	// Index is the numeric index for count-based expansion (nil for for_each)
	Index *int

	// EachKey is the key for for_each expansion (nil for count)
	EachKey *cty.Value

	// EachValue is the value for for_each expansion
	EachValue *cty.Value

	// Node is the original graph node
	Node *Node
}

// ExpandResult contains the results of expanding a resource.
type ExpandResult struct {
	// Instances are the expanded resource instances
	Instances []*ExpandedResource

	// IsSingle is true if this is a single instance (no count or for_each)
	IsSingle bool
}

// ResourceExpander handles count and for_each expansion.
type ResourceExpander struct {
	// countValues maps resource keys to their evaluated count values
	countValues map[string]int

	// boolCountKeys tracks resources whose count is a bool (0 or 1, single-instance semantics)
	boolCountKeys map[string]bool

	// forEachValues maps resource keys to their evaluated for_each values
	forEachValues map[string]map[string]cty.Value
}

// NewResourceExpander creates a new resource expander.
func NewResourceExpander() *ResourceExpander {
	return &ResourceExpander{
		countValues:   make(map[string]int),
		boolCountKeys: make(map[string]bool),
		forEachValues: make(map[string]map[string]cty.Value),
	}
}

// SetCount sets the evaluated count value for a resource.
func (e *ResourceExpander) SetCount(key string, count int) {
	e.countValues[key] = count
}

// SetBoolCount sets a bool-derived count for a resource (0 or 1).
// When count > 0, produces a single instance with no numeric index suffix.
func (e *ResourceExpander) SetBoolCount(key string, count int) {
	e.countValues[key] = count
	e.boolCountKeys[key] = true
}

// SetForEach sets the evaluated for_each value for a resource.
func (e *ResourceExpander) SetForEach(key string, values map[string]cty.Value) {
	e.forEachValues[key] = values
}

// Expand expands a resource node into its instances.
func (e *ResourceExpander) Expand(node *Node) *ExpandResult {
	// Check for count
	if count, ok := e.countValues[node.Key]; ok {
		if count == 0 {
			return &ExpandResult{
				Instances: nil,
				IsSingle:  false,
			}
		}

		// Bool-derived counts produce a single instance without an index suffix.
		if e.boolCountKeys[node.Key] {
			return &ExpandResult{
				Instances: []*ExpandedResource{{
					Key:         node.Key,
					OriginalKey: node.Key,
					Node:        node,
				}},
				IsSingle: true,
			}
		}

		instances := make([]*ExpandedResource, count)
		for i := 0; i < count; i++ {
			idx := i
			instances[i] = &ExpandedResource{
				Key:         fmt.Sprintf("%s[%d]", node.Key, i),
				OriginalKey: node.Key,
				Index:       &idx,
				Node:        node,
			}
		}
		return &ExpandResult{
			Instances: instances,
			IsSingle:  false,
		}
	}

	// Check for for_each
	if forEachVals, ok := e.forEachValues[node.Key]; ok {
		if len(forEachVals) == 0 {
			return &ExpandResult{
				Instances: nil,
				IsSingle:  false,
			}
		}

		instances := make([]*ExpandedResource, 0, len(forEachVals))
		for k, v := range forEachVals {
			key := cty.StringVal(k)
			val := v
			instances = append(instances, &ExpandedResource{
				Key:         fmt.Sprintf("%s[%q]", node.Key, k),
				OriginalKey: node.Key,
				EachKey:     &key,
				EachValue:   &val,
				Node:        node,
			})
		}
		return &ExpandResult{
			Instances: instances,
			IsSingle:  false,
		}
	}

	// Single instance
	return &ExpandResult{
		Instances: []*ExpandedResource{
			{
				Key:         node.Key,
				OriginalKey: node.Key,
				Node:        node,
			},
		},
		IsSingle: true,
	}
}

// InstanceKey generates an instance key for a resource with count or for_each.
func InstanceKey(resourceKey string, index *int, eachKey *cty.Value) string {
	if index != nil {
		return fmt.Sprintf("%s[%d]", resourceKey, *index)
	}
	if eachKey != nil && eachKey.Type() == cty.String {
		return fmt.Sprintf("%s[%q]", resourceKey, eachKey.AsString())
	}
	return resourceKey
}

// ParseInstanceKey parses an instance key back into its components.
// Returns the base key, optional index, and optional each key.
func ParseInstanceKey(instanceKey string) (baseKey string, index *int, eachKey *string) {
	// Look for [...]
	bracketStart := -1
	for i := len(instanceKey) - 1; i >= 0; i-- {
		if instanceKey[i] == '[' {
			bracketStart = i
			break
		}
	}

	if bracketStart == -1 {
		return instanceKey, nil, nil
	}

	baseKey = instanceKey[:bracketStart]
	indexPart := instanceKey[bracketStart+1 : len(instanceKey)-1]

	// Check if it's a number
	var idx int
	if _, err := fmt.Sscanf(indexPart, "%d", &idx); err == nil {
		return baseKey, &idx, nil
	}

	// Must be a string key (with quotes)
	if len(indexPart) >= 2 && indexPart[0] == '"' && indexPart[len(indexPart)-1] == '"' {
		key := indexPart[1 : len(indexPart)-1]
		return baseKey, nil, &key
	}

	return instanceKey, nil, nil
}
