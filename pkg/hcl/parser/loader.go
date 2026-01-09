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
	"os"
	"path/filepath"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclparse"
)

// Loader handles loading and parsing HCL files from the filesystem.
type Loader struct {
	parser *hclparse.Parser
}

// NewLoader creates a new HCL file loader.
func NewLoader() *Loader {
	return &Loader{
		parser: hclparse.NewParser(),
	}
}

// LoadDirectory loads all HCL files from the specified directory.
// It looks for files with .hcl extension.
func (l *Loader) LoadDirectory(dir string) (map[string]*hcl.File, hcl.Diagnostics) {
	files := make(map[string]*hcl.File)
	var diags hcl.Diagnostics

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, hcl.Diagnostics{{
			Severity: hcl.DiagError,
			Summary:  "Failed to read directory",
			Detail:   err.Error(),
		}}
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		if !isHCLFile(name) {
			continue
		}

		path := filepath.Join(dir, name)
		file, fileDiags := l.LoadFile(path)
		diags = append(diags, fileDiags...)

		if file != nil {
			files[path] = file
		}
	}

	if len(files) == 0 && !diags.HasErrors() {
		diags = append(diags, &hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "No HCL files found",
			Detail:   "The directory does not contain any .hcl files.",
		})
	}

	return files, diags
}

// LoadFile loads and parses a single HCL file.
func (l *Loader) LoadFile(path string) (*hcl.File, hcl.Diagnostics) {
	src, err := os.ReadFile(path)
	if err != nil {
		return nil, hcl.Diagnostics{{
			Severity: hcl.DiagError,
			Summary:  "Failed to read file",
			Detail:   err.Error(),
			Subject:  &hcl.Range{Filename: path},
		}}
	}

	return l.ParseFile(path, src)
}

// ParseFile parses HCL source code.
func (l *Loader) ParseFile(filename string, src []byte) (*hcl.File, hcl.Diagnostics) {
	// Determine if it's HCL or JSON based on extension
	if strings.HasSuffix(filename, ".json") {
		return l.parser.ParseJSON(src, filename)
	}
	return l.parser.ParseHCL(src, filename)
}

// Files returns all files that have been parsed by this loader.
func (l *Loader) Files() map[string]*hcl.File {
	return l.parser.Files()
}

// isHCLFile returns true if the filename indicates an HCL configuration file.
func isHCLFile(name string) bool {
	return strings.HasSuffix(name, ".hcl") || strings.HasSuffix(name, ".hcl.json")
}
