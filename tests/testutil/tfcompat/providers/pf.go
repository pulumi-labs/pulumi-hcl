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

package providers

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	dschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	pschema "github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	rschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// PFXProvider is a minimal terraform-plugin-framework provider: two resources
// holding a single string, and one data source returning a constant. It
// exists so tfcompat covers providers built on the plugin framework rather
// than terraform-plugin-sdk/v2. pfx_widget accepts a `moved` from pfx_thing,
// which no terraform-plugin-sdk/v2 provider can (its MoveResourceState
// unconditionally errors).
func PFXProvider() provider.Provider { return &pfxProvider{} }

type pfxProvider struct{}

var _ provider.Provider = (*pfxProvider)(nil)

func (p *pfxProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "pfx"
}

func (p *pfxProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = pschema.Schema{}
}

func (p *pfxProvider) Configure(context.Context, provider.ConfigureRequest, *provider.ConfigureResponse) {
}

func (p *pfxProvider) Resources(context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		func() resource.Resource { return &pfxThing{} },
		func() resource.Resource { return &pfxWidget{} },
	}
}

func (p *pfxProvider) DataSources(context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		func() datasource.DataSource { return &pfxLookup{} },
	}
}

type pfxThing struct{}

var _ resource.Resource = (*pfxThing)(nil)

type pfxThingModel struct {
	ID   types.String `tfsdk:"id"`
	Name types.String `tfsdk:"name"`
}

func (r *pfxThing) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_thing"
}

// pfxThingSchema is shared by pfx_thing and pfx_widget: the cross-type `moved`
// test moves state between two resources of identical shape.
func pfxThingSchema() rschema.Schema {
	return rschema.Schema{
		Attributes: map[string]rschema.Attribute{
			"id": rschema.StringAttribute{
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": rschema.StringAttribute{Required: true},
		},
	}
}

func (r *pfxThing) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = pfxThingSchema()
}

func (r *pfxThing) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan pfxThingModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	plan.ID = types.StringValue("pfx-id")
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *pfxThing) Read(_ context.Context, _ resource.ReadRequest, _ *resource.ReadResponse) {}

func (r *pfxThing) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan pfxThingModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *pfxThing) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {}

// pfxWidget is pfx_thing under another type name. It implements MoveState so a
// `moved { from = pfx_thing.x, to = pfx_widget.x }` is a state-only move.
type pfxWidget struct{}

var (
	_ resource.Resource              = (*pfxWidget)(nil)
	_ resource.ResourceWithMoveState = (*pfxWidget)(nil)
)

func (r *pfxWidget) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_widget"
}

func (r *pfxWidget) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = pfxThingSchema()
}

func (r *pfxWidget) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan pfxThingModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	plan.ID = types.StringValue("pfx-widget-id")
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *pfxWidget) Read(_ context.Context, _ resource.ReadRequest, _ *resource.ReadResponse) {}

func (r *pfxWidget) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan pfxThingModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *pfxWidget) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
}

func (r *pfxWidget) MoveState(context.Context) []resource.StateMover {
	sourceSchema := pfxThingSchema()
	return []resource.StateMover{{
		SourceSchema: &sourceSchema,
		StateMover: func(ctx context.Context, req resource.MoveStateRequest, resp *resource.MoveStateResponse) {
			if req.SourceTypeName != "pfx_thing" || req.SourceState == nil {
				return
			}
			var src pfxThingModel
			resp.Diagnostics.Append(req.SourceState.Get(ctx, &src)...)
			if resp.Diagnostics.HasError() {
				return
			}
			resp.Diagnostics.Append(resp.TargetState.Set(ctx, &src)...)
		},
	}}
}

type pfxLookup struct{}

var _ datasource.DataSource = (*pfxLookup)(nil)

type pfxLookupModel struct {
	Value types.String `tfsdk:"value"`
}

func (d *pfxLookup) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_lookup"
}

func (d *pfxLookup) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = dschema.Schema{
		Attributes: map[string]dschema.Attribute{
			"value": dschema.StringAttribute{Computed: true},
		},
	}
}

func (d *pfxLookup) Read(ctx context.Context, _ datasource.ReadRequest, resp *datasource.ReadResponse) {
	state := pfxLookupModel{Value: types.StringValue("pfx-value")}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
