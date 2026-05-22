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

// Package modules loads and parses Terraform-compatible HCL module
// configurations. Remote source resolution (git, http, registry, etc.) is
// delegated to the vendored opentofu getmodules package which wraps
// hashicorp/go-getter.
package modules

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"github.com/pulumi-labs/pulumi-hcl/pkg/hcl/ast"
	"github.com/pulumi-labs/pulumi-hcl/pkg/hcl/parser"
	"github.com/pulumi-labs/pulumi-hcl/vendored/getmodules"
	"github.com/pulumi/pulumi/sdk/v3/go/common/util/contract"
)

// Loader loads and parses module configurations.
type Loader struct {
	mu sync.Mutex

	parser    *parser.Parser
	cache     map[string]*LoadedModule
	callStack []string
	cacheDir  string

	fetcher *getmodules.PackageFetcher
}

// LoadedModule represents a loaded and parsed module.
type LoadedModule struct {
	Config     *ast.Config
	SourcePath string
}

// NewLoader creates a new module loader.
func NewLoader() *Loader {
	return &Loader{
		parser:   parser.NewParser(),
		cache:    make(map[string]*LoadedModule),
		cacheDir: defaultCacheDir(),
		fetcher:  getmodules.NewPackageFetcher(context.Background(), nil),
	}
}

func defaultCacheDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = os.TempDir()
	}
	return filepath.Join(home, ".pulumi", "modules")
}

// LoadModule loads a module from the given source, relative to the caller's directory.
func (l *Loader) LoadModule(source string, callerDir string) (*LoadedModule, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	resolvedPath, err := l.resolveSource(source, callerDir)
	if err != nil {
		return nil, fmt.Errorf("resolving module source %q: %w", source, err)
	}

	if slices.Contains(l.callStack, resolvedPath) {
		return nil, fmt.Errorf("module cycle detected: %s",
			strings.Join(append(l.callStack, resolvedPath), " -> "))
	}

	if cached, ok := l.cache[resolvedPath]; ok {
		return cached, nil
	}

	l.callStack = append(l.callStack, resolvedPath)
	defer func() {
		l.callStack = l.callStack[:len(l.callStack)-1]
	}()

	config, diags := l.parser.ParseDirectory(resolvedPath)
	if diags.HasErrors() {
		return nil, fmt.Errorf("parsing module: %s", diags.Error())
	}

	module := &LoadedModule{Config: config, SourcePath: resolvedPath}
	l.cache[resolvedPath] = module
	return module, nil
}

// resolveSource resolves a module source to an absolute path on disk.
// Local paths resolve in-place; remote sources are fetched via go-getter
// (through the vendored getmodules.PackageFetcher) into ~/.pulumi/modules/.
func (l *Loader) resolveSource(source string, callerDir string) (string, error) {
	source, subdir := splitSourceSubdir(source)

	var resolvedPath string
	var err error
	switch {
	case strings.HasPrefix(source, "./") || strings.HasPrefix(source, "../"):
		resolvedPath, err = l.resolveLocalSource(source, callerDir)
	case filepath.IsAbs(source):
		resolvedPath, err = l.resolveAbsoluteSource(source)
	case isRegistrySource(source):
		resolvedPath, err = l.resolveRegistrySource(source)
	default:
		// Anything else (git::, github.com/..., bitbucket.org/..., http(s)://,
		// s3::, gcs::, hg::, etc.) goes through the package fetcher, which
		// runs the upstream opentofu detectors to normalize the address.
		resolvedPath, err = l.fetchRemote(source, "remote")
	}
	if err != nil {
		return "", err
	}

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
	if idx > 0 && source[idx-1] == ':' {
		nextIdx := strings.Index(source[idx+2:], "//")
		if nextIdx == -1 {
			return source, ""
		}
		idx = idx + 2 + nextIdx
	}
	return source[:idx], source[idx+2:]
}

func (l *Loader) resolveLocalSource(source string, callerDir string) (string, error) {
	resolved := filepath.Join(callerDir, source)
	absPath, err := filepath.Abs(resolved)
	if err != nil {
		return "", fmt.Errorf("resolving path: %w", err)
	}
	return statDir(absPath)
}

func (l *Loader) resolveAbsoluteSource(source string) (string, error) {
	return statDir(source)
}

func statDir(p string) (string, error) {
	info, err := os.Stat(p)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("module directory does not exist: %s", p)
		}
		return "", fmt.Errorf("accessing module directory: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("module source is not a directory: %s", p)
	}
	return p, nil
}

// fetchRemote runs the vendored getmodules detectors on `source` and then
// downloads it (if not already cached) into a stable path under cacheDir/kind/.
// The returned path is the populated cache directory.
func (l *Loader) fetchRemote(source, kind string) (string, error) {
	pkgAddr, detectedSubdir, err := getmodules.NormalizePackageAddress(source)
	if err != nil {
		return "", fmt.Errorf("normalizing module source %q: %w", source, err)
	}
	// We strip subdir before calling fetchRemote; the detector should not
	// reintroduce one. If it does, the caller (resolveSource) lost it.
	if detectedSubdir != "" {
		return "", fmt.Errorf("module source %q resolved to package %q with unexpected subdir %q",
			source, pkgAddr, detectedSubdir)
	}

	cacheDir := filepath.Join(l.cacheDir, kind, hashSource(pkgAddr))
	if info, statErr := os.Stat(cacheDir); statErr == nil && info.IsDir() {
		if dirHasFiles(cacheDir) {
			return cacheDir, nil
		}
		// Empty dir from a prior failed fetch — wipe and retry. PackageFetcher
		// won't reuse an empty cache and would error if the directory exists.
		if rmErr := os.RemoveAll(cacheDir); rmErr != nil {
			return "", fmt.Errorf("clearing stale cache dir %s: %w", cacheDir, rmErr)
		}
	}

	if err := os.MkdirAll(filepath.Dir(cacheDir), 0755); err != nil {
		return "", fmt.Errorf("creating cache parent directory: %w", err)
	}

	if err := l.fetcher.FetchPackage(context.Background(), cacheDir, pkgAddr); err != nil {
		// On any failure, drop the partially-populated cache dir so the next
		// attempt starts clean instead of seeing a stale empty/half-populated tree.
		contract.IgnoreError(os.RemoveAll(cacheDir))
		return "", fmt.Errorf("fetching module from %q: %w", pkgAddr, err)
	}
	return cacheDir, nil
}

// dirHasFiles reports whether dir contains any entries.
func dirHasFiles(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	return len(entries) > 0
}

// resolveRegistrySource downloads a module from the Terraform Registry.
// Format: namespace/name/provider (e.g., hashicorp/consul/aws), with an
// optional ?version=… query.
//
// The registry's modules.v1 protocol returns the actual archive URL via the
// X-Terraform-Get header. That URL frequently uses go-getter schemes like
// `git::https://...?ref=<sha>`, so we feed it through PackageFetcher rather
// than calling http.Get directly.
func (l *Loader) resolveRegistrySource(source string) (string, error) {
	version := ""
	baseSource := source
	if before, query, ok := strings.Cut(source, "?"); ok {
		baseSource = before
		for param := range strings.SplitSeq(query, "&") {
			if after, ok := strings.CutPrefix(param, "version="); ok {
				version = after
			}
		}
	}

	parts := strings.Split(baseSource, "/")
	if len(parts) != 3 {
		return "", fmt.Errorf("invalid registry source format: %s", source)
	}
	namespace, name, provider := parts[0], parts[1], parts[2]

	downloadURL, err := l.getRegistryDownloadURL(namespace, name, provider, version)
	if err != nil {
		return "", err
	}

	// Cache by the *resolved* download URL: that pins the exact ref/version
	// the registry handed us, so two calls with different ?version= settings
	// don't collide even if the original source string looked similar.
	return l.fetchRemote(downloadURL, "registry")
}

type registryModuleVersion struct {
	Version string `json:"version"`
}

type registryModuleVersions struct {
	Modules []struct {
		Versions []registryModuleVersion `json:"versions"`
	} `json:"modules"`
}

func (l *Loader) getRegistryDownloadURL(namespace, name, provider, version string) (string, error) {
	baseURL := "https://registry.terraform.io/v1/modules"

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
		version = versions.Modules[0].Versions[0].Version
	}

	downloadURL := fmt.Sprintf("%s/%s/%s/%s/%s/download", baseURL, namespace, name, provider, version)
	resp, err := http.Get(downloadURL)
	if err != nil {
		return "", fmt.Errorf("getting download URL: %w", err)
	}
	defer contract.IgnoreClose(resp.Body)

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

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("reading download URL: %w", err)
	}
	return string(body), nil
}

// isRegistrySource checks if a source looks like a Terraform Registry address
// (namespace/name/provider). Subdomains/hostnames are filtered out by the
// `.` check on the first segment.
func isRegistrySource(source string) bool {
	if idx := strings.Index(source, "?"); idx != -1 {
		source = source[:idx]
	}
	parts := strings.Split(source, "/")
	if len(parts) != 3 {
		return false
	}
	for _, part := range parts {
		if part == "" || strings.Contains(part, ":") {
			return false
		}
	}
	return !strings.Contains(parts[0], ".")
}

func hashSource(source string) string {
	h := sha256.Sum256([]byte(source))
	return hex.EncodeToString(h[:8])
}

// GetCallStack returns the current module call stack (for debugging/error messages).
func (l *Loader) GetCallStack() []string {
	return append([]string{}, l.callStack...)
}
