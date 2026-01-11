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

// Package modules handles loading and resolving Terraform module sources.
package modules

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/pulumi/pulumi-language-hcl/pkg/hcl/ast"
	"github.com/pulumi/pulumi-language-hcl/pkg/hcl/parser"
)

// Loader loads and parses module configurations.
type Loader struct {
	// parser is the HCL parser instance
	parser *parser.Parser

	// cache stores loaded module configs by their resolved path
	cache map[string]*LoadedModule

	// callStack tracks the module call path for cycle detection
	callStack []string
}

// LoadedModule represents a loaded and parsed module.
type LoadedModule struct {
	// Config is the parsed module configuration.
	Config *ast.Config

	// SourcePath is the resolved absolute path to the module.
	SourcePath string
}

// NewLoader creates a new module loader.
func NewLoader() *Loader {
	return &Loader{
		parser: parser.NewParser(),
		cache:  make(map[string]*LoadedModule),
	}
}

// LoadModule loads a module from the given source, relative to the caller's directory.
// It returns the loaded module configuration and resolved path.
func (l *Loader) LoadModule(source string, callerDir string) (*LoadedModule, error) {
	// Resolve the source to an absolute path
	resolvedPath, err := l.resolveSource(source, callerDir)
	if err != nil {
		return nil, fmt.Errorf("resolving module source %q: %w", source, err)
	}

	// Check for cycles
	for _, path := range l.callStack {
		if path == resolvedPath {
			return nil, fmt.Errorf("module cycle detected: %s", strings.Join(append(l.callStack, resolvedPath), " -> "))
		}
	}

	// Check cache
	if cached, ok := l.cache[resolvedPath]; ok {
		return cached, nil
	}

	// Add to call stack for cycle detection
	l.callStack = append(l.callStack, resolvedPath)
	defer func() {
		l.callStack = l.callStack[:len(l.callStack)-1]
	}()

	// Parse the module
	config, diags := l.parser.ParseDirectory(resolvedPath)
	if diags.HasErrors() {
		return nil, fmt.Errorf("parsing module: %s", diags.Error())
	}

	module := &LoadedModule{
		Config:     config,
		SourcePath: resolvedPath,
	}

	// Cache the result
	l.cache[resolvedPath] = module

	return module, nil
}

// resolveSource resolves a module source to an absolute path.
// Currently supports local paths only. Remote sources (registry, git) are not yet implemented.
func (l *Loader) resolveSource(source string, callerDir string) (string, error) {
	// Local paths start with ./ or ../
	if strings.HasPrefix(source, "./") || strings.HasPrefix(source, "../") {
		// Resolve relative to caller's directory
		resolved := filepath.Join(callerDir, source)
		absPath, err := filepath.Abs(resolved)
		if err != nil {
			return "", fmt.Errorf("resolving path: %w", err)
		}

		// Verify the path exists and is a directory
		info, err := os.Stat(absPath)
		if err != nil {
			if os.IsNotExist(err) {
				return "", fmt.Errorf("module directory does not exist: %s", absPath)
			}
			return "", fmt.Errorf("accessing module directory: %w", err)
		}
		if !info.IsDir() {
			return "", fmt.Errorf("module source is not a directory: %s", absPath)
		}

		return absPath, nil
	}

	// Absolute paths
	if filepath.IsAbs(source) {
		info, err := os.Stat(source)
		if err != nil {
			if os.IsNotExist(err) {
				return "", fmt.Errorf("module directory does not exist: %s", source)
			}
			return "", fmt.Errorf("accessing module directory: %w", err)
		}
		if !info.IsDir() {
			return "", fmt.Errorf("module source is not a directory: %s", source)
		}
		return source, nil
	}

	// Registry format: namespace/name/provider (e.g., hashicorp/consul/aws)
	if isRegistrySource(source) {
		return "", fmt.Errorf("registry module sources not yet supported: %s", source)
	}

	// Git format: git::url (e.g., git::https://example.com/repo.git)
	if strings.HasPrefix(source, "git::") {
		return "", fmt.Errorf("git module sources not yet supported: %s", source)
	}

	// HTTP/S format
	if strings.HasPrefix(source, "http://") || strings.HasPrefix(source, "https://") {
		return "", fmt.Errorf("HTTP module sources not yet supported: %s", source)
	}

	// Unknown format - treat as local path without prefix
	resolved := filepath.Join(callerDir, source)
	absPath, err := filepath.Abs(resolved)
	if err != nil {
		return "", fmt.Errorf("resolving path: %w", err)
	}

	info, err := os.Stat(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("module directory does not exist: %s (from source %q)", absPath, source)
		}
		return "", fmt.Errorf("accessing module directory: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("module source is not a directory: %s", absPath)
	}

	return absPath, nil
}

// isRegistrySource checks if a source looks like a Terraform Registry address.
// Format: namespace/name/provider (e.g., hashicorp/consul/aws)
func isRegistrySource(source string) bool {
	parts := strings.Split(source, "/")
	if len(parts) != 3 {
		return false
	}
	// All parts should be non-empty and not contain special characters
	for _, part := range parts {
		if part == "" || strings.Contains(part, ".") || strings.Contains(part, ":") {
			return false
		}
	}
	return true
}

// GetCallStack returns the current module call stack (for debugging/error messages).
func (l *Loader) GetCallStack() []string {
	return append([]string{}, l.callStack...)
}
