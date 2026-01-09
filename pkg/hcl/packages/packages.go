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

// Package packages handles Pulumi package schema loading and type mapping.
package packages

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/blang/semver"
	"github.com/pulumi/pulumi/sdk/v3/go/common/tokens"
	"github.com/pulumi/pulumi/sdk/v3/go/common/workspace"
)

// PackageLoader loads and caches Pulumi package schemas and provider info.
type PackageLoader struct {
	mu           sync.RWMutex
	schemas      map[string]*PackageSchema
	providerInfo map[string]*ProviderInfo

	// testMode allows TF-style types to pass through without provider lookup.
	// This is useful for unit testing.
	testMode bool
}

// PackageSchema contains a parsed Pulumi package schema.
type PackageSchema struct {
	// Name is the package name (e.g., "aws", "gcp", "azure").
	Name string

	// Version is the package version.
	Version semver.Version

	// Resources maps Pulumi type tokens to resource schemas.
	Resources map[string]*ResourceSchema

	// Functions maps Pulumi type tokens to function schemas.
	Functions map[string]*FunctionSchema

	// rawSchema holds the raw JSON schema for further introspection if needed.
	rawSchema map[string]interface{}
}

// ProviderInfo contains Terraform bridge mapping information from `-get-provider-info`.
// This is only populated for bridged providers.
type ProviderInfo struct {
	// Name is the provider name.
	Name string

	// Version is the provider version.
	Version string

	// Resources maps Terraform resource type names to their Pulumi tokens.
	// E.g., "aws_instance" -> "aws:ec2/instance:Instance"
	Resources map[string]ResourceInfo

	// DataSources maps Terraform data source type names to their Pulumi tokens.
	// E.g., "aws_ami" -> "aws:ec2/getAmi:getAmi"
	DataSources map[string]DataSourceInfo

	// IsBridged indicates whether this provider is bridged from Terraform.
	IsBridged bool

	// ResourceTokens is the set of valid Pulumi resource tokens (for fast validation).
	ResourceTokens map[string]bool

	// FunctionTokens is the set of valid Pulumi function tokens (for fast validation).
	FunctionTokens map[string]bool
}

// ResourceInfo contains information about a Terraform-bridged resource.
type ResourceInfo struct {
	// Tok is the Pulumi type token.
	Tok tokens.Type
}

// DataSourceInfo contains information about a Terraform-bridged data source.
type DataSourceInfo struct {
	// Tok is the Pulumi function token.
	Tok tokens.ModuleMember
}

// ResourceSchema describes a Pulumi resource type.
type ResourceSchema struct {
	// Token is the Pulumi type token (e.g., "aws:s3/bucket:Bucket").
	Token string

	// InputProperties describes the input properties.
	InputProperties map[string]*PropertySchema

	// RequiredInputs lists required input property names.
	RequiredInputs []string

	// OutputProperties describes the output properties (includes inputs).
	OutputProperties map[string]*PropertySchema

	// Description is the resource description.
	Description string
}

// FunctionSchema describes a Pulumi function (data source).
type FunctionSchema struct {
	// Token is the Pulumi function token.
	Token string

	// Inputs describes the function inputs.
	Inputs map[string]*PropertySchema

	// RequiredInputs lists required input property names.
	RequiredInputs []string

	// Outputs describes the function outputs.
	Outputs map[string]*PropertySchema

	// Description is the function description.
	Description string
}

// PropertySchema describes a property's type and metadata.
type PropertySchema struct {
	// Name is the property name.
	Name string

	// Type is the property type.
	Type PropertyType

	// Description is the property description.
	Description string

	// Secret indicates if the property should be treated as a secret.
	Secret bool

	// Default is the default value, if any.
	Default interface{}
}

// PropertyType represents a property's type information.
type PropertyType struct {
	// Kind is the basic type kind.
	Kind PropertyKind

	// ElementType is the element type for arrays/maps.
	ElementType *PropertyType

	// ObjectProperties are properties for object types.
	ObjectProperties map[string]*PropertySchema

	// Ref is a reference to another type.
	Ref string
}

// PropertyKind represents the basic type kinds.
type PropertyKind int

const (
	PropertyKindString PropertyKind = iota
	PropertyKindNumber
	PropertyKindBoolean
	PropertyKindArray
	PropertyKindMap
	PropertyKindObject
	PropertyKindAsset
	PropertyKindArchive
	PropertyKindAny
	PropertyKindRef
)

// NewPackageLoader creates a new package loader.
func NewPackageLoader() *PackageLoader {
	return &PackageLoader{
		schemas:      make(map[string]*PackageSchema),
		providerInfo: make(map[string]*ProviderInfo),
	}
}

// SetTestMode enables or disables test mode.
// In test mode, type resolution skips schema/provider validation.
func (l *PackageLoader) SetTestMode(enabled bool) {
	l.testMode = enabled
}

// PreloadProviders loads provider info for multiple providers in parallel.
// This should be called before processing resources to avoid repeated
// sequential provider invocations during type resolution.
func (l *PackageLoader) PreloadProviders(providerNames []string) {
	if l.testMode || len(providerNames) == 0 {
		return
	}

	var wg sync.WaitGroup
	for _, name := range providerNames {
		// Check if already loaded
		l.mu.RLock()
		_, exists := l.providerInfo[name]
		l.mu.RUnlock()
		if exists {
			continue
		}

		wg.Add(1)
		go func(providerName string) {
			defer wg.Done()
			// LoadProviderInfo handles caching internally
			l.LoadProviderInfo(providerName, nil)
		}(name)
	}
	wg.Wait()
}

// ExtractProviderFromType extracts the provider name from a resource type.
// For example, "aws_instance" returns "aws", "google_compute_instance" returns "google".
func ExtractProviderFromType(resourceType string) string {
	parts := strings.SplitN(resourceType, "_", 2)
	if len(parts) >= 1 {
		return parts[0]
	}
	return ""
}

// LoadPackage loads a package schema by name and optional version constraint.
func (l *PackageLoader) LoadPackage(name string, version *semver.Version) (*PackageSchema, error) {
	l.mu.RLock()
	key := name
	if version != nil {
		key = fmt.Sprintf("%s@%s", name, version.String())
	}
	if schema, ok := l.schemas[key]; ok {
		l.mu.RUnlock()
		return schema, nil
	}
	l.mu.RUnlock()

	// Load the schema
	schema, err := l.loadSchema(name, version)
	if err != nil {
		return nil, err
	}

	l.mu.Lock()
	l.schemas[key] = schema
	l.mu.Unlock()

	return schema, nil
}

// LoadProviderInfo loads the Terraform bridge provider info for a provider.
// Returns nil if the provider is not bridged from Terraform.
func (l *PackageLoader) LoadProviderInfo(name string, version *semver.Version) (*ProviderInfo, error) {
	l.mu.RLock()
	key := name
	if version != nil {
		key = fmt.Sprintf("%s@%s", name, version.String())
	}
	if info, ok := l.providerInfo[key]; ok {
		l.mu.RUnlock()
		return info, nil
	}
	l.mu.RUnlock()

	// Load the provider info
	info, err := l.loadProviderInfo(name, version)
	if err != nil {
		// Not a bridged provider or error - cache as non-bridged
		info = &ProviderInfo{
			Name:        name,
			IsBridged:   false,
			Resources:   make(map[string]ResourceInfo),
			DataSources: make(map[string]DataSourceInfo),
		}
	}

	l.mu.Lock()
	l.providerInfo[key] = info
	l.mu.Unlock()

	return info, nil
}

// loadSchema loads a schema from the provider plugin.
func (l *PackageLoader) loadSchema(name string, version *semver.Version) (*PackageSchema, error) {
	// First try to find the plugin using workspace APIs
	pluginPath, err := l.getPluginPath(name, version)
	if err != nil {
		return nil, fmt.Errorf("finding plugin %s: %w", name, err)
	}

	// Check for schema.json in the plugin directory
	pluginDir := filepath.Dir(pluginPath)
	schemaPath := filepath.Join(pluginDir, "schema.json")
	data, err := os.ReadFile(schemaPath)
	if err != nil {
		// No cached schema, invoke the plugin to get it
		data, err = l.getSchemaFromPlugin(pluginPath)
		if err != nil {
			return nil, fmt.Errorf("getting schema from plugin: %w", err)
		}
	}

	return l.parseSchema(name, data)
}

// hclCacheFileName is the name of the cache file for HCL mappings.
// Stored in the plugin directory (e.g., ~/.pulumi/plugins/resource-aws-v6.0.0/).
const hclCacheFileName = "pulumi-hcl.cache"

// providerInfoCache is the structure stored in the cache file.
type providerInfoCache struct {
	Name        string            `json:"name"`
	Version     string            `json:"version,omitempty"`
	IsBridged   bool              `json:"isBridged,omitempty"`
	Resources   map[string]string `json:"resources,omitempty"`   // TF type -> Pulumi token
	DataSources map[string]string `json:"dataSources,omitempty"` // TF type -> Pulumi token
	// Schema token lists for fast validation (avoids loading full schema)
	ResourceTokens []string `json:"resourceTokens,omitempty"` // Valid Pulumi resource tokens
	FunctionTokens []string `json:"functionTokens,omitempty"` // Valid Pulumi function tokens
}

// loadProviderInfo loads the Terraform bridge provider info.
// It first checks for a cached version in the plugin directory,
// and falls back to invoking `-get-provider-info` if not cached.
func (l *PackageLoader) loadProviderInfo(name string, version *semver.Version) (*ProviderInfo, error) {
	pluginPath, err := l.getPluginPath(name, version)
	if err != nil {
		return nil, fmt.Errorf("finding plugin %s: %w", name, err)
	}

	pluginDir := filepath.Dir(pluginPath)
	cachePath := filepath.Join(pluginDir, hclCacheFileName)

	// Try to load from cache first
	if info, err := l.loadProviderInfoFromCache(name, cachePath); err == nil {
		return info, nil
	}

	// Cache miss - invoke the plugin with -get-provider-info flag
	cmd := exec.Command(pluginPath, "-get-provider-info")
	output, err := cmd.Output()
	if err != nil {
		// Not a bridged provider - cache this fact to avoid re-invoking
		l.cacheNonBridgedProvider(name, cachePath)
		return nil, fmt.Errorf("provider %s is not a Terraform-bridged provider", name)
	}

	info, err := l.parseProviderInfo(name, output)
	if err != nil {
		return nil, err
	}

	// Cache the result for future runs
	l.cacheProviderInfo(info, cachePath)

	return info, nil
}

// loadProviderInfoFromCache loads provider info from a cache file.
func (l *PackageLoader) loadProviderInfoFromCache(name, cachePath string) (*ProviderInfo, error) {
	data, err := os.ReadFile(cachePath)
	if err != nil {
		return nil, err
	}

	var cache providerInfoCache
	if err := json.Unmarshal(data, &cache); err != nil {
		return nil, err
	}

	// Convert cache to ProviderInfo
	info := &ProviderInfo{
		Name:           cache.Name,
		Version:        cache.Version,
		IsBridged:      cache.IsBridged,
		Resources:      make(map[string]ResourceInfo),
		DataSources:    make(map[string]DataSourceInfo),
		ResourceTokens: make(map[string]bool),
		FunctionTokens: make(map[string]bool),
	}

	for tfType, tok := range cache.Resources {
		info.Resources[tfType] = ResourceInfo{Tok: tokens.Type(tok)}
	}

	for tfType, tok := range cache.DataSources {
		info.DataSources[tfType] = DataSourceInfo{Tok: tokens.ModuleMember(tok)}
	}

	// Load cached token sets for fast validation
	for _, tok := range cache.ResourceTokens {
		info.ResourceTokens[tok] = true
	}
	for _, tok := range cache.FunctionTokens {
		info.FunctionTokens[tok] = true
	}

	return info, nil
}

// cacheProviderInfo writes provider info to a cache file.
// It also extracts schema token lists for fast validation.
func (l *PackageLoader) cacheProviderInfo(info *ProviderInfo, cachePath string) {
	cache := providerInfoCache{
		Name:        info.Name,
		Version:     info.Version,
		IsBridged:   info.IsBridged,
		Resources:   make(map[string]string),
		DataSources: make(map[string]string),
	}

	for tfType, res := range info.Resources {
		cache.Resources[tfType] = string(res.Tok)
	}

	for tfType, ds := range info.DataSources {
		cache.DataSources[tfType] = string(ds.Tok)
	}

	// Also extract schema token lists for fast validation
	cache.ResourceTokens, cache.FunctionTokens = l.extractSchemaTokens(info.Name)

	// Update info with the extracted tokens
	info.ResourceTokens = make(map[string]bool)
	info.FunctionTokens = make(map[string]bool)
	for _, tok := range cache.ResourceTokens {
		info.ResourceTokens[tok] = true
	}
	for _, tok := range cache.FunctionTokens {
		info.FunctionTokens[tok] = true
	}

	l.writeCache(cache, cachePath)
}

// cacheNonBridgedProvider caches a non-bridged provider with its schema tokens.
func (l *PackageLoader) cacheNonBridgedProvider(name, cachePath string) {
	cache := providerInfoCache{
		Name:      name,
		IsBridged: false,
	}

	// Extract schema token lists - this is the main data for non-bridged providers
	cache.ResourceTokens, cache.FunctionTokens = l.extractSchemaTokens(name)

	l.writeCache(cache, cachePath)
}

// extractSchemaTokens extracts resource and function token lists from a provider schema.
func (l *PackageLoader) extractSchemaTokens(providerName string) ([]string, []string) {
	schema, err := l.LoadPackage(providerName, nil)
	if err != nil {
		return nil, nil
	}

	resourceTokens := make([]string, 0, len(schema.Resources))
	for tok := range schema.Resources {
		resourceTokens = append(resourceTokens, tok)
	}

	functionTokens := make([]string, 0, len(schema.Functions))
	for tok := range schema.Functions {
		functionTokens = append(functionTokens, tok)
	}

	return resourceTokens, functionTokens
}

// writeCache writes a cache structure to disk.
func (l *PackageLoader) writeCache(cache providerInfoCache, cachePath string) {
	data, err := json.Marshal(cache) // Compact JSON
	if err != nil {
		return
	}

	writingPath := cachePath + ".writing"
	if err := os.WriteFile(writingPath, data, 0644); err != nil {
		return
	}
	os.Rename(writingPath, cachePath)
}

// getPluginPath finds the path to a plugin binary using workspace APIs.
func (l *PackageLoader) getPluginPath(name string, version *semver.Version) (string, error) {
	// Get the plugin directory using workspace APIs
	pluginDir, err := workspace.GetPluginDir()
	if err != nil {
		return "", fmt.Errorf("getting plugin dir: %w", err)
	}

	// Find the plugin directory
	pattern := fmt.Sprintf("resource-%s-v*", name)
	allMatches, err := filepath.Glob(filepath.Join(pluginDir, pattern))
	if err != nil {
		return "", err
	}

	// Filter out .lock files - we only want directories
	var matches []string
	for _, match := range allMatches {
		if !strings.HasSuffix(match, ".lock") {
			matches = append(matches, match)
		}
	}

	if len(matches) == 0 {
		return "", fmt.Errorf("plugin %s not found in %s", name, pluginDir)
	}

	// Sort matches to get consistent ordering (higher versions last)
	// The directory names are like "resource-aws-v6.0.0"
	// Sorting alphabetically will put newer versions after older ones in most cases
	// (this is a simplification - proper semver sorting would be better)

	// If version specified, find exact match
	if version != nil {
		versionStr := version.String()
		for _, match := range matches {
			base := filepath.Base(match)
			if strings.Contains(base, "v"+versionStr) {
				return l.findPluginBinary(match, name)
			}
		}
		return "", fmt.Errorf("plugin %s@%s not found", name, versionStr)
	}

	// Use the last match (typically highest version due to sorting)
	return l.findPluginBinary(matches[len(matches)-1], name)
}

// findPluginBinary finds the plugin binary in a plugin directory.
func (l *PackageLoader) findPluginBinary(dir, name string) (string, error) {
	binaryName := fmt.Sprintf("pulumi-resource-%s", name)
	if runtime.GOOS == "windows" {
		binaryName += ".exe"
	}

	path := filepath.Join(dir, binaryName)
	if _, err := os.Stat(path); err == nil {
		return path, nil
	}

	return "", fmt.Errorf("plugin binary not found in %s", dir)
}

// getSchemaFromPlugin invokes the plugin to get its schema.
func (l *PackageLoader) getSchemaFromPlugin(pluginPath string) ([]byte, error) {
	// Most plugins support GetSchema via the gRPC interface, but we can also
	// use `pulumi package get-schema` as a fallback
	pluginDir := filepath.Dir(pluginPath)
	pluginName := filepath.Base(pluginDir)

	// Extract the provider name from the directory name (e.g., "resource-aws-v6.0.0" -> "aws")
	parts := strings.Split(pluginName, "-")
	if len(parts) >= 2 {
		name := parts[1]
		cmd := exec.Command("pulumi", "package", "get-schema", name)
		return cmd.Output()
	}

	return nil, fmt.Errorf("could not determine provider name from %s", pluginPath)
}

// parseSchema parses a JSON schema into a PackageSchema.
func (l *PackageLoader) parseSchema(name string, data []byte) (*PackageSchema, error) {
	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parsing schema JSON: %w", err)
	}

	schema := &PackageSchema{
		Name:      name,
		Resources: make(map[string]*ResourceSchema),
		Functions: make(map[string]*FunctionSchema),
		rawSchema: raw,
	}

	// Parse version
	if versionStr, ok := raw["version"].(string); ok {
		if v, err := semver.Parse(versionStr); err == nil {
			schema.Version = v
		}
	}

	// Parse resources
	if resources, ok := raw["resources"].(map[string]interface{}); ok {
		for token, resData := range resources {
			resSchema := l.parseResourceSchema(token, resData)
			schema.Resources[token] = resSchema
		}
	}

	// Parse functions
	if functions, ok := raw["functions"].(map[string]interface{}); ok {
		for token, funcData := range functions {
			funcSchema := l.parseFunctionSchema(token, funcData)
			schema.Functions[token] = funcSchema
		}
	}

	return schema, nil
}

// parseProviderInfo parses the JSON output from `-get-provider-info`.
func (l *PackageLoader) parseProviderInfo(name string, data []byte) (*ProviderInfo, error) {
	// The structure from pulumi-terraform-bridge is:
	// {
	//   "name": "aws",
	//   "version": "6.0.0",
	//   "resources": {
	//     "aws_instance": { "tok": "aws:ec2/instance:Instance", ... },
	//     ...
	//   },
	//   "dataSources": {
	//     "aws_ami": { "tok": "aws:ec2/getAmi:getAmi", ... },
	//     ...
	//   }
	// }
	var raw struct {
		Name        string `json:"name"`
		Version     string `json:"version"`
		Resources   map[string]struct {
			Tok string `json:"tok"`
		} `json:"resources"`
		DataSources map[string]struct {
			Tok string `json:"tok"`
		} `json:"dataSources"`
	}

	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parsing provider info JSON: %w", err)
	}

	info := &ProviderInfo{
		Name:        name,
		Version:     raw.Version,
		IsBridged:   true,
		Resources:   make(map[string]ResourceInfo),
		DataSources: make(map[string]DataSourceInfo),
	}

	for tfType, res := range raw.Resources {
		info.Resources[tfType] = ResourceInfo{
			Tok: tokens.Type(res.Tok),
		}
	}

	for tfType, ds := range raw.DataSources {
		info.DataSources[tfType] = DataSourceInfo{
			Tok: tokens.ModuleMember(ds.Tok),
		}
	}

	return info, nil
}

// parseResourceSchema parses a resource schema from JSON.
func (l *PackageLoader) parseResourceSchema(token string, data interface{}) *ResourceSchema {
	resMap, ok := data.(map[string]interface{})
	if !ok {
		return &ResourceSchema{Token: token}
	}

	res := &ResourceSchema{
		Token:            token,
		InputProperties:  make(map[string]*PropertySchema),
		OutputProperties: make(map[string]*PropertySchema),
	}

	if desc, ok := resMap["description"].(string); ok {
		res.Description = desc
	}

	if inputProps, ok := resMap["inputProperties"].(map[string]interface{}); ok {
		for propName, propData := range inputProps {
			res.InputProperties[propName] = l.parsePropertySchema(propName, propData)
		}
	}

	if required, ok := resMap["requiredInputs"].([]interface{}); ok {
		for _, r := range required {
			if s, ok := r.(string); ok {
				res.RequiredInputs = append(res.RequiredInputs, s)
			}
		}
	}

	if props, ok := resMap["properties"].(map[string]interface{}); ok {
		for propName, propData := range props {
			res.OutputProperties[propName] = l.parsePropertySchema(propName, propData)
		}
	}

	return res
}

// parseFunctionSchema parses a function schema from JSON.
func (l *PackageLoader) parseFunctionSchema(token string, data interface{}) *FunctionSchema {
	funcMap, ok := data.(map[string]interface{})
	if !ok {
		return &FunctionSchema{Token: token}
	}

	fn := &FunctionSchema{
		Token:   token,
		Inputs:  make(map[string]*PropertySchema),
		Outputs: make(map[string]*PropertySchema),
	}

	if desc, ok := funcMap["description"].(string); ok {
		fn.Description = desc
	}

	if inputs, ok := funcMap["inputs"].(map[string]interface{}); ok {
		if props, ok := inputs["properties"].(map[string]interface{}); ok {
			for propName, propData := range props {
				fn.Inputs[propName] = l.parsePropertySchema(propName, propData)
			}
		}
		if required, ok := inputs["required"].([]interface{}); ok {
			for _, r := range required {
				if s, ok := r.(string); ok {
					fn.RequiredInputs = append(fn.RequiredInputs, s)
				}
			}
		}
	}

	if outputs, ok := funcMap["outputs"].(map[string]interface{}); ok {
		if props, ok := outputs["properties"].(map[string]interface{}); ok {
			for propName, propData := range props {
				fn.Outputs[propName] = l.parsePropertySchema(propName, propData)
			}
		}
	}

	return fn
}

// parsePropertySchema parses a property schema from JSON.
func (l *PackageLoader) parsePropertySchema(name string, data interface{}) *PropertySchema {
	propMap, ok := data.(map[string]interface{})
	if !ok {
		return &PropertySchema{Name: name, Type: PropertyType{Kind: PropertyKindAny}}
	}

	prop := &PropertySchema{Name: name}

	if desc, ok := propMap["description"].(string); ok {
		prop.Description = desc
	}

	if secret, ok := propMap["secret"].(bool); ok {
		prop.Secret = secret
	}

	if def, ok := propMap["default"]; ok {
		prop.Default = def
	}

	prop.Type = l.parsePropertyType(propMap)

	return prop
}

// parsePropertyType parses a property type from JSON.
func (l *PackageLoader) parsePropertyType(propMap map[string]interface{}) PropertyType {
	if ref, ok := propMap["$ref"].(string); ok {
		return PropertyType{Kind: PropertyKindRef, Ref: ref}
	}

	typeStr, _ := propMap["type"].(string)

	switch typeStr {
	case "string":
		return PropertyType{Kind: PropertyKindString}
	case "integer", "number":
		return PropertyType{Kind: PropertyKindNumber}
	case "boolean":
		return PropertyType{Kind: PropertyKindBoolean}
	case "array":
		pt := PropertyType{Kind: PropertyKindArray}
		if items, ok := propMap["items"].(map[string]interface{}); ok {
			elemType := l.parsePropertyType(items)
			pt.ElementType = &elemType
		}
		return pt
	case "object":
		pt := PropertyType{Kind: PropertyKindObject}
		if addProps, ok := propMap["additionalProperties"].(map[string]interface{}); ok {
			pt.Kind = PropertyKindMap
			elemType := l.parsePropertyType(addProps)
			pt.ElementType = &elemType
		}
		if props, ok := propMap["properties"].(map[string]interface{}); ok {
			pt.ObjectProperties = make(map[string]*PropertySchema)
			for propName, pdata := range props {
				pt.ObjectProperties[propName] = l.parsePropertySchema(propName, pdata)
			}
		}
		return pt
	default:
		return PropertyType{Kind: PropertyKindAny}
	}
}

// ResolveResourceType resolves a resource type to a Pulumi type token.
// It handles underscore-separated names with the following priority:
//   - 3+ parts (e.g., aws_ec2_instance): Try Pulumi synthetic first (aws:ec2:Instance),
//     fall back to TF mapping if not found in schema
//   - 2 parts (e.g., aws_instance): TF mapping lookup only
//
// This allows both Pulumi-style (aws_ec2_instance) and TF-style (aws_instance) naming.
func (l *PackageLoader) ResolveResourceType(resourceType string) (string, error) {
	parts := strings.Split(resourceType, "_")
	if len(parts) < 2 {
		return "", fmt.Errorf("invalid resource type: %s (expected provider_type or provider_module_type format)", resourceType)
	}

	providerName := parts[0]

	// For 3+ parts, try Pulumi synthetic first
	if len(parts) >= 3 {
		// Try to resolve as Pulumi synthetic: provider_module_type -> provider:module:Type
		pulumiToken, err := l.tryPulumiSynthetic(providerName, parts[1:])
		if err == nil {
			return pulumiToken, nil
		}
		// Fall through to TF mapping
	}

	// Try TF mapping (for 2-part names, or as fallback for 3+ part names)
	token, err := l.tryTerraformMapping(providerName, resourceType)
	if err == nil {
		return token, nil
	}

	// For 2-part names, fall back to native Pulumi with "index" module
	// e.g., random_pet -> random:index/randomPet:RandomPet
	if len(parts) == 2 {
		return l.tryNativePulumiType(providerName, parts[1])
	}

	return "", err
}

// tryNativePulumiType attempts to resolve a 2-part type using native Pulumi naming.
// It converts provider_type to provider:index/type:Type format.
func (l *PackageLoader) tryNativePulumiType(providerName, typeName string) (string, error) {
	// Build camelCase for the module path: randomPet
	camelCase := typeName
	if len(camelCase) > 0 {
		camelCase = strings.ToLower(camelCase[:1]) + camelCase[1:]
	}

	// Build PascalCase for the type name: RandomPet
	pascalCase := typeName
	if len(pascalCase) > 0 {
		pascalCase = strings.ToUpper(pascalCase[:1]) + pascalCase[1:]
	}

	// Construct the token: provider:index/camelCase:PascalCase
	pulumiToken := fmt.Sprintf("%s:index/%s:%s", providerName, camelCase, pascalCase)

	if l.testMode {
		return pulumiToken, nil
	}

	// Use cached token set for fast validation
	if l.hasResourceToken(providerName, pulumiToken) {
		return pulumiToken, nil
	}

	return "", fmt.Errorf("resource type %s not found in provider %s (tried %s)", typeName, providerName, pulumiToken)
}

// hasResourceToken checks if a resource token exists, using cached token sets.
func (l *PackageLoader) hasResourceToken(providerName, token string) bool {
	// Try to get provider info (loads from cache if available)
	info, _ := l.LoadProviderInfo(providerName, nil)
	if info != nil && len(info.ResourceTokens) > 0 {
		return info.ResourceTokens[token]
	}

	// Fall back to loading full schema
	schema, err := l.LoadPackage(providerName, nil)
	if err != nil {
		return false
	}
	_, ok := schema.Resources[token]
	return ok
}

// hasFunctionToken checks if a function token exists, using cached token sets.
func (l *PackageLoader) hasFunctionToken(providerName, token string) bool {
	// Try to get provider info (loads from cache if available)
	info, _ := l.LoadProviderInfo(providerName, nil)
	if info != nil && len(info.FunctionTokens) > 0 {
		return info.FunctionTokens[token]
	}

	// Fall back to loading full schema
	schema, err := l.LoadPackage(providerName, nil)
	if err != nil {
		return false
	}
	_, ok := schema.Functions[token]
	return ok
}

// tryPulumiSynthetic attempts to resolve a resource type using Pulumi synthetic naming.
// It converts provider_module_type to provider:module:Type and validates against the schema.
func (l *PackageLoader) tryPulumiSynthetic(providerName string, remaining []string) (string, error) {
	if len(remaining) < 2 {
		return "", fmt.Errorf("need at least module and type for Pulumi synthetic")
	}

	// For provider_module_type format:
	// - First remaining part is the module
	// - Rest is the type name (joined and capitalized)
	moduleName := remaining[0]
	typeParts := remaining[1:]

	// Build the type name by capitalizing each part
	var typeName string
	for _, part := range typeParts {
		if len(part) > 0 {
			typeName += strings.ToUpper(part[:1]) + part[1:]
		}
	}

	// Construct the Pulumi token: provider:module:Type
	pulumiToken := fmt.Sprintf("%s:%s:%s", providerName, moduleName, typeName)

	// Validate against the schema
	if l.testMode {
		// In test mode, skip schema validation
		return pulumiToken, nil
	}

	// Use cached token sets for fast validation
	if l.hasResourceToken(providerName, pulumiToken) {
		return pulumiToken, nil
	}

	// Try alternate token formats that Pulumi uses
	// e.g., aws:ec2:Instance might be stored as aws:ec2/instance:Instance
	altToken := fmt.Sprintf("%s:%s/%s:%s", providerName, moduleName, strings.ToLower(typeName), typeName)
	if l.hasResourceToken(providerName, altToken) {
		return altToken, nil
	}

	return "", fmt.Errorf("resource type %s not found in %s schema", pulumiToken, providerName)
}

// tryTerraformMapping attempts to resolve a resource type using the TF bridged provider mapping.
func (l *PackageLoader) tryTerraformMapping(providerName, resourceType string) (string, error) {
	// In test mode, return a synthetic token based on the TF name
	if l.testMode {
		// Convert aws_instance to aws:index:Instance for testing
		parts := strings.SplitN(resourceType, "_", 2)
		if len(parts) >= 2 {
			typeName := parts[1]
			// Capitalize the type name
			if len(typeName) > 0 {
				typeName = strings.ToUpper(typeName[:1]) + typeName[1:]
			}
			return fmt.Sprintf("%s:index:%s", providerName, typeName), nil
		}
	}

	// Load the provider info to get the TF->Pulumi mapping
	info, err := l.LoadProviderInfo(providerName, nil)
	if err != nil {
		return "", fmt.Errorf("loading provider info for %s: %w", providerName, err)
	}

	if !info.IsBridged {
		return "", fmt.Errorf("provider %s is not a Terraform-bridged provider; use 3-part naming (e.g., %s_module_type)", providerName, providerName)
	}

	// Look up the Terraform type in the mapping
	resInfo, ok := info.Resources[resourceType]
	if !ok {
		return "", fmt.Errorf("unknown resource type: %s (not found in provider %s)", resourceType, providerName)
	}

	return string(resInfo.Tok), nil
}

// ResolveDataSourceType resolves a data source type to a Pulumi function token.
// It handles underscore-separated names with the following priority:
//   - 3+ parts (e.g., aws_ec2_get_ami): Try Pulumi synthetic first (aws:ec2:getAmi),
//     fall back to TF mapping if not found in schema
//   - 2 parts (e.g., aws_ami): TF mapping lookup only
func (l *PackageLoader) ResolveDataSourceType(dataSourceType string) (string, error) {
	parts := strings.Split(dataSourceType, "_")
	if len(parts) < 2 {
		return "", fmt.Errorf("invalid data source type: %s (expected provider_type or provider_module_type format)", dataSourceType)
	}

	providerName := parts[0]

	// For 3+ parts, try Pulumi synthetic first
	if len(parts) >= 3 {
		pulumiToken, err := l.tryPulumiSyntheticFunction(providerName, parts[1:])
		if err == nil {
			return pulumiToken, nil
		}
		// Fall through to TF mapping
	}

	// Try TF mapping (for 2-part names, or as fallback for 3+ part names)
	return l.tryTerraformDataSourceMapping(providerName, dataSourceType)
}

// tryPulumiSyntheticFunction attempts to resolve a data source using Pulumi synthetic naming.
// It converts provider_module_get_type to provider:module:getType and validates against the schema.
func (l *PackageLoader) tryPulumiSyntheticFunction(providerName string, remaining []string) (string, error) {
	if len(remaining) < 2 {
		return "", fmt.Errorf("need at least module and function for Pulumi synthetic")
	}

	// For provider_module_type format:
	// - First remaining part is the module
	// - Rest is the function name (typically getXxx format)
	moduleName := remaining[0]
	funcParts := remaining[1:]

	// Build the function name - for data sources, Pulumi uses getXxx format
	// Convert: get_ami -> getAmi, or ami -> getAmi
	var funcName string
	for i, part := range funcParts {
		if i == 0 {
			// First part: if it's "get", keep lowercase; otherwise prefix with "get" and capitalize
			if part == "get" {
				funcName = "get"
			} else {
				funcName = "get" + strings.ToUpper(part[:1]) + part[1:]
			}
		} else {
			// Subsequent parts: capitalize first letter
			if len(part) > 0 {
				funcName += strings.ToUpper(part[:1]) + part[1:]
			}
		}
	}

	// Construct the Pulumi token: provider:module:getType
	pulumiToken := fmt.Sprintf("%s:%s:%s", providerName, moduleName, funcName)

	// Validate against the schema
	if l.testMode {
		return pulumiToken, nil
	}

	// Use cached token sets for fast validation
	if l.hasFunctionToken(providerName, pulumiToken) {
		return pulumiToken, nil
	}

	// Try alternate format
	altToken := fmt.Sprintf("%s:%s/%s:%s", providerName, moduleName, funcName, funcName)
	if l.hasFunctionToken(providerName, altToken) {
		return altToken, nil
	}

	return "", fmt.Errorf("data source %s not found in %s schema", pulumiToken, providerName)
}

// tryTerraformDataSourceMapping attempts to resolve a data source using the TF bridged provider mapping.
func (l *PackageLoader) tryTerraformDataSourceMapping(providerName, dataSourceType string) (string, error) {
	if l.testMode {
		// Convert aws_ami to aws:index:getAmi for testing
		parts := strings.SplitN(dataSourceType, "_", 2)
		if len(parts) >= 2 {
			funcName := "get" + strings.ToUpper(parts[1][:1]) + parts[1][1:]
			return fmt.Sprintf("%s:index:%s", providerName, funcName), nil
		}
	}

	info, err := l.LoadProviderInfo(providerName, nil)
	if err != nil {
		return "", fmt.Errorf("loading provider info for %s: %w", providerName, err)
	}

	if !info.IsBridged {
		return "", fmt.Errorf("provider %s is not a Terraform-bridged provider; use 3-part naming (e.g., %s_module_get_type)", providerName, providerName)
	}

	dsInfo, ok := info.DataSources[dataSourceType]
	if !ok {
		return "", fmt.Errorf("unknown data source type: %s (not found in provider %s)", dataSourceType, providerName)
	}

	return string(dsInfo.Tok), nil
}

// GetResourceSchema returns the schema for a resource type.
func (l *PackageLoader) GetResourceSchema(typeToken string) (*ResourceSchema, error) {
	parts := strings.Split(typeToken, ":")
	if len(parts) < 1 {
		return nil, fmt.Errorf("invalid type token: %s", typeToken)
	}

	pkgName := parts[0]
	schema, err := l.LoadPackage(pkgName, nil)
	if err != nil {
		return nil, err
	}

	resSchema, ok := schema.Resources[typeToken]
	if !ok {
		return nil, fmt.Errorf("resource type not found: %s", typeToken)
	}

	return resSchema, nil
}

// GetFunctionSchema returns the schema for a function (data source) type.
func (l *PackageLoader) GetFunctionSchema(funcToken string) (*FunctionSchema, error) {
	parts := strings.Split(funcToken, ":")
	if len(parts) < 1 {
		return nil, fmt.Errorf("invalid function token: %s", funcToken)
	}

	pkgName := parts[0]
	schema, err := l.LoadPackage(pkgName, nil)
	if err != nil {
		return nil, err
	}

	funcSchema, ok := schema.Functions[funcToken]
	if !ok {
		return nil, fmt.Errorf("function type not found: %s", funcToken)
	}

	return funcSchema, nil
}
