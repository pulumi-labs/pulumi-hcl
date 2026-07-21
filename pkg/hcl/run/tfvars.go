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

package run

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclparse"
	"github.com/zclconf/go-cty/cty"
)

// loadTfvars reads the variable-value files that are loaded automatically from
// the root module directory — `terraform.tfvars`, `terraform.tfvars.json`, and
// every `*.auto.tfvars` and `*.auto.tfvars.json` — into root variable values. A
// name set by more than one file takes its value from the last file applied,
// which is the last in that list, breaking ties lexically by file name.
func loadTfvars(dir string) (map[string]cty.Value, error) {
	// os.ReadDir sorts by file name, which is the order the files are applied
	// in: `terraform.tfvars` sorts ahead of `terraform.tfvars.json`, and the
	// automatic files are applied lexically among themselves.
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var files, auto []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		switch name := entry.Name(); {
		case name == "terraform.tfvars" || name == "terraform.tfvars.json":
			files = append(files, filepath.Join(dir, name))
		case strings.HasSuffix(name, ".auto.tfvars") || strings.HasSuffix(name, ".auto.tfvars.json"):
			auto = append(auto, filepath.Join(dir, name))
		}
	}
	files = append(files, auto...)

	parser := hclparse.NewParser()
	values := map[string]cty.Value{}
	for _, path := range files {
		var file *hcl.File
		var diags hcl.Diagnostics
		if strings.HasSuffix(path, ".json") {
			file, diags = parser.ParseJSONFile(path)
		} else {
			file, diags = parser.ParseHCLFile(path)
		}
		if diags.HasErrors() {
			return nil, fmt.Errorf("reading %s: %s", filepath.Base(path), diags.Error())
		}
		attrs, diags := file.Body.JustAttributes()
		if diags.HasErrors() {
			return nil, fmt.Errorf("reading %s: %s", filepath.Base(path), diags.Error())
		}
		for name, attr := range attrs {
			// Variable files are evaluated in an empty scope: values are
			// literals, and may not refer to anything else in the program.
			val, diags := attr.Expr.Value(nil)
			if diags.HasErrors() {
				return nil, fmt.Errorf("reading %s: %s", filepath.Base(path), diags.Error())
			}
			values[name] = val
		}
	}
	return values, nil
}
