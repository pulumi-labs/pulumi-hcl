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
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/pulumi-labs/pulumi-hcl/pkg/hcl/ast"
	"github.com/pulumi-labs/pulumi-hcl/pkg/hcl/parser"
	"github.com/pulumi/pulumi/sdk/v3/go/common/util/contract"
)

// Loader loads and parses module configurations.
type Loader struct {
	// parser is the HCL parser instance
	parser *parser.Parser

	// cache stores loaded module configs by their resolved path
	cache map[string]*LoadedModule

	// callStack tracks the module call path for cycle detection
	callStack []string

	// cacheDir is where downloaded modules are stored (~/.pulumi/modules/)
	cacheDir string
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
	cacheDir := defaultCacheDir()
	return &Loader{
		parser:   parser.NewParser(),
		cache:    make(map[string]*LoadedModule),
		cacheDir: cacheDir,
	}
}

// defaultCacheDir returns the default module cache directory (~/.pulumi/modules/).
func defaultCacheDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = os.TempDir()
	}
	return filepath.Join(home, ".pulumi", "modules")
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
	if slices.Contains(l.callStack, resolvedPath) {
		return nil, fmt.Errorf("module cycle detected: %s", strings.Join(append(l.callStack, resolvedPath), " -> "))
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
// Supports local paths, Git URLs, and Terraform Registry addresses.
func (l *Loader) resolveSource(source string, callerDir string) (string, error) {
	// Parse the source to extract subdir if present (e.g., "git::url//subdir")
	source, subdir := splitSourceSubdir(source)

	var resolvedPath string
	var err error

	switch {
	// Local paths start with ./ or ../
	case strings.HasPrefix(source, "./") || strings.HasPrefix(source, "../"):
		resolvedPath, err = l.resolveLocalSource(source, callerDir)

	// Absolute paths
	case filepath.IsAbs(source):
		resolvedPath, err = l.resolveAbsoluteSource(source)

	// Git format: git::url
	case strings.HasPrefix(source, "git::"):
		resolvedPath, err = l.resolveGitSource(source)

	// GitHub shorthand: github.com/org/repo
	case strings.HasPrefix(source, "github.com/"):
		resolvedPath, err = l.resolveGitSource("git::https://" + source)

	// BitBucket shorthand: bitbucket.org/org/repo
	case strings.HasPrefix(source, "bitbucket.org/"):
		resolvedPath, err = l.resolveGitSource("git::https://" + source)

	// HTTP/S archives
	case strings.HasPrefix(source, "http://") || strings.HasPrefix(source, "https://"):
		resolvedPath, err = l.resolveHTTPSource(source)

	// Registry format: namespace/name/provider (e.g., hashicorp/consul/aws)
	case isRegistrySource(source):
		resolvedPath, err = l.resolveRegistrySource(source)

	// Unknown format - treat as local path without prefix
	default:
		resolvedPath, err = l.resolveLocalSource(source, callerDir)
	}

	if err != nil {
		return "", err
	}

	// Apply subdir if specified
	if subdir != "" {
		resolvedPath = filepath.Join(resolvedPath, subdir)
		if info, statErr := os.Stat(resolvedPath); statErr != nil || !info.IsDir() {
			return "", fmt.Errorf("subdir %q does not exist in module", subdir)
		}
	}

	return resolvedPath, nil
}

// splitSourceSubdir splits a source into the base source and optional subdir.
// e.g., "git::https://example.com/repo.git//modules/foo" -> ("git::https://example.com/repo.git", "modules/foo")
func splitSourceSubdir(source string) (string, string) {
	idx := strings.Index(source, "//")
	if idx == -1 {
		return source, ""
	}
	// Skip protocol separators (http://, https://, git::ssh://)
	if idx > 0 && source[idx-1] == ':' {
		// Look for the next //
		nextIdx := strings.Index(source[idx+2:], "//")
		if nextIdx == -1 {
			return source, ""
		}
		idx = idx + 2 + nextIdx
	}
	return source[:idx], source[idx+2:]
}

// resolveLocalSource resolves a local file path.
func (l *Loader) resolveLocalSource(source string, callerDir string) (string, error) {
	resolved := filepath.Join(callerDir, source)
	absPath, err := filepath.Abs(resolved)
	if err != nil {
		return "", fmt.Errorf("resolving path: %w", err)
	}

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

// resolveAbsoluteSource resolves an absolute file path.
func (l *Loader) resolveAbsoluteSource(source string) (string, error) {
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

// resolveGitSource clones a Git repository and returns the local path.
// Format: git::https://example.com/repo.git?ref=v1.0.0
func (l *Loader) resolveGitSource(source string) (string, error) {
	// Strip git:: prefix
	gitURL := strings.TrimPrefix(source, "git::")

	// Parse ref from query string (e.g., ?ref=v1.0.0)
	ref := ""
	if idx := strings.Index(gitURL, "?"); idx != -1 {
		query := gitURL[idx+1:]
		gitURL = gitURL[:idx]
		for _, param := range strings.Split(query, "&") {
			if strings.HasPrefix(param, "ref=") {
				ref = strings.TrimPrefix(param, "ref=")
			}
		}
	}

	// Create a stable cache key based on URL and ref
	cacheKey := hashSource(gitURL + "@" + ref)
	cacheDir := filepath.Join(l.cacheDir, "git", cacheKey)

	// Check if already cached
	if info, err := os.Stat(cacheDir); err == nil && info.IsDir() {
		return cacheDir, nil
	}

	// Ensure cache directory exists
	if err := os.MkdirAll(filepath.Dir(cacheDir), 0755); err != nil {
		return "", fmt.Errorf("creating cache directory: %w", err)
	}

	// Clone the repository
	args := []string{"clone", "--depth=1"}
	if ref != "" {
		args = append(args, "--branch", ref)
	}
	args = append(args, gitURL, cacheDir)

	cmd := exec.Command("git", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git clone failed: %s: %w", string(output), err)
	}

	return cacheDir, nil
}

// resolveHTTPSource downloads an archive from HTTP/HTTPS and extracts it.
func (l *Loader) resolveHTTPSource(source string) (string, error) {
	// Create a stable cache key
	cacheKey := hashSource(source)
	cacheDir := filepath.Join(l.cacheDir, "http", cacheKey)

	// Check if already cached
	if info, err := os.Stat(cacheDir); err == nil && info.IsDir() {
		return cacheDir, nil
	}

	// Ensure cache directory exists
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return "", fmt.Errorf("creating cache directory: %w", err)
	}

	// Download the archive
	resp, err := http.Get(source)
	if err != nil {
		return "", fmt.Errorf("downloading module: %w", err)
	}
	defer contract.IgnoreClose(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("downloading module: HTTP %d", resp.StatusCode)
	}

	// Create temporary file for the archive
	tmpFile, err := os.CreateTemp("", "module-*.zip")
	if err != nil {
		return "", fmt.Errorf("creating temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer func() { contract.IgnoreError(os.Remove(tmpPath)) }()

	if _, err := io.Copy(tmpFile, resp.Body); err != nil {
		contract.IgnoreError(tmpFile.Close())
		return "", fmt.Errorf("downloading module: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		return "", fmt.Errorf("closing temp file: %w", err)
	}

	// Extract the archive
	if err := extractZip(tmpPath, cacheDir); err != nil {
		contract.IgnoreError(os.RemoveAll(cacheDir))
		return "", fmt.Errorf("extracting module: %w", err)
	}

	return cacheDir, nil
}

// resolveRegistrySource downloads a module from the Terraform Registry.
// Format: namespace/name/provider (e.g., hashicorp/consul/aws)
// Optional version: namespace/name/provider?version=1.0.0
func (l *Loader) resolveRegistrySource(source string) (string, error) {
	// Parse version from query string
	version := ""
	baseSource := source
	if idx := strings.Index(source, "?"); idx != -1 {
		query := source[idx+1:]
		baseSource = source[:idx]
		for _, param := range strings.Split(query, "&") {
			if strings.HasPrefix(param, "version=") {
				version = strings.TrimPrefix(param, "version=")
			}
		}
	}

	parts := strings.Split(baseSource, "/")
	if len(parts) != 3 {
		return "", fmt.Errorf("invalid registry source format: %s", source)
	}
	namespace, name, provider := parts[0], parts[1], parts[2]

	// Create cache key
	cacheKey := fmt.Sprintf("%s-%s-%s-%s", namespace, name, provider, version)
	cacheDir := filepath.Join(l.cacheDir, "registry", cacheKey)

	// Check if already cached
	if info, err := os.Stat(cacheDir); err == nil && info.IsDir() {
		return cacheDir, nil
	}

	// Query the registry API for the download URL
	downloadURL, err := l.getRegistryDownloadURL(namespace, name, provider, version)
	if err != nil {
		return "", err
	}

	// Ensure cache directory exists
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return "", fmt.Errorf("creating cache directory: %w", err)
	}

	// Download and extract the module
	resp, err := http.Get(downloadURL)
	if err != nil {
		return "", fmt.Errorf("downloading module from registry: %w", err)
	}
	defer contract.IgnoreClose(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("downloading module from registry: HTTP %d", resp.StatusCode)
	}

	// Create temporary file for the archive
	tmpFile, err := os.CreateTemp("", "module-*.tar.gz")
	if err != nil {
		return "", fmt.Errorf("creating temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer func() { contract.IgnoreError(os.Remove(tmpPath)) }()

	if _, err := io.Copy(tmpFile, resp.Body); err != nil {
		contract.IgnoreError(tmpFile.Close())
		return "", fmt.Errorf("downloading module: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		return "", fmt.Errorf("closing temp file: %w", err)
	}

	// Extract the archive (registry uses tar.gz)
	if err := extractTarGz(tmpPath, cacheDir); err != nil {
		contract.IgnoreError(os.RemoveAll(cacheDir))
		return "", fmt.Errorf("extracting module: %w", err)
	}

	return cacheDir, nil
}

// registryModuleVersion represents a version from the registry API.
type registryModuleVersion struct {
	Version string `json:"version"`
}

// registryModuleVersions represents the versions response from the registry API.
type registryModuleVersions struct {
	Modules []struct {
		Versions []registryModuleVersion `json:"versions"`
	} `json:"modules"`
}

// getRegistryDownloadURL queries the Terraform Registry API to get the download URL.
func (l *Loader) getRegistryDownloadURL(namespace, name, provider, version string) (string, error) {
	baseURL := "https://registry.terraform.io/v1/modules"

	// If no version specified, get the latest
	if version == "" {
		versionsURL := fmt.Sprintf("%s/%s/%s/%s/versions", baseURL, namespace, name, provider)
		resp, err := http.Get(versionsURL)
		if err != nil {
			return "", fmt.Errorf("querying registry versions: %w", err)
		}
		defer contract.IgnoreClose(resp.Body)

		if resp.StatusCode == http.StatusNotFound {
			return "", fmt.Errorf("module %s/%s/%s not found in registry", namespace, name, provider)
		}
		if resp.StatusCode != http.StatusOK {
			return "", fmt.Errorf("querying registry versions: HTTP %d", resp.StatusCode)
		}

		var versions registryModuleVersions
		if err := json.NewDecoder(resp.Body).Decode(&versions); err != nil {
			return "", fmt.Errorf("parsing registry response: %w", err)
		}

		if len(versions.Modules) == 0 || len(versions.Modules[0].Versions) == 0 {
			return "", fmt.Errorf("no versions available for module %s/%s/%s", namespace, name, provider)
		}

		// Use the first version (latest)
		version = versions.Modules[0].Versions[0].Version
	}

	// Get the download URL for the specific version
	downloadURL := fmt.Sprintf("%s/%s/%s/%s/%s/download", baseURL, namespace, name, provider, version)
	resp, err := http.Get(downloadURL)
	if err != nil {
		return "", fmt.Errorf("getting download URL: %w", err)
	}
	defer contract.IgnoreClose(resp.Body)

	// The registry returns a 204 with X-Terraform-Get header containing the actual URL
	if resp.StatusCode == http.StatusNoContent {
		actualURL := resp.Header.Get("X-Terraform-Get")
		if actualURL == "" {
			return "", fmt.Errorf("registry did not return download URL")
		}
		return actualURL, nil
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("getting download URL: HTTP %d", resp.StatusCode)
	}

	// Some registries return the URL in the response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("reading download URL: %w", err)
	}

	return string(body), nil
}

// isRegistrySource checks if a source looks like a Terraform Registry address.
// Format: namespace/name/provider (e.g., hashicorp/consul/aws)
func isRegistrySource(source string) bool {
	// Strip query string for validation
	if idx := strings.Index(source, "?"); idx != -1 {
		source = source[:idx]
	}

	parts := strings.Split(source, "/")
	if len(parts) != 3 {
		return false
	}
	// All parts should be non-empty and not look like URLs
	for _, part := range parts {
		if part == "" || strings.Contains(part, ":") {
			return false
		}
	}
	// First part shouldn't look like a domain (no dots unless it's github.com etc.)
	if strings.Contains(parts[0], ".") {
		return false
	}
	return true
}

// hashSource creates a stable hash for a source URL to use as a cache key.
func hashSource(source string) string {
	h := sha256.Sum256([]byte(source))
	return hex.EncodeToString(h[:8])
}

// extractZip extracts a zip archive to the destination directory.
func extractZip(src, dest string) error {
	r, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer contract.IgnoreClose(r)

	for _, f := range r.File {
		fpath := filepath.Join(dest, f.Name)

		// Security check: ensure path is within dest
		if !strings.HasPrefix(filepath.Clean(fpath), filepath.Clean(dest)+string(os.PathSeparator)) {
			return fmt.Errorf("invalid file path in archive: %s", f.Name)
		}

		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(fpath, os.ModePerm); err != nil {
				return err
			}
			continue
		}

		if err := os.MkdirAll(filepath.Dir(fpath), os.ModePerm); err != nil {
			return err
		}

		outFile, err := os.OpenFile(fpath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
		if err != nil {
			return err
		}

		rc, err := f.Open()
		if err != nil {
			contract.IgnoreError(outFile.Close())
			return err
		}

		_, err = io.Copy(outFile, rc)
		closeErr := outFile.Close()
		contract.IgnoreError(rc.Close())
		if err != nil {
			return err
		}
		if closeErr != nil {
			return closeErr
		}
	}

	return nil
}

// extractTarGz extracts a tar.gz archive to the destination directory.
func extractTarGz(src, dest string) error {
	// Use tar command for simplicity and security
	cmd := exec.Command("tar", "-xzf", src, "-C", dest)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("tar extraction failed: %s: %w", string(output), err)
	}
	return nil
}

// GetCallStack returns the current module call stack (for debugging/error messages).
func (l *Loader) GetCallStack() []string {
	return append([]string{}, l.callStack...)
}
