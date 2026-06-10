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

// PFXProvider is the plugin-framework counterpart of SimpleProvider: one
// resource and one data source, plus a `prefix` provider-config attribute
// both concatenate into `prefix_result` so tests can observe provider config
// flowing end-to-end.
func PFXProvider() provider.Provider { return &pfxProvider{} }

type pfxProvider struct{}

var _ provider.Provider = (*pfxProvider)(nil)

func (p *pfxProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "pfx"
}

func (p *pfxProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = pschema.Schema{
		Attributes: map[string]pschema.Attribute{
			"prefix": pschema.StringAttribute{Optional: true},
		},
	}
}

func (p *pfxProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var cfg struct {
		Prefix types.String `tfsdk:"prefix"`
	}
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.ResourceData = cfg.Prefix.ValueString()
	resp.DataSourceData = cfg.Prefix.ValueString()
}

func (p *pfxProvider) Resources(context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		func() resource.Resource { return &pfxThing{} },
	}
}

func (p *pfxProvider) DataSources(context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		func() datasource.DataSource { return &pfxLookup{} },
	}
}

type pfxThing struct {
	prefix string
}

var _ resource.ResourceWithConfigure = (*pfxThing)(nil)

type pfxThingModel struct {
	ID           types.String `tfsdk:"id"`
	Name         types.String `tfsdk:"name"`
	Note         types.String `tfsdk:"note"`
	Echo         types.String `tfsdk:"echo"`
	PrefixResult types.String `tfsdk:"prefix_result"`
}

func (r *pfxThing) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_thing"
}

func (r *pfxThing) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = rschema.Schema{
		Attributes: map[string]rschema.Attribute{
			"id": rschema.StringAttribute{
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name":          rschema.StringAttribute{Required: true},
			"note":          rschema.StringAttribute{Optional: true},
			"echo":          rschema.StringAttribute{Computed: true},
			"prefix_result": rschema.StringAttribute{Computed: true},
		},
	}
}

func (r *pfxThing) Configure(_ context.Context, req resource.ConfigureRequest, _ *resource.ConfigureResponse) {
	if prefix, ok := req.ProviderData.(string); ok {
		r.prefix = prefix
	}
}

// fill computes the resource's computed attributes from its arguments.
func (r *pfxThing) fill(m *pfxThingModel) {
	echo := m.Name.ValueString()
	if !m.Note.IsNull() {
		echo += ":" + m.Note.ValueString()
	}
	m.Echo = types.StringValue(echo)
	m.PrefixResult = types.StringValue(r.prefix + "-" + m.Name.ValueString())
}

func (r *pfxThing) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan pfxThingModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	plan.ID = types.StringValue("pfx-id")
	r.fill(&plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *pfxThing) Read(_ context.Context, _ resource.ReadRequest, _ *resource.ReadResponse) {}

func (r *pfxThing) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan pfxThingModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.fill(&plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *pfxThing) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {}

type pfxLookup struct {
	prefix string
}

var _ datasource.DataSourceWithConfigure = (*pfxLookup)(nil)

type pfxLookupModel struct {
	Query        types.String `tfsdk:"query"`
	PrefixResult types.String `tfsdk:"prefix_result"`
}

func (d *pfxLookup) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_lookup"
}

func (d *pfxLookup) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = dschema.Schema{
		Attributes: map[string]dschema.Attribute{
			"query":         dschema.StringAttribute{Required: true},
			"prefix_result": dschema.StringAttribute{Computed: true},
		},
	}
}

func (d *pfxLookup) Configure(_ context.Context, req datasource.ConfigureRequest, _ *datasource.ConfigureResponse) {
	if prefix, ok := req.ProviderData.(string); ok {
		d.prefix = prefix
	}
}

func (d *pfxLookup) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg pfxLookupModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	cfg.PrefixResult = types.StringValue(d.prefix + "-" + cfg.Query.ValueString())
	resp.Diagnostics.Append(resp.State.Set(ctx, &cfg)...)
}
