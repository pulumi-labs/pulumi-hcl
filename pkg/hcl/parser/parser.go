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

package parser

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/blang/semver"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/ext/typeexpr"
	"github.com/pulumi-labs/pulumi-hcl/pkg/hcl/ast"
	"github.com/pulumi/pulumi/sdk/v3/go/common/tokens"
	"github.com/zclconf/go-cty/cty"
)

// Parser parses HCL files into an AST configuration.
type Parser struct {
	loader *Loader
}

// NewParser creates a new HCL parser.
func NewParser() *Parser {
	return &Parser{
		loader: NewLoader(),
	}
}

// ParseDirectory parses all HCL files in a directory into a configuration.
func (p *Parser) ParseDirectory(dir string) (*ast.Config, hcl.Diagnostics) {
	files, diags := p.loader.LoadDirectory(dir)
	if diags.HasErrors() {
		return nil, diags
	}

	return p.parseFiles(files)
}

// ParseFile parses a single HCL file into a configuration.
func (p *Parser) ParseFile(path string) (*ast.Config, hcl.Diagnostics) {
	file, diags := p.loader.LoadFile(path)
	if diags.HasErrors() {
		return nil, diags
	}

	return p.parseFiles(map[string]*hcl.File{path: file})
}

// ParseSource parses HCL source code into a configuration.
func (p *Parser) ParseSource(filename string, src []byte) (*ast.Config, hcl.Diagnostics) {
	file, diags := p.loader.ParseFile(filename, src)
	if diags.HasErrors() {
		return nil, diags
	}

	return p.parseFiles(map[string]*hcl.File{filename: file})
}

// parseFiles processes all loaded HCL files into a configuration.
func (p *Parser) parseFiles(files map[string]*hcl.File) (*ast.Config, hcl.Diagnostics) {
	config := ast.NewConfig()
	config.Files = files
	var diags hcl.Diagnostics

	calls := map[string]struct{}{}
	for _, file := range files {
		fileCalls, scanned := ast.ProviderFunctionCallsInBody(file.Body)
		if !scanned {
			config.ProviderFunctionCallsIncomplete = true
		}
		for _, name := range fileCalls {
			calls[name] = struct{}{}
		}

		content, contentDiags := file.Body.Content(rootSchema)
		diags = append(diags, contentDiags...)
		if contentDiags.HasErrors() {
			continue
		}

		for _, block := range content.Blocks {
			blockDiags := p.parseBlock(config, block)
			diags = append(diags, blockDiags...)
		}
	}
	config.ProviderFunctionCalls = slices.Sorted(maps.Keys(calls))

	config.Diagnostics = diags
	return config, diags
}

// parseBlock parses a single top-level block.
func (p *Parser) parseBlock(config *ast.Config, block *hcl.Block) hcl.Diagnostics {
	switch block.Type {
	case "terraform":
		return p.parseTerraformBlock(config, block)
	case "provider":
		return p.parseProviderBlock(config, block)
	case "variable":
		return p.parseVariableBlock(config, block)
	case "locals":
		return p.parseLocalsBlock(config, block)
	case "resource":
		return p.parseResourceBlock(config, block, false)
	case "data":
		return p.parseResourceBlock(config, block, true)
	case "output":
		return p.parseOutputBlock(config, block)
	case "module":
		return p.parseModuleBlock(config, block)
	case "moved":
		return p.parseMovedBlock(config, block)
	case "import":
		return p.parseImportBlock(config, block)
	case "check":
		return p.parseCheckBlock(config, block)
	case "call":
		return p.parseCallBlock(config, block)
	default:
		return hcl.Diagnostics{{
			Severity: hcl.DiagError,
			Summary:  "Unknown block type",
			Detail:   fmt.Sprintf("Block type %q is not supported.", block.Type),
			Subject:  &block.DefRange,
		}}
	}
}

// parseTerraformBlock parses a terraform block. Multiple terraform blocks
// merge into a single configuration, matching Terraform's behavior.
func (p *Parser) parseTerraformBlock(config *ast.Config, block *hcl.Block) hcl.Diagnostics {
	var diags hcl.Diagnostics

	content, contentDiags := block.Body.Content(terraformSchema)
	diags = append(diags, contentDiags...)

	if len(block.Labels) != 0 {
		diags = append(diags, &hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Invalid terraform block",
			Detail:   "The terraform block does not accept any labels.",
			Subject:  &block.DefRange,
		})
		return diags
	}

	tf := config.Terraform
	if tf == nil {
		tf = &ast.Terraform{
			RequiredProviders: make(map[string]*ast.RequiredProvider),
			DeclRange:         block.DefRange,
		}
	}

	if attr, ok := content.Attributes["required_version_range"]; ok {
		if tf.RequiredVersionRange != nil {
			diags = append(diags, &hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Duplicate required_version_range attribute",
				Detail:   "required_version_range is already set by a previous terraform block.",
				Subject:  attr.Range.Ptr(),
			})
		} else {
			tf.RequiredVersionRange = attr.Expr
		}
	}

	if attr, ok := content.Attributes["required_version"]; ok {
		diags = append(diags, &hcl.Diagnostic{
			Severity: hcl.DiagWarning,
			Summary:  "Unsupported attribute: required_version",
			Detail: "Pulumi HCL does not enforce required_version. " +
				"Use required_version_range to declare a supported Pulumi version range.",
			Subject: attr.Range.Ptr(),
		})
	}

	for _, subBlock := range content.Blocks {
		switch subBlock.Type {
		case "required_providers":
			providerDiags := p.parseRequiredProviders(tf, subBlock)
			diags = append(diags, providerDiags...)
		case "component":
			if tf.Component != nil {
				diags = append(diags, &hcl.Diagnostic{
					Severity: hcl.DiagError,
					Summary:  "Duplicate component block",
					Detail:   "Only one component block is allowed per terraform block.",
					Subject:  &subBlock.DefRange,
				})
				continue
			}
			comp, compDiags := p.parseTerraformComponentBlock(subBlock)
			diags = append(diags, compDiags...)
			tf.Component = comp
		case "package":
			if tf.Package != nil {
				diags = append(diags, &hcl.Diagnostic{
					Severity: hcl.DiagError,
					Summary:  "Duplicate package block",
					Detail:   "Only one package block is allowed per terraform block.",
					Subject:  &subBlock.DefRange,
				})
				continue
			}
			pkg, pkgDiags := p.parseTerraformPackageBlock(subBlock)
			diags = append(diags, pkgDiags...)
			tf.Package = pkg
		case "backend":
			diags = append(diags, &hcl.Diagnostic{
				Severity: hcl.DiagWarning,
				Summary:  "Ignoring terraform backend block",
				Detail: "Pulumi manages state through its own backend; the terraform `backend` " +
					"block is ignored. Configure the backend via `pulumi login`.",
				Subject: &subBlock.DefRange,
			})
		case "provider_meta":
			diags = append(diags, &hcl.Diagnostic{
				Severity: hcl.DiagWarning,
				Summary:  "Ignoring terraform provider_meta block",
				Detail: "Pulumi providers do not consume Terraform's `provider_meta` " +
					"side channel; the block is ignored.",
				Subject: &subBlock.DefRange,
			})
		}
	}

	config.Terraform = tf
	return diags
}

// parseTerraformComponentBlock parses a component sub-block within terraform.
func (p *Parser) parseTerraformComponentBlock(block *hcl.Block) (*ast.ComponentBlock, hcl.Diagnostics) {
	content, diags := block.Body.Content(terraformComponentSchema)

	comp := &ast.ComponentBlock{
		Module:    "index",
		DeclRange: block.DefRange,
	}

	if attr, ok := content.Attributes["name"]; ok {
		val, valDiags := attr.Expr.Value(nil)
		diags = append(diags, valDiags...)
		if val.Type() == cty.String {
			comp.Name = val.AsString()
			if !tokens.IsName(comp.Name) {
				diags = append(diags, &hcl.Diagnostic{
					Severity: hcl.DiagError,
					Summary:  "Invalid component name",
					Detail:   fmt.Sprintf("%q is not a valid Pulumi name.", comp.Name),
					Subject:  attr.Expr.Range().Ptr(),
				})
			}
		}
	}

	if attr, ok := content.Attributes["module"]; ok {
		val, valDiags := attr.Expr.Value(nil)
		diags = append(diags, valDiags...)
		if val.Type() == cty.String {
			comp.Module = val.AsString()
			if !tokens.IsName(comp.Module) {
				diags = append(diags, &hcl.Diagnostic{
					Severity: hcl.DiagError,
					Summary:  "Invalid component module",
					Detail:   fmt.Sprintf("%q is not a valid Pulumi name.", comp.Module),
					Subject:  attr.Expr.Range().Ptr(),
				})
			}
		}
	}

	return comp, diags
}

// parseTerraformPackageBlock parses a package sub-block within terraform.
func (p *Parser) parseTerraformPackageBlock(block *hcl.Block) (*ast.PackageBlock, hcl.Diagnostics) {
	content, diags := block.Body.Content(terraformPackageSchema)

	pkg := &ast.PackageBlock{
		Version:   "0.0.0-dev",
		DeclRange: block.DefRange,
	}

	if attr, ok := content.Attributes["name"]; ok {
		val, valDiags := attr.Expr.Value(nil)
		diags = append(diags, valDiags...)
		if val.Type() == cty.String {
			pkg.Name = val.AsString()
			if !tokens.IsName(pkg.Name) {
				diags = append(diags, &hcl.Diagnostic{
					Severity: hcl.DiagError,
					Summary:  "Invalid package name",
					Detail:   fmt.Sprintf("%q is not a valid Pulumi name.", pkg.Name),
					Subject:  attr.Expr.Range().Ptr(),
				})
			}
		}
	}

	if attr, ok := content.Attributes["version"]; ok {
		val, valDiags := attr.Expr.Value(nil)
		diags = append(diags, valDiags...)
		if val.Type() == cty.String {
			pkg.Version = val.AsString()
			if _, err := semver.Parse(pkg.Version); err != nil {
				diags = append(diags, &hcl.Diagnostic{
					Severity: hcl.DiagError,
					Summary:  "Invalid package version",
					Detail:   fmt.Sprintf("%q is not a valid semver version: %s", pkg.Version, err),
					Subject:  attr.Expr.Range().Ptr(),
				})
			}
		}
	}

	return pkg, diags
}

// parseRequiredProviders parses the required_providers block.
func (p *Parser) parseRequiredProviders(tf *ast.Terraform, block *hcl.Block) hcl.Diagnostics {
	var diags hcl.Diagnostics

	attrs, attrDiags := block.Body.JustAttributes()
	diags = append(diags, attrDiags...)

	for name, attr := range attrs {
		if _, exists := tf.RequiredProviders[name]; exists {
			diags = append(diags, &hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Duplicate required_providers entry",
				Detail: fmt.Sprintf(
					"required provider %q is already declared by a previous required_providers block.",
					name),
				Subject: attr.Range.Ptr(),
			})
			continue
		}

		provider := &ast.RequiredProvider{
			Name:      name,
			DeclRange: attr.Range,
		}

		// The value can be a string (version only) or an object with
		// `source`, `version`, and/or `configuration_aliases`. We walk
		// the object item-by-item rather than evaluating it whole because
		// `configuration_aliases` holds traversals (e.g. `[simple.foo]`)
		// that can't resolve in the empty parse-time context. pulumi-hcl
		// doesn't act on `configuration_aliases` — the matching provider
		// arrives via the caller's `providers = {...}` block at runtime
		// — so we accept and discard it.
		if pairs, mapDiags := hcl.ExprMap(attr.Expr); !mapDiags.HasErrors() {
			for _, pair := range pairs {
				kw := hcl.ExprAsKeyword(pair.Key)
				switch kw {
				case "source":
					v, vd := pair.Value.Value(nil)
					diags = append(diags, vd...)
					if v.Type() == cty.String && !v.IsNull() {
						provider.Source = v.AsString()
					}
				case "version":
					v, vd := pair.Value.Value(nil)
					diags = append(diags, vd...)
					if v.Type() == cty.String && !v.IsNull() {
						provider.Version = v.AsString()
					}
				case "configuration_aliases":
					// Accept and ignore — see comment above.
				}
			}
		} else {
			val, valDiags := attr.Expr.Value(nil)
			diags = append(diags, valDiags...)
			if val.Type() == cty.String {
				provider.Version = val.AsString()
			}
		}

		// Pulumi providers don't go through TF-style constraint resolution, so
		// any version given must be a concrete semver.
		if provider.Version != "" && provider.IsPulumi() {
			if _, err := semver.ParseTolerant(provider.Version); err != nil {
				diags = append(diags, &hcl.Diagnostic{
					Severity: hcl.DiagError,
					Summary:  "Invalid provider version",
					Detail: fmt.Sprintf(
						"%q is not a valid semver version for Pulumi provider %q: %s",
						provider.Version, name, err),
					Subject: attr.Expr.Range().Ptr(),
				})
			}
		}

		tf.RequiredProviders[name] = provider
	}

	return diags
}

// parseProviderBlock parses a provider block.
func (p *Parser) parseProviderBlock(config *ast.Config, block *hcl.Block) hcl.Diagnostics {
	var diags hcl.Diagnostics

	// `call` is reserved for the method-call namespace populated by
	// `call "<res>" "<method>" { ... }` blocks.
	if block.Labels[0] == "call" {
		return hcl.Diagnostics{{
			Severity: hcl.DiagError,
			Summary:  "Reserved provider name",
			Detail: `"call" is reserved as the namespace for method calls on resources ` +
				`(declared via call blocks) and cannot be used as a provider package name.`,
			Subject: block.LabelRanges[0].Ptr(),
		}}
	}

	content, remain, contentDiags := block.Body.PartialContent(providerSchema)
	diags = append(diags, contentDiags...)

	provider := &ast.Provider{
		Name:      block.Labels[0],
		Config:    remain,
		DeclRange: block.DefRange,
	}

	if attr, ok := content.Attributes["alias"]; ok {
		val, valDiags := attr.Expr.Value(nil)
		diags = append(diags, valDiags...)
		if val.Type() == cty.String {
			provider.Alias = val.AsString()
		}
	}
	if attr, ok := content.Attributes["for_each"]; ok {
		provider.ForEach = attr.Expr
	}
	if provider.ForEach != nil && provider.Alias == "" {
		diags = append(diags, &hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  `Alias required when using "for_each"`,
			Detail:   "The for_each argument is allowed only for provider configurations with an alias.",
			Subject:  provider.ForEach.Range().Ptr(),
		})
	}
	for _, subBlock := range content.Blocks {
		if subBlock.Type == "pulumi" {
			diags = append(diags, p.parsePulumiProviderOptions(subBlock, provider)...)
		}
	}

	key := provider.Key()
	if _, exists := config.Providers[key]; exists {
		diags = append(diags, &hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Duplicate provider configuration",
			Detail:   fmt.Sprintf("A provider configuration for %q already exists.", key),
			Subject:  &block.DefRange,
		})
		return diags
	}

	config.Providers[key] = provider
	return diags
}

// parsePulumiProviderOptions parses a provider block's nested `pulumi { }`
// block, extracting Pulumi-specific options onto provider. These options live
// in their own block so they cannot collide with the provider's own
// configuration attributes.
func (p *Parser) parsePulumiProviderOptions(block *hcl.Block, provider *ast.Provider) hcl.Diagnostics {
	content, diags := block.Body.Content(pulumiProviderOptionsSchema)

	if attr, ok := content.Attributes["env_var_mappings"]; ok {
		provider.EnvVarMappings = attr.Expr
	}
	if attr, ok := content.Attributes["plugin_download_url"]; ok {
		provider.PluginDownloadURL = attr.Expr
	}
	if attr, ok := content.Attributes["additional_secret_outputs"]; ok {
		provider.AdditionalSecretOutputs = attr.Expr
	}
	if attr, ok := content.Attributes["version"]; ok {
		provider.Version = attr.Expr
	}

	return diags
}

// parseVariableBlock parses a variable block.
func (p *Parser) parseVariableBlock(config *ast.Config, block *hcl.Block) hcl.Diagnostics {
	var diags hcl.Diagnostics

	name := block.Labels[0]
	if _, exists := config.Variables[name]; exists {
		diags = append(diags, &hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Duplicate variable",
			Detail:   fmt.Sprintf("A variable named %q was already declared.", name),
			Subject:  &block.DefRange,
		})
		return diags
	}

	content, contentDiags := block.Body.Content(variableSchema)
	diags = append(diags, contentDiags...)

	variable := &ast.Variable{
		Name:      name,
		Nullable:  true, // Default
		DeclRange: block.DefRange,
	}

	if attr, ok := content.Attributes["type"]; ok {
		variable.Type = attr.Expr
		ty, defs, typeDiags := typeexpr.TypeConstraintWithDefaults(attr.Expr)
		diags = append(diags, typeDiags...)
		if !typeDiags.HasErrors() {
			variable.TypeConstraint = ty
			variable.TypeDefaults = defs
		}
	}

	if attr, ok := content.Attributes["default"]; ok {
		variable.Default = attr.Expr
	}

	if attr, ok := content.Attributes["description"]; ok {
		val, valDiags := attr.Expr.Value(nil)
		diags = append(diags, valDiags...)
		if val.Type() == cty.String {
			variable.Description = val.AsString()
		}
	}

	if attr, ok := content.Attributes["sensitive"]; ok {
		val, valDiags := attr.Expr.Value(nil)
		diags = append(diags, valDiags...)
		if val.Type() == cty.Bool {
			variable.Sensitive = val.True()
		}
	}

	if attr, ok := content.Attributes["nullable"]; ok {
		val, valDiags := attr.Expr.Value(nil)
		diags = append(diags, valDiags...)
		if val.Type() == cty.Bool {
			variable.Nullable = val.True()
		}
	}

	for _, subBlock := range content.Blocks {
		if subBlock.Type == "validation" {
			validation, valDiags := p.parseValidationBlock(subBlock)
			diags = append(diags, valDiags...)
			if validation != nil {
				variable.Validations = append(variable.Validations, validation)
			}
		}
	}

	config.Variables[name] = variable
	return diags
}

// parseValidationBlock parses a validation block within a variable.
func (p *Parser) parseValidationBlock(block *hcl.Block) (*ast.Validation, hcl.Diagnostics) {
	content, diags := block.Body.Content(validationSchema)

	validation := &ast.Validation{
		DeclRange: block.DefRange,
	}

	if attr, ok := content.Attributes["condition"]; ok {
		validation.Condition = attr.Expr
	}

	if attr, ok := content.Attributes["error_message"]; ok {
		validation.ErrorMessage = attr.Expr
	}

	return validation, diags
}

// parseLocalsBlock parses a locals block.
func (p *Parser) parseLocalsBlock(config *ast.Config, block *hcl.Block) hcl.Diagnostics {
	var diags hcl.Diagnostics

	attrs, attrDiags := block.Body.JustAttributes()
	diags = append(diags, attrDiags...)

	for name, attr := range attrs {
		if _, exists := config.Locals[name]; exists {
			diags = append(diags, &hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Duplicate local value",
				Detail:   fmt.Sprintf("A local value named %q was already declared.", name),
				Subject:  &attr.Range,
			})
			continue
		}

		config.Locals[name] = &ast.Local{
			Name:      name,
			Value:     attr.Expr,
			DeclRange: attr.Range,
		}
	}

	return diags
}

// parseResourceBlock parses a resource or data block and records it in the
// config's Resources or DataSources map.
func (p *Parser) parseResourceBlock(config *ast.Config, block *hcl.Block, isDataSource bool) hcl.Diagnostics {
	resourceType := block.Labels[0]
	name := block.Labels[1]
	key := ast.ResourceKey(resourceType, name)

	targetMap := config.Resources
	blockType := "resource"
	if isDataSource {
		targetMap = config.DataSources
		blockType = "data source"
	}

	if _, exists := targetMap[key]; exists {
		return hcl.Diagnostics{{
			Severity: hcl.DiagError,
			Summary:  fmt.Sprintf("Duplicate %s", blockType),
			Detail:   fmt.Sprintf("A %s %q %q was already declared.", blockType, resourceType, name),
			Subject:  &block.DefRange,
		}}
	}

	resource, diags := p.decodeResourceBlock(block, isDataSource)
	targetMap[key] = resource
	return diags
}

// decodeResourceBlock decodes a resource or data block body into a Resource
// without recording it in the config, so callers that scope the block
// elsewhere (e.g. a check's data source) can reuse the same decoding.
func (p *Parser) decodeResourceBlock(block *hcl.Block, isDataSource bool) (*ast.Resource, hcl.Diagnostics) {
	var diags hcl.Diagnostics

	resourceType := block.Labels[0]
	name := block.Labels[1]

	content, remain, contentDiags := block.Body.PartialContent(resourceSchema)
	diags = append(diags, contentDiags...)

	resource := &ast.Resource{
		Type:         resourceType,
		Name:         name,
		Config:       remain,
		DeclRange:    block.DefRange,
		TypeRange:    block.LabelRanges[0],
		IsDataSource: isDataSource,
	}

	// Parse meta-arguments
	if attr, ok := content.Attributes["count"]; ok {
		resource.Count = attr.Expr
	}

	if attr, ok := content.Attributes["for_each"]; ok {
		resource.ForEach = attr.Expr
	}

	if attr, ok := content.Attributes["depends_on"]; ok {
		// depends_on should be a list of references
		exprs, exprDiags := hcl.ExprList(attr.Expr)
		diags = append(diags, exprDiags...)
		for _, expr := range exprs {
			traversal, travDiags := hcl.AbsTraversalForExpr(expr)
			diags = append(diags, travDiags...)
			if traversal != nil {
				resource.DependsOn = append(resource.DependsOn, traversal)
			}
		}
	}

	if attr, ok := content.Attributes["provider"]; ok {
		resource.Provider = attr.Expr
	}

	if attr, ok := content.Attributes["providers"]; ok {
		exprs, exprDiags := hcl.ExprList(attr.Expr)
		diags = append(diags, exprDiags...)
		for _, expr := range exprs {
			traversal, travDiags := hcl.AbsTraversalForExpr(expr)
			diags = append(diags, travDiags...)
			if traversal != nil {
				resource.Providers = append(resource.Providers, traversal)
			}
		}
	}

	// Parse nested blocks
	for _, subBlock := range content.Blocks {
		switch subBlock.Type {
		case "pulumi":
			diags = append(diags, p.parsePulumiResourceOptions(subBlock, resource)...)
		case "lifecycle":
			lcResult, lcDiags := p.parseLifecycleBlock(subBlock)
			diags = append(diags, lcDiags...)
			resource.Lifecycle = lcResult.Lifecycle
			resource.Preconditions = append(resource.Preconditions, lcResult.Preconditions...)
			resource.Postconditions = append(resource.Postconditions, lcResult.Postconditions...)
		case "connection":
			conn, connDiags := p.parseConnectionBlock(subBlock)
			diags = append(diags, connDiags...)
			resource.Connection = conn
		case "provisioner":
			prov, provDiags := p.parseProvisionerBlock(subBlock)
			diags = append(diags, provDiags...)
			if prov != nil {
				resource.Provisioners = append(resource.Provisioners, prov)
			}
		case "timeouts":
			timeouts, timeoutsDiags := p.parseTimeoutsBlock(subBlock)
			diags = append(diags, timeoutsDiags...)
			resource.Timeouts = timeouts
		}
	}

	return resource, diags
}

// parsePulumiResourceOptions parses a resource or data block's nested
// `pulumi { }` block, extracting Pulumi-specific options onto resource.
func (p *Parser) parsePulumiResourceOptions(block *hcl.Block, resource *ast.Resource) hcl.Diagnostics {
	content, diags := block.Body.Content(pulumiResourceOptionsSchema)

	if attr, ok := content.Attributes["parent"]; ok {
		traversal, travDiags := hcl.AbsTraversalForExpr(attr.Expr)
		diags = append(diags, travDiags...)
		if traversal != nil {
			resource.ResourceParent = traversal
		}
	}

	if attr, ok := content.Attributes["additional_secret_outputs"]; ok {
		exprs, exprDiags := hcl.ExprList(attr.Expr)
		diags = append(diags, exprDiags...)
		for _, expr := range exprs {
			traversal, travDiags := hcl.RelTraversalForExpr(expr)
			diags = append(diags, travDiags...)
			if traversal != nil {
				resource.AdditionalSecretOutputs = append(resource.AdditionalSecretOutputs, traversal)
			}
		}
	}

	if attr, ok := content.Attributes["retain_on_delete"]; ok {
		resource.RetainOnDelete = attr.Expr
	}

	if attr, ok := content.Attributes["deleted_with"]; ok {
		traversal, travDiags := hcl.AbsTraversalForExpr(attr.Expr)
		diags = append(diags, travDiags...)
		if traversal != nil {
			resource.DeletedWith = traversal
		}
	}

	if attr, ok := content.Attributes["replace_with"]; ok {
		exprs, exprDiags := hcl.ExprList(attr.Expr)
		diags = append(diags, exprDiags...)
		for _, expr := range exprs {
			traversal, travDiags := hcl.AbsTraversalForExpr(expr)
			diags = append(diags, travDiags...)
			if traversal != nil {
				resource.ReplaceWith = append(resource.ReplaceWith, traversal)
			}
		}
	}

	if attr, ok := content.Attributes["hide_diffs"]; ok {
		exprs, exprDiags := hcl.ExprList(attr.Expr)
		diags = append(diags, exprDiags...)
		for _, expr := range exprs {
			traversal, travDiags := hcl.RelTraversalForExpr(expr)
			diags = append(diags, travDiags...)
			if traversal != nil {
				resource.HideDiff = append(resource.HideDiff, traversal)
			}
		}
	}

	if attr, ok := content.Attributes["replace_on_changes"]; ok {
		exprs, exprDiags := hcl.ExprList(attr.Expr)
		diags = append(diags, exprDiags...)
		for _, expr := range exprs {
			traversal, travDiags := hcl.RelTraversalForExpr(expr)
			diags = append(diags, travDiags...)
			if traversal != nil {
				resource.ReplaceOnChanges = append(resource.ReplaceOnChanges, traversal)
			}
		}
	}

	if attr, ok := content.Attributes["import_id"]; ok {
		val, valDiags := attr.Expr.Value(nil)
		diags = append(diags, valDiags...)
		if val.Type() == cty.String {
			resource.ImportID = val.AsString()
		}
	}

	if attr, ok := content.Attributes["env_var_mappings"]; ok {
		resource.EnvVarMappings = attr.Expr
	}

	if attr, ok := content.Attributes["version"]; ok {
		resource.Version = attr.Expr
	}

	if attr, ok := content.Attributes["plugin_download_url"]; ok {
		resource.PluginDownloadURL = attr.Expr
	}

	if attr, ok := content.Attributes["aliases"]; ok {
		resource.Aliases = attr.Expr
	}

	return diags
}

// lifecycleResult contains the parsed lifecycle block plus any preconditions/postconditions.
type lifecycleResult struct {
	Lifecycle      *ast.Lifecycle
	Preconditions  []*ast.CheckRule
	Postconditions []*ast.CheckRule
}

// parseLifecycleBlock parses a lifecycle block.
func (p *Parser) parseLifecycleBlock(block *hcl.Block) (*lifecycleResult, hcl.Diagnostics) {
	content, diags := block.Body.Content(lifecycleSchema)

	lifecycle := &ast.Lifecycle{
		DeclRange: block.DefRange,
	}

	if attr, ok := content.Attributes["create_before_destroy"]; ok {
		val, valDiags := attr.Expr.Value(nil)
		diags = append(diags, valDiags...)
		if val.Type() == cty.Bool {
			b := val.True()
			lifecycle.CreateBeforeDestroy = &b
		}
	}

	if attr, ok := content.Attributes["prevent_destroy"]; ok {
		val, valDiags := attr.Expr.Value(nil)
		diags = append(diags, valDiags...)
		if val.Type() == cty.Bool {
			lifecycle.PreventDestroy = ptr(val.True())
		}
	}

	if attr, ok := content.Attributes["ignore_changes"]; ok {
		// Check for "all" keyword
		kw := hcl.ExprAsKeyword(attr.Expr)
		if kw == "all" {
			lifecycle.IgnoreAllChanges = true
		} else {
			// Parse as list of traversals
			exprs, exprDiags := hcl.ExprList(attr.Expr)
			diags = append(diags, exprDiags...)
			for _, expr := range exprs {
				traversal, travDiags := hcl.RelTraversalForExpr(expr)
				diags = append(diags, travDiags...)
				if traversal != nil {
					lifecycle.IgnoreChanges = append(lifecycle.IgnoreChanges, traversal)
				}
			}
		}
	}

	if attr, ok := content.Attributes["replace_triggered_by"]; ok {
		exprs, exprDiags := hcl.ExprList(attr.Expr)
		diags = append(diags, exprDiags...)
		lifecycle.ReplaceTriggeredBy = exprs
	}

	result := &lifecycleResult{
		Lifecycle: lifecycle,
	}

	// Parse preconditions and postconditions
	for _, subBlock := range content.Blocks {
		switch subBlock.Type {
		case "precondition":
			rule, ruleDiags := p.parseCheckRule(subBlock)
			diags = append(diags, ruleDiags...)
			if rule != nil {
				result.Preconditions = append(result.Preconditions, rule)
			}
		case "postcondition":
			rule, ruleDiags := p.parseCheckRule(subBlock)
			diags = append(diags, ruleDiags...)
			if rule != nil {
				result.Postconditions = append(result.Postconditions, rule)
			}
		}
	}

	return result, diags
}

// parseConnectionBlock parses a connection block. The block body is stored
// verbatim on the Connection so the runtime can re-evaluate every attribute
// (including ones the parser inspected like `type`) against the live HCL
// eval context — PartialContent strips recognized attrs from `remain`, so
// using it here would hide `host`, `user`, etc. from the runtime.
func (p *Parser) parseConnectionBlock(block *hcl.Block) (*ast.Connection, hcl.Diagnostics) {
	content, _, diags := block.Body.PartialContent(connectionSchema)

	conn := &ast.Connection{
		Config:    block.Body,
		DeclRange: block.DefRange,
	}

	if attr, ok := content.Attributes["type"]; ok {
		val, valDiags := attr.Expr.Value(nil)
		diags = append(diags, valDiags...)
		if val.Type() == cty.String {
			conn.Type = val.AsString()
		}
	}

	if conn.Type == "" {
		conn.Type = "ssh" // Default
	}

	return conn, diags
}

// exprAsKeywordOrString returns the value of expr if it's either a bare
// keyword (e.g. `continue`) or a string literal (`"continue"`). TF accepts
// both forms for `when` and `on_failure`; we match that.
func exprAsKeywordOrString(expr hcl.Expression) string {
	if kw := hcl.ExprAsKeyword(expr); kw != "" {
		return kw
	}
	val, diags := expr.Value(nil)
	if diags.HasErrors() || !val.IsKnown() || val.IsNull() {
		return ""
	}
	if val.Type() != cty.String {
		return ""
	}
	return val.AsString()
}

// parseProvisionerBlock parses a provisioner block.
func (p *Parser) parseProvisionerBlock(block *hcl.Block) (*ast.Provisioner, hcl.Diagnostics) {
	content, remain, diags := block.Body.PartialContent(provisionerSchema)

	provisioner := &ast.Provisioner{
		Type:      block.Labels[0],
		Config:    remain,
		When:      "create", // Default
		OnFailure: "fail",   // Default
		DeclRange: block.DefRange,
	}

	if attr, ok := content.Attributes["when"]; ok {
		if kw := exprAsKeywordOrString(attr.Expr); kw != "" {
			if kw != "create" && kw != "destroy" {
				diags = append(diags, &hcl.Diagnostic{
					Severity: hcl.DiagError,
					Summary:  "Invalid \"when\" value",
					Detail:   fmt.Sprintf("Expected \"create\" or \"destroy\", got %q.", kw),
					Subject:  attr.Expr.Range().Ptr(),
				})
			} else {
				provisioner.When = kw
			}
		}
	}

	if attr, ok := content.Attributes["on_failure"]; ok {
		if kw := exprAsKeywordOrString(attr.Expr); kw != "" {
			if kw != "fail" && kw != "continue" {
				diags = append(diags, &hcl.Diagnostic{
					Severity: hcl.DiagError,
					Summary:  "Invalid \"on_failure\" value",
					Detail:   fmt.Sprintf("Expected \"fail\" or \"continue\", got %q.", kw),
					Subject:  attr.Expr.Range().Ptr(),
				})
			} else {
				provisioner.OnFailure = kw
			}
		}
	}

	// Parse connection override
	for _, subBlock := range content.Blocks {
		if subBlock.Type == "connection" {
			conn, connDiags := p.parseConnectionBlock(subBlock)
			diags = append(diags, connDiags...)
			provisioner.Connection = conn
		}
	}

	return provisioner, diags
}

// parseTimeoutsBlock parses a timeouts block.
func (p *Parser) parseTimeoutsBlock(block *hcl.Block) (*ast.Timeouts, hcl.Diagnostics) {
	content, diags := block.Body.Content(timeoutsSchema)

	timeouts := &ast.Timeouts{
		DeclRange: block.DefRange,
	}

	if attr, ok := content.Attributes["create"]; ok {
		timeouts.Create = attr.Expr
	}

	if attr, ok := content.Attributes["read"]; ok {
		timeouts.Read = attr.Expr
	}

	if attr, ok := content.Attributes["update"]; ok {
		timeouts.Update = attr.Expr
	}

	if attr, ok := content.Attributes["delete"]; ok {
		timeouts.Delete = attr.Expr
	}

	return timeouts, diags
}

// parseOutputBlock parses an output block.
func (p *Parser) parseOutputBlock(config *ast.Config, block *hcl.Block) hcl.Diagnostics {
	var diags hcl.Diagnostics

	name := block.Labels[0]
	if _, exists := config.Outputs[name]; exists {
		diags = append(diags, &hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Duplicate output",
			Detail:   fmt.Sprintf("An output named %q was already declared.", name),
			Subject:  &block.DefRange,
		})
		return diags
	}

	content, contentDiags := block.Body.Content(outputSchema)
	diags = append(diags, contentDiags...)

	output := &ast.Output{
		Name:      name,
		DeclRange: block.DefRange,
	}

	if attr, ok := content.Attributes["value"]; ok {
		output.Value = attr.Expr
	}

	if attr, ok := content.Attributes["description"]; ok {
		val, valDiags := attr.Expr.Value(nil)
		diags = append(diags, valDiags...)
		if val.Type() == cty.String {
			output.Description = val.AsString()
		}
	}

	if attr, ok := content.Attributes["sensitive"]; ok {
		val, valDiags := attr.Expr.Value(nil)
		diags = append(diags, valDiags...)
		if val.Type() == cty.Bool {
			output.Sensitive = val.True()
		}
	}

	if attr, ok := content.Attributes["depends_on"]; ok {
		exprs, exprDiags := hcl.ExprList(attr.Expr)
		diags = append(diags, exprDiags...)
		for _, expr := range exprs {
			traversal, travDiags := hcl.AbsTraversalForExpr(expr)
			diags = append(diags, travDiags...)
			if traversal != nil {
				output.DependsOn = append(output.DependsOn, traversal)
			}
		}
	}

	// Parse preconditions
	for _, subBlock := range content.Blocks {
		if subBlock.Type == "precondition" {
			rule, ruleDiags := p.parseCheckRule(subBlock)
			diags = append(diags, ruleDiags...)
			if rule != nil {
				output.Preconditions = append(output.Preconditions, rule)
			}
		}
	}

	config.Outputs[name] = output
	return diags
}

// parseCheckBlock parses a top-level check block. Each assert block reuses the
// condition/error_message schema shared with preconditions. A check may declare
// at most one scoped data source, which is decoded onto the check and read, at
// evaluation time, into a context visible only to the check's assertions.
func (p *Parser) parseCheckBlock(config *ast.Config, block *hcl.Block) hcl.Diagnostics {
	var diags hcl.Diagnostics

	name := block.Labels[0]
	if _, exists := config.Checks[name]; exists {
		diags = append(diags, &hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Duplicate check",
			Detail:   fmt.Sprintf("A check named %q was already declared.", name),
			Subject:  &block.DefRange,
		})
		return diags
	}

	content, contentDiags := block.Body.Content(checkSchema)
	diags = append(diags, contentDiags...)

	check := &ast.Check{
		Name:      name,
		DeclRange: block.DefRange,
	}

	for _, subBlock := range content.Blocks {
		switch subBlock.Type {
		case "assert":
			rule, ruleDiags := p.parseCheckRule(subBlock)
			diags = append(diags, ruleDiags...)
			if rule != nil {
				check.Asserts = append(check.Asserts, rule)
			}
		case "data":
			if check.DataResource != nil {
				diags = append(diags, &hcl.Diagnostic{
					Severity: hcl.DiagError,
					Summary:  "Multiple data resource blocks",
					Detail: fmt.Sprintf("This check block already has a data resource defined at %s.",
						check.DataResource.DeclRange),
					Subject: &subBlock.DefRange,
				})
				continue
			}
			ds, dsDiags := p.decodeResourceBlock(subBlock, true)
			diags = append(diags, dsDiags...)
			check.DataResource = ds
		}
	}

	if len(check.Asserts) == 0 {
		diags = append(diags, &hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Missing assert block",
			Detail:   "A check block must contain at least one assert block.",
			Subject:  &block.DefRange,
		})
		return diags
	}

	config.Checks[name] = check
	return diags
}

// parseCheckRule parses a precondition/postcondition block.
func (p *Parser) parseCheckRule(block *hcl.Block) (*ast.CheckRule, hcl.Diagnostics) {
	content, diags := block.Body.Content(preconditionSchema)

	rule := &ast.CheckRule{
		DeclRange: block.DefRange,
	}

	if attr, ok := content.Attributes["condition"]; ok {
		rule.Condition = attr.Expr
	}

	if attr, ok := content.Attributes["error_message"]; ok {
		rule.ErrorMessage = attr.Expr
	}

	return rule, diags
}

// parseModuleBlock parses a module block.
func (p *Parser) parseModuleBlock(config *ast.Config, block *hcl.Block) hcl.Diagnostics {
	var diags hcl.Diagnostics

	name := block.Labels[0]
	if _, exists := config.Modules[name]; exists {
		diags = append(diags, &hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Duplicate module",
			Detail:   fmt.Sprintf("A module named %q was already declared.", name),
			Subject:  &block.DefRange,
		})
		return diags
	}

	content, remain, contentDiags := block.Body.PartialContent(moduleSchema)
	diags = append(diags, contentDiags...)

	module := &ast.Module{
		Name:      name,
		Config:    remain,
		Providers: make(map[string]hcl.Expression),
		DeclRange: block.DefRange,
	}

	if attr, ok := content.Attributes["source"]; ok {
		val, valDiags := attr.Expr.Value(nil)
		diags = append(diags, valDiags...)
		if val.Type() == cty.String {
			module.Source = val.AsString()
		}
	}

	if attr, ok := content.Attributes["version"]; ok {
		val, valDiags := attr.Expr.Value(nil)
		diags = append(diags, valDiags...)
		if val.Type() == cty.String {
			module.Version = val.AsString()
		}
	}

	if attr, ok := content.Attributes["count"]; ok {
		module.Count = attr.Expr
	}

	if attr, ok := content.Attributes["for_each"]; ok {
		module.ForEach = attr.Expr
	}

	if attr, ok := content.Attributes["depends_on"]; ok {
		exprs, exprDiags := hcl.ExprList(attr.Expr)
		diags = append(diags, exprDiags...)
		for _, expr := range exprs {
			traversal, travDiags := hcl.AbsTraversalForExpr(expr)
			diags = append(diags, travDiags...)
			if traversal != nil {
				module.DependsOn = append(module.DependsOn, traversal)
			}
		}
	}

	// Parse providers map. Keys may be bare provider names ("simple"),
	// dotted local aliases ("simple.foo"), or quoted strings. HCL flags a
	// bare dotted form as "Ambiguous attribute key" at Value() time, so
	// resolve via AbsTraversalForExpr first (it goes through the
	// ObjectConsKeyExpr wrapper without triggering that diagnostic).
	if attr, ok := content.Attributes["providers"]; ok {
		pairs, pairDiags := hcl.ExprMap(attr.Expr)
		diags = append(diags, pairDiags...)
		for _, pair := range pairs {
			key, keyDiags := providersMapKey(pair.Key)
			diags = append(diags, keyDiags...)
			if key == "" {
				continue
			}
			module.Providers[key] = pair.Value
		}
	}

	config.Modules[name] = module
	return diags
}

// providersMapKey resolves a key from a module-call's `providers = {...}`
// map to its canonical "name" or "name.alias" string form. Reports a
// diagnostic when the key isn't a traversal (bare or dotted) or a static
// string.
func providersMapKey(key hcl.Expression) (string, hcl.Diagnostics) {
	if trav, td := hcl.AbsTraversalForExpr(key); !td.HasErrors() && len(trav) > 0 {
		var name strings.Builder
		name.WriteString(trav.RootName())
		for i := 1; i < len(trav); i++ {
			attr, ok := trav[i].(hcl.TraverseAttr)
			if !ok {
				return "", hcl.Diagnostics{&hcl.Diagnostic{
					Severity: hcl.DiagError,
					Summary:  "Invalid provider map key",
					Detail:   "Each key in a module's `providers = {...}` map must be a local provider name, optionally followed by a single alias step (e.g. `simple` or `simple.foo`).",
					Subject:  key.Range().Ptr(),
				}}
			}
			name.WriteString("." + attr.Name)
		}
		return name.String(), nil
	}
	v, vd := key.Value(nil)
	if vd.HasErrors() {
		return "", vd
	}
	if v.Type() != cty.String || v.IsNull() {
		return "", hcl.Diagnostics{&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Invalid provider map key",
			Detail:   "Each key in a module's `providers = {...}` map must be a local provider name, optionally followed by a single alias step (e.g. `simple` or `simple.foo`), or a string literal of that form.",
			Subject:  key.Range().Ptr(),
		}}
	}
	return v.AsString(), nil
}

// parseMovedBlock parses a moved block.
func (p *Parser) parseMovedBlock(config *ast.Config, block *hcl.Block) hcl.Diagnostics {
	content, diags := block.Body.Content(movedSchema)

	moved := &ast.Moved{
		DeclRange: block.DefRange,
	}

	if attr, ok := content.Attributes["from"]; ok {
		traversal, travDiags := hcl.AbsTraversalForExpr(attr.Expr)
		diags = append(diags, travDiags...)
		if traversal != nil {
			moved.From = traversal
		}
	}

	if attr, ok := content.Attributes["to"]; ok {
		traversal, travDiags := hcl.AbsTraversalForExpr(attr.Expr)
		diags = append(diags, travDiags...)
		if traversal != nil {
			moved.To = traversal
		}
	}

	config.Moved = append(config.Moved, moved)
	return diags
}

// parseCallBlock parses a call block.
func (p *Parser) parseCallBlock(config *ast.Config, block *hcl.Block) hcl.Diagnostics {
	var diags hcl.Diagnostics

	resourceName := block.Labels[0]
	methodName := block.Labels[1]
	key := ast.CallKey(resourceName, methodName)

	if _, exists := config.Calls[key]; exists {
		diags = append(diags, &hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Duplicate call block",
			Detail:   fmt.Sprintf("A call block for %q.%q was already declared.", resourceName, methodName),
			Subject:  &block.DefRange,
		})
		return diags
	}

	config.Calls[key] = &ast.Call{
		ResourceName: resourceName,
		MethodName:   methodName,
		Config:       block.Body,
		DeclRange:    block.DefRange,
	}
	return diags
}

// parseImportBlock parses an import block.
func (p *Parser) parseImportBlock(config *ast.Config, block *hcl.Block) hcl.Diagnostics {
	content, diags := block.Body.Content(importSchema)

	imp := &ast.Import{
		DeclRange: block.DefRange,
	}

	if attr, ok := content.Attributes["to"]; ok {
		traversal, travDiags := hcl.AbsTraversalForExpr(attr.Expr)
		diags = append(diags, travDiags...)
		if traversal != nil {
			imp.To = traversal
		}
	}

	if attr, ok := content.Attributes["id"]; ok {
		val, valDiags := attr.Expr.Value(nil)
		diags = append(diags, valDiags...)
		if val.Type() == cty.String {
			imp.Id = val.AsString()
		}
	}

	if attr, ok := content.Attributes["provider"]; ok {
		imp.Provider = attr.Expr
	}

	config.Imports = append(config.Imports, imp)
	return diags
}

func ptr[T any](v T) *T { return &v }
