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

	"github.com/blang/semver"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/ext/typeexpr"
	"github.com/pulumi/pulumi-language-hcl/pkg/hcl/ast"
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

	for _, file := range files {
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

	config.Diagnostics = diags
	return config, diags
}

// parseBlock parses a single top-level block.
func (p *Parser) parseBlock(config *ast.Config, block *hcl.Block) hcl.Diagnostics {
	switch block.Type {
	case "terraform":
		return p.parseTerraformBlock(config, block)
	case "pulumi":
		return p.parsePulumiBlock(config, block)
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

// parseTerraformBlock parses a terraform block.
func (p *Parser) parseTerraformBlock(config *ast.Config, block *hcl.Block) hcl.Diagnostics {
	var diags hcl.Diagnostics

	if config.Terraform != nil {
		diags = append(diags, &hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Duplicate terraform block",
			Detail:   "Only one terraform block is allowed per configuration.",
			Subject:  &block.DefRange,
		})
		return diags
	}

	content, contentDiags := block.Body.Content(terraformSchema)
	diags = append(diags, contentDiags...)

	terraform := &ast.Terraform{
		RequiredProviders: make(map[string]*ast.RequiredProvider),
		DeclRange:         block.DefRange,
	}

	if attr, ok := content.Attributes["required_version"]; ok {
		val, valDiags := attr.Expr.Value(nil)
		diags = append(diags, valDiags...)
		if val.Type() == cty.String {
			terraform.RequiredVersion = val.AsString()
		}
	}

	for _, subBlock := range content.Blocks {
		switch subBlock.Type {
		case "required_providers":
			providerDiags := p.parseRequiredProviders(terraform, subBlock)
			diags = append(diags, providerDiags...)
		case "backend":
			terraform.Backend = &ast.Backend{
				Type:      subBlock.Labels[0],
				Config:    subBlock.Body,
				DeclRange: subBlock.DefRange,
			}
		case "cloud":
			// Cloud block is ignored for Pulumi
		}
	}

	config.Terraform = terraform
	return diags
}

// parsePulumiBlock parses a pulumi block.
func (p *Parser) parsePulumiBlock(config *ast.Config, block *hcl.Block) hcl.Diagnostics {
	var diags hcl.Diagnostics

	if config.Pulumi != nil {
		diags = append(diags, &hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Duplicate pulumi block",
			Detail:   "Only one pulumi block is allowed per configuration.",
			Subject:  &block.DefRange,
		})
		return diags
	}

	content, contentDiags := block.Body.Content(pulumiSchema)
	diags = append(diags, contentDiags...)

	if len(block.Labels) != 0 {
		diags = append(diags, &hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Invalid pulumi block",
			Detail:   "The pulumi block does not accept any labels.",
			Subject:  &block.DefRange,
		})
		return diags
	}

	pulumi := &ast.Pulumi{
		DeclRange: block.DefRange,
	}

	if attr, ok := content.Attributes["required_version_range"]; ok {
		pulumi.RequiredVersionRange = attr.Expr
	}

	for _, subBlock := range content.Blocks {
		switch subBlock.Type {
		case "component":
			if pulumi.Component != nil {
				diags = append(diags, &hcl.Diagnostic{
					Severity: hcl.DiagError,
					Summary:  "Duplicate component block",
					Detail:   "Only one component block is allowed per pulumi block.",
					Subject:  &subBlock.DefRange,
				})
				continue
			}
			comp, compDiags := p.parsePulumiComponentBlock(subBlock)
			diags = append(diags, compDiags...)
			pulumi.Component = comp
		case "package":
			if pulumi.Package != nil {
				diags = append(diags, &hcl.Diagnostic{
					Severity: hcl.DiagError,
					Summary:  "Duplicate package block",
					Detail:   "Only one package block is allowed per pulumi block.",
					Subject:  &subBlock.DefRange,
				})
				continue
			}
			pkg, pkgDiags := p.parsePulumiPackageBlock(subBlock)
			diags = append(diags, pkgDiags...)
			pulumi.Package = pkg
		}
	}

	config.Pulumi = pulumi
	return diags
}

// parsePulumiComponentBlock parses a component sub-block within pulumi.
func (p *Parser) parsePulumiComponentBlock(block *hcl.Block) (*ast.ComponentBlock, hcl.Diagnostics) {
	content, diags := block.Body.Content(pulumiComponentSchema)

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

// parsePulumiPackageBlock parses a package sub-block within pulumi.
func (p *Parser) parsePulumiPackageBlock(block *hcl.Block) (*ast.PackageBlock, hcl.Diagnostics) {
	content, diags := block.Body.Content(pulumiPackageSchema)

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
func (p *Parser) parseRequiredProviders(terraform *ast.Terraform, block *hcl.Block) hcl.Diagnostics {
	var diags hcl.Diagnostics

	attrs, attrDiags := block.Body.JustAttributes()
	diags = append(diags, attrDiags...)

	for name, attr := range attrs {
		provider := &ast.RequiredProvider{
			Name:      name,
			DeclRange: attr.Range,
		}

		// The value can be a string (version only) or an object
		val, valDiags := attr.Expr.Value(nil)
		diags = append(diags, valDiags...)

		if val.Type() == cty.String {
			provider.Version = val.AsString()
		} else if val.Type().IsObjectType() {
			if sourceVal := val.GetAttr("source"); !sourceVal.IsNull() {
				provider.Source = sourceVal.AsString()
			}
			if versionVal := val.GetAttr("version"); !versionVal.IsNull() {
				provider.Version = versionVal.AsString()
			}
		}

		terraform.RequiredProviders[name] = provider
	}

	return diags
}

// parseProviderBlock parses a provider block.
func (p *Parser) parseProviderBlock(config *ast.Config, block *hcl.Block) hcl.Diagnostics {
	var diags hcl.Diagnostics

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
		ty, typeDiags := typeexpr.TypeConstraint(attr.Expr)
		diags = append(diags, typeDiags...)
		if !typeDiags.HasErrors() {
			variable.TypeConstraint = ty
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

// parseResourceBlock parses a resource or data block.
func (p *Parser) parseResourceBlock(config *ast.Config, block *hcl.Block, isDataSource bool) hcl.Diagnostics {
	var diags hcl.Diagnostics

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
		diags = append(diags, &hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  fmt.Sprintf("Duplicate %s", blockType),
			Detail:   fmt.Sprintf("A %s %q %q was already declared.", blockType, resourceType, name),
			Subject:  &block.DefRange,
		})
		return diags
	}

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
		providerRef, refDiags := p.parseProviderRef(attr.Expr)
		diags = append(diags, refDiags...)
		resource.Provider = providerRef
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

	if attr, ok := content.Attributes["parent"]; ok {
		traversal, travDiags := hcl.AbsTraversalForExpr(attr.Expr)
		diags = append(diags, travDiags...)
		if traversal != nil {
			resource.ResourceParent = traversal
		}
	}

	if attr, ok := content.Attributes["additional_secret_outputs"]; ok {
		resource.AdditionalSecretOutputs = attr.Expr
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
			// Try as string literal first (preferred: "propertyName" in camelCase)
			val, valDiags := expr.Value(nil)
			if !valDiags.HasErrors() && val.Type() == cty.String {
				resource.HideDiff = append(resource.HideDiff, val.AsString())
			} else {
				// Fallback: bare identifier traversal (e.g. for hand-written HCL)
				traversal, travDiags := hcl.RelTraversalForExpr(expr)
				diags = append(diags, travDiags...)
				if traversal != nil {
					resource.HideDiff = append(resource.HideDiff, traversal.RootName())
				}
			}
		}
	}

	if attr, ok := content.Attributes["replace_on_changes"]; ok {
		exprs, exprDiags := hcl.ExprList(attr.Expr)
		diags = append(diags, exprDiags...)
		for _, expr := range exprs {
			// Try as string literal first (preferred: "propertyName" in camelCase)
			val, valDiags := expr.Value(nil)
			if !valDiags.HasErrors() && val.Type() == cty.String {
				resource.ReplaceOnChanges = append(resource.ReplaceOnChanges, val.AsString())
			} else {
				// Fallback: bare identifier traversal (e.g. for hand-written HCL)
				traversal, travDiags := hcl.RelTraversalForExpr(expr)
				diags = append(diags, travDiags...)
				if traversal != nil {
					resource.ReplaceOnChanges = append(resource.ReplaceOnChanges, traversal.RootName())
				}
			}
		}
	}

	if attr, ok := content.Attributes["replacement_trigger"]; ok {
		resource.ReplacementTrigger = attr.Expr
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

	// Parse nested blocks
	for _, subBlock := range content.Blocks {
		switch subBlock.Type {
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

	targetMap[key] = resource
	return diags
}

// parseProviderRef parses a provider reference expression.
func (p *Parser) parseProviderRef(expr hcl.Expression) (*ast.ProviderRef, hcl.Diagnostics) {
	traversal, diags := hcl.AbsTraversalForExpr(expr)
	if diags.HasErrors() {
		return nil, diags
	}

	ref := &ast.ProviderRef{
		Range: expr.Range(),
	}

	if len(traversal) >= 1 {
		ref.Name = traversal.RootName()
	}

	if len(traversal) >= 2 {
		if step, ok := traversal[1].(hcl.TraverseAttr); ok {
			ref.Alias = step.Name
		}
	}

	return ref, diags
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
			lifecycle.PreventDestroy = new(val.True())
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

// parseConnectionBlock parses a connection block.
func (p *Parser) parseConnectionBlock(block *hcl.Block) (*ast.Connection, hcl.Diagnostics) {
	content, remain, diags := block.Body.PartialContent(connectionSchema)

	conn := &ast.Connection{
		Config:    remain,
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
		kw := hcl.ExprAsKeyword(attr.Expr)
		if kw != "" {
			provisioner.When = kw
		}
	}

	if attr, ok := content.Attributes["on_failure"]; ok {
		kw := hcl.ExprAsKeyword(attr.Expr)
		if kw != "" {
			provisioner.OnFailure = kw
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
		val, valDiags := attr.Expr.Value(nil)
		diags = append(diags, valDiags...)
		if val.Type() == cty.String {
			timeouts.Create = val.AsString()
		}
	}

	if attr, ok := content.Attributes["read"]; ok {
		val, valDiags := attr.Expr.Value(nil)
		diags = append(diags, valDiags...)
		if val.Type() == cty.String {
			timeouts.Read = val.AsString()
		}
	}

	if attr, ok := content.Attributes["update"]; ok {
		val, valDiags := attr.Expr.Value(nil)
		diags = append(diags, valDiags...)
		if val.Type() == cty.String {
			timeouts.Update = val.AsString()
		}
	}

	if attr, ok := content.Attributes["delete"]; ok {
		val, valDiags := attr.Expr.Value(nil)
		diags = append(diags, valDiags...)
		if val.Type() == cty.String {
			timeouts.Delete = val.AsString()
		}
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
		Providers: make(map[string]*ast.ProviderRef),
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

	// Parse providers map
	if attr, ok := content.Attributes["providers"]; ok {
		pairs, pairDiags := hcl.ExprMap(attr.Expr)
		diags = append(diags, pairDiags...)
		for _, pair := range pairs {
			keyVal, keyDiags := pair.Key.Value(nil)
			diags = append(diags, keyDiags...)
			if keyVal.Type() != cty.String {
				continue
			}
			key := keyVal.AsString()

			ref, refDiags := p.parseProviderRef(pair.Value)
			diags = append(diags, refDiags...)
			if ref != nil {
				module.Providers[key] = ref
			}
		}
	}

	config.Modules[name] = module
	return diags
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
		ref, refDiags := p.parseProviderRef(attr.Expr)
		diags = append(diags, refDiags...)
		imp.Provider = ref
	}

	config.Imports = append(config.Imports, imp)
	return diags
}
