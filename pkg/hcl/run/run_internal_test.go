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
	"testing"

	"github.com/pulumi-labs/pulumi-hcl/pkg/hcl/eval"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/property"
	"github.com/stretchr/testify/assert"
	"github.com/zclconf/go-cty/cty"
)

func TestProviderRefFromCty(t *testing.T) {
	t.Parallel()

	providerOutputs := cty.ObjectVal(map[string]cty.Value{
		"urn": cty.StringVal("urn:pulumi:dev::p::pulumi:providers:aws::aws-west"),
		"id":  cty.StringVal("provider-id-123"),
	})

	ref := property.ResourceReference{
		URN: resource.URN("urn:pulumi:dev::p::pulumi:providers:aws::aws-west"),
		ID:  property.New("provider-id-123"),
	}
	callResult := cty.ObjectVal(map[string]cty.Value{
		"__ref": cty.CapsuleVal(eval.ResourceReferenceCapsuleType, &ref),
	})

	nonProviderResource := cty.ObjectVal(map[string]cty.Value{
		"name": cty.StringVal("my-bucket"),
		"tags": cty.MapValEmpty(cty.String),
	})

	tests := []struct {
		name    string
		val     cty.Value
		want    string
		wantErr string
	}{
		{
			name: "provider resource outputs",
			val:  providerOutputs,
			want: "urn:pulumi:dev::p::pulumi:providers:aws::aws-west::provider-id-123",
		},
		{
			name: "call result with __ref",
			val:  callResult,
			want: "urn:pulumi:dev::p::pulumi:providers:aws::aws-west::provider-id-123",
		},
		{
			name:    "string value",
			val:     cty.StringVal("aws.west"),
			wantErr: "provider value must be an object, got string",
		},
		{
			name:    "non-provider resource (object without urn/id or __ref)",
			val:     nonProviderResource,
			wantErr: "provider value is not a resource reference",
		},
		{
			name:    "null value",
			val:     cty.NullVal(cty.DynamicPseudoType),
			wantErr: "provider value is null",
		},
		{
			name:    "unknown value",
			val:     cty.UnknownVal(cty.DynamicPseudoType),
			wantErr: "provider value is not yet known",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := providerRefFromCty(tt.val)
			if tt.wantErr != "" {
				assert.EqualError(t, err, tt.wantErr)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
