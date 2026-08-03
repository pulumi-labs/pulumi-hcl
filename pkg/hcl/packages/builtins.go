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

package packages

import (
	"strings"

	"github.com/pulumi/pulumi/pkg/v3/codegen/schema"
	"github.com/pulumi/pulumi/sdk/v3/go/common/util/contract"
	"github.com/pulumi/pulumi/sdk/v3/go/property"
	"github.com/zclconf/go-cty/cty"
	ctyjson "github.com/zclconf/go-cty/cty/json"
)

const (
	// TerraformProviderSource is the source address of the builtin `terraform`
	// provider. Declaring it in required_providers is what brings its
	// provider-defined functions (provider::<local>::encode_expr, ...) into
	// scope; the provider itself ships no installable plugin.
	TerraformProviderSource = "terraform.io/builtin/terraform"

	// TerraformDataType is Terraform's builtin `terraform_data` managed
	// resource. Its provider (`terraform`) ships no installable plugin;
	// Terraform/OpenTofu implement it internally.
	TerraformDataType = "terraform_data"
	// StashToken is the Pulumi engine's builtin resource terraform_data lowers
	// onto.
	StashToken = "pulumi:index:Stash"

	// RemoteStateType is Terraform's builtin `terraform_remote_state` data
	// source.
	RemoteStateType = "terraform_remote_state"
	// LocalReferenceToken / RemoteReferenceToken are the pulumi-terraform invokes
	// terraform_remote_state lowers onto, by backend.
	LocalReferenceToken  = "terraform:state:getLocalReference"
	RemoteReferenceToken = "terraform:state:getRemoteReference"
)

// IsTerraformProviderSource reports whether a required_providers source
// address names the builtin terraform provider.
func IsTerraformProviderSource(source string) bool {
	return strings.EqualFold(source, TerraformProviderSource)
}

// TerraformDataSchema is the synthetic schema terraform_data resolves to, so it
// flows through the generic registration path like any other resource. It is
// backed by the engine's builtin Stash resource (hence the Stash token) but
// described in terraform_data's own terms; the lowerTerraformData* hooks bridge
// the two property surfaces around the Stash registration:
//
//   - `input` is the value Stash stores.
//   - `output` mirrors `input`. It is read back from Stash's `input` output,
//     which tracks the current input across updates (Stash's own `output`
//     property is frozen at create time and is not used).
//   - `triggers_replace` is declared so it is evaluated from config, but it is
//     not sent to Stash; lowerTerraformDataInputs moves it to the engine's
//     replacement trigger so a change to it forces replacement (and a new id).
//
// `id` is the Stash resource id and is attached by the registration machinery,
// so it is not declared here.
func TerraformDataSchema() *schema.Resource {
	// All of terraform_data's attributes are optional/nullable, so the property
	// types are wrapped in OptionalType (an un-wrapped type reads as required).
	anyProp := func(name string) *schema.Property {
		return &schema.Property{Name: name, Type: &schema.OptionalType{ElementType: schema.AnyType}}
	}
	return &schema.Resource{
		Token:           StashToken,
		InputProperties: []*schema.Property{anyProp("input"), anyProp("triggers_replace")},
		Properties: []*schema.Property{
			anyProp("input"), anyProp("output"), anyProp("triggers_replace"),
		},
	}
}

// terraform_data's attributes are dynamically typed, and the Pulumi property
// encoding cannot represent every cty type: a set flattens to an ordered array
// that would re-expand as a tuple. Each value is therefore boxed as a
// {type, value} wrapper object — persisting the cty type through the engine
// and its state alongside the value, the way OpenTofu stores dynamic
// attributes.
const (
	TerraformDataTypeKey  = "type"
	TerraformDataValueKey = "value"
)

// WrapTerraformDataValue boxes a terraform_data value of the given cty type as
// a {type, value} wrapper.
func WrapTerraformDataValue(t cty.Type, v property.Value) property.Value {
	typeJSON, err := ctyjson.MarshalType(t)
	contract.AssertNoErrorf(err, "marshaling cty type for the terraform_data wrapper")
	return property.New(map[string]property.Value{
		TerraformDataTypeKey:  property.New(string(typeJSON)),
		TerraformDataValueKey: v,
	})
}

// TerraformRemoteStateSchema is the synthetic schema terraform_remote_state
// resolves to: the TF data source surface as inputs. lowerRemoteStateInvoke
// selects the concrete pulumi-terraform invoke (local or remote) by backend and
// translates the inputs to its arguments, and remoteStateResult assembles the
// data source's object from it. The placeholder Token is always overridden there.
func TerraformRemoteStateSchema() *schema.Function {
	opt := func(t schema.Type) schema.Type { return &schema.OptionalType{ElementType: t} }
	return &schema.Function{
		Token: LocalReferenceToken,
		Inputs: &schema.ObjectType{
			Properties: []*schema.Property{
				{Name: "backend", Type: opt(schema.StringType)},
				{Name: "config", Type: opt(schema.AnyType)},
				{Name: "workspace", Type: opt(schema.StringType)},
				{Name: "defaults", Type: opt(schema.AnyType)},
			},
		},
		// The data source's own arguments are readable attributes of it, so they
		// are part of the result too; remoteStateResult stores them back.
		ReturnType: &schema.ObjectType{
			Properties: []*schema.Property{
				{Name: "outputs", Type: schema.AnyType},
				{Name: "backend", Type: opt(schema.StringType)},
				{Name: "config", Type: opt(schema.AnyType)},
				{Name: "workspace", Type: opt(schema.StringType)},
				{Name: "defaults", Type: opt(schema.AnyType)},
			},
		},
	}
}
