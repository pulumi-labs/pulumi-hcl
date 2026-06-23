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

	version "github.com/hashicorp/go-version"
	regaddr "github.com/opentofu/registry-address/v2"
	"github.com/opentofu/svchost"
	"github.com/opentofu/svchost/disco"
	"github.com/pulumi-labs/pulumi-hcl/pkg/hcl/ast"
	"github.com/pulumi-labs/pulumi-hcl/pkg/hcl/parser"
	"github.com/pulumi-labs/pulumi-hcl/vendored/getmodules"
	"github.com/pulumi/pulumi/sdk/v3/go/common/util/contract"
	"github.com/pulumi/pulumi/sdk/v3/go/common/workspace"
)

// fetchMu serializes every PackageFetcher.FetchPackage call in the process.
// The vendored getmodules package builds each PackageFetcher from a shallow
// clone of a package-global getter table, so distinct fetchers — and thus
// distinct Loaders — still share the same go-getter getter instances.
// go-getter mutates those getters on every fetch, which upstream OpenTofu
// only gets away with because it installs modules sequentially. We hold this
// lock to enforce that same single-flight contract.
var fetchMu sync.Mutex

// Loader loads and parses module configurations.
type Loader struct {
	mu sync.Mutex

	parser    *parser.Parser
	cache     map[string]*LoadedModule
	callStack []string
	cacheDir  string

	fetcher *getmodules.PackageFetcher

	// disco resolves a registry host to its modules.v1 endpoint via service
	// discovery. Tests pre-seed it with an httptest.Server via ForceHostServices.
	disco *disco.Disco
}

// LoadedModule represents a loaded and parsed module.
type LoadedModule struct {
	Config     *ast.Config
	SourcePath string
}

// NewLoader creates a new module loader.
func NewLoader(ctx context.Context) *Loader {
	return &Loader{
		parser:   parser.NewParser(),
		cache:    make(map[string]*LoadedModule),
		cacheDir: defaultCacheDir(),
		fetcher:  getmodules.NewPackageFetcher(ctx, nil),
		disco:    disco.New(),
	}
}

func defaultCacheDir() string {
	// Resolve "modules" under the Pulumi home the same way plugins resolve under
	// it (workspace.GetPluginDir uses GetPulumiPath), so PULUMI_HOME relocates the
	// module cache too.
	dir, err := workspace.GetPulumiPath("modules")
	if err != nil {
		return filepath.Join(os.TempDir(), "modules")
	}
	return dir
}

// LoadModule loads a module from the given source. versionConstraint is a
// Terraform-style constraint (`~> 4.0`, `>= 1.0.0`, `4.2.1`); ignored for
// non-registry sources.
func (l *Loader) LoadModule(source, versionConstraint, callerDir string) (*LoadedModule, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	resolvedPath, err := l.resolveSource(source, versionConstraint, callerDir)
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
// versionConstraint applies only to registry sources.
func (l *Loader) resolveSource(source, versionConstraint, callerDir string) (string, error) {
	source, subdir := getmodules.SplitPackageSubdir(source)

	var resolvedPath string
	var err error
	switch {
	case strings.HasPrefix(source, "./") || strings.HasPrefix(source, "../"):
		resolvedPath, err = l.resolveLocalSource(source, callerDir)
	case filepath.IsAbs(source):
		resolvedPath, err = l.resolveAbsoluteSource(source)
	case isRegistrySource(source):
		resolvedPath, err = l.resolveRegistrySource(source, versionConstraint)
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

// fetchRemote normalizes source and downloads it (if not cached) into
// cacheDir/kind/<hash>. Returns the populated cache directory.
func (l *Loader) fetchRemote(source, kind string) (string, error) {
	pkgAddr, detectedSubdir, err := getmodules.NormalizePackageAddress(source)
	if err != nil {
		return "", fmt.Errorf("normalizing module source %q: %w", source, err)
	}
	if detectedSubdir != "" {
		return "", fmt.Errorf("module source %q resolved to package %q with unexpected subdir %q",
			source, pkgAddr, detectedSubdir)
	}

	cacheDir := filepath.Join(l.cacheDir, kind, hashSource(pkgAddr))
	if info, statErr := os.Stat(cacheDir); statErr == nil && info.IsDir() {
		if dirHasFiles(cacheDir) {
			return cacheDir, nil
		}
		// Empty cache dir from a prior failed fetch — go-getter errors if
		// the target already exists, so wipe before retrying.
		if rmErr := os.RemoveAll(cacheDir); rmErr != nil {
			return "", fmt.Errorf("clearing stale cache dir %s: %w", cacheDir, rmErr)
		}
	}

	if err := os.MkdirAll(filepath.Dir(cacheDir), 0o755); err != nil {
		return "", fmt.Errorf("creating cache parent directory: %w", err)
	}

	// Grab a **package** level lock on using **any** [getmodules.PackageFetcher].
	fetchMu.Lock()
	defer fetchMu.Unlock()
	err = l.fetcher.FetchPackage(context.Background(), cacheDir, pkgAddr)
	if err != nil {
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

// resolveRegistrySource downloads a module via modules.v1. source is
// `[host/]namespace/name/provider` with an optional ?version=…; versionConstraint
// takes precedence over the query string.
func (l *Loader) resolveRegistrySource(source, versionConstraint string) (string, error) {
	baseSource := source
	if before, query, ok := strings.Cut(source, "?"); ok {
		baseSource = before
		for param := range strings.SplitSeq(query, "&") {
			if after, ok := strings.CutPrefix(param, "version="); ok && versionConstraint == "" {
				versionConstraint = after
			}
		}
	}

	mod, err := regaddr.ParseModuleSource(baseSource)
	if err != nil {
		return "", fmt.Errorf("invalid registry source format %q: %w", source, err)
	}
	pkg := mod.Package

	baseURL, err := l.registryBaseURLForHost(pkg.Host)
	if err != nil {
		return "", err
	}

	downloadURL, err := l.getRegistryDownloadURL(baseURL, pkg.Namespace, pkg.Name, pkg.TargetSystem, versionConstraint)
	if err != nil {
		return "", err
	}

	// Cache by the resolved URL so different version constraints don't collide.
	return l.fetchRemote(downloadURL, "registry")
}

// registryBaseURLForHost resolves a registry host to its modules.v1 base URL
// via service discovery.
func (l *Loader) registryBaseURLForHost(host svchost.Hostname) (string, error) {
	u, err := l.disco.DiscoverServiceURL(context.Background(), host, "modules.v1")
	if err != nil {
		return "", fmt.Errorf("discovering registry %q: %w", host, err)
	}
	return strings.TrimSuffix(u.String(), "/"), nil
}

type registryModuleVersion struct {
	Version string `json:"version"`
}

type registryModuleVersions struct {
	Modules []struct {
		Versions []registryModuleVersion `json:"versions"`
	} `json:"modules"`
}

// getRegistryDownloadURL resolves a concrete download URL via the modules.v1
// protocol. Empty versionConstraint means "highest published version".
func (l *Loader) getRegistryDownloadURL(baseURL, namespace, name, provider, versionConstraint string) (string, error) {
	chosen, err := l.selectRegistryVersion(baseURL, namespace, name, provider, versionConstraint)
	if err != nil {
		return "", err
	}

	downloadURL := fmt.Sprintf("%s/%s/%s/%s/%s/download", baseURL, namespace, name, provider, chosen)
	resp, err := http.Get(downloadURL)
	if err != nil {
		return "", fmt.Errorf("getting download URL: %w", err)
	}
	defer contract.IgnoreClose(resp.Body)

	// Terraform Registry: 204 + X-Terraform-Get header.
	// OpenTofu Registry:  200 + JSON body {"location": "..."}.
	// Some impls:         200 + raw URL body.
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

	if hdr := resp.Header.Get("X-Terraform-Get"); hdr != "" {
		return hdr, nil
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("reading download URL: %w", err)
	}
	trimmed := strings.TrimSpace(string(body))
	if strings.HasPrefix(trimmed, "{") {
		var parsed struct {
			Location string `json:"location"`
		}
		if err := json.Unmarshal([]byte(trimmed), &parsed); err != nil {
			return "", fmt.Errorf("parsing registry download response: %w", err)
		}
		if parsed.Location == "" {
			return "", fmt.Errorf("registry download response missing 'location'")
		}
		return parsed.Location, nil
	}
	return trimmed, nil
}

// selectRegistryVersion returns the highest published version satisfying
// versionConstraint, or the highest overall when it's empty.
func (l *Loader) selectRegistryVersion(baseURL, namespace, name, provider, versionConstraint string) (string, error) {
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

	var constraints version.Constraints
	if versionConstraint != "" {
		constraints, err = version.NewConstraint(versionConstraint)
		if err != nil {
			return "", fmt.Errorf("parsing version constraint %q: %w", versionConstraint, err)
		}
	}

	candidates := make([]*version.Version, 0, len(versions.Modules[0].Versions))
	for _, v := range versions.Modules[0].Versions {
		parsed, parseErr := version.NewVersion(v.Version)
		if parseErr != nil {
			continue
		}
		if constraints != nil && !constraints.Check(parsed) {
			continue
		}
		candidates = append(candidates, parsed)
	}
	if len(candidates) == 0 {
		if versionConstraint != "" {
			return "", fmt.Errorf(
				"no published version of module %s/%s/%s satisfies constraint %q",
				namespace, name, provider, versionConstraint)
		}
		return "", fmt.Errorf("no valid versions for module %s/%s/%s", namespace, name, provider)
	}
	slices.SortFunc(candidates, func(a, b *version.Version) int { return b.Compare(a) })
	return candidates[0].Original(), nil
}

// isRegistrySource reports whether source is a Terraform Registry address
// (`[host/]namespace/name/provider`). Detection is delegated to
// opentofu/registry-address, which also rejects version-control hosts like
// github.com so they fall through to the remote getter. The query string
// (`?version=…`) is stripped first because registry addresses don't permit it.
func isRegistrySource(source string) bool {
	if idx := strings.Index(source, "?"); idx != -1 {
		source = source[:idx]
	}
	_, err := regaddr.ParseModuleSource(source)
	return err == nil
}

func hashSource(source string) string {
	h := sha256.Sum256([]byte(source))
	return hex.EncodeToString(h[:8])
}

// GetCallStack returns the current module call stack (for debugging/error messages).
func (l *Loader) GetCallStack() []string {
	return append([]string{}, l.callStack...)
}
