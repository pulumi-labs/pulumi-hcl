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
	"maps"
	"os"
	"path/filepath"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclparse"
	"github.com/zclconf/go-cty/cty"
)

// tfvarsValue evaluates one `name = value` assignment from a variable-value
// file. Variable-value files are evaluated in an empty scope: values are
// literals, and may not refer to anything else in the program. An assignment is
// evaluated only if a variable of that name is declared, so a value that cannot
// be evaluated is an error only when it is actually used.
func tfvarsValue(attr *hcl.Attribute) (cty.Value, error) {
	val, diags := attr.Expr.Value(nil)
	if diags.HasErrors() {
		return cty.NilVal, fmt.Errorf("reading %s: %s", attr.Range.Filename, diags.Error())
	}
	return val, nil
}

// loadTfvars reads the variable-value files that are loaded automatically from
// the root module directory — `terraform.tfvars`, `terraform.tfvars.json`, and
// every `*.auto.tfvars` and `*.auto.tfvars.json` — into root variable values. A
// name set by more than one file takes its value from the last file applied,
// which is the last in that list, breaking ties lexically by file name. Each
// assignment carries the file it came from in its source range.
func loadTfvars(dir string) (map[string]*hcl.Attribute, error) {
	// os.ReadDir sorts by file name, which is the order the files are applied
	// in: `terraform.tfvars` sorts ahead of `terraform.tfvars.json`, and the
	// automatic files are applied lexically among themselves.
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var files, auto []string
	for _, entry := range entries {
		switch name := entry.Name(); {
		case name == "terraform.tfvars" || name == "terraform.tfvars.json":
			files = append(files, name)
		case strings.HasSuffix(name, ".auto.tfvars") || strings.HasSuffix(name, ".auto.tfvars.json"):
			auto = append(auto, name)
		}
	}
	files = append(files, auto...)

	parser := hclparse.NewParser()
	values := map[string]*hcl.Attribute{}
	for _, name := range files {
		// A name matching one of these is a variable-value file whatever it is
		// on disk: a directory of that name is a failure to read the file, not
		// a file that isn't there.
		src, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", name, err)
		}
		var file *hcl.File
		var diags hcl.Diagnostics
		if strings.HasSuffix(name, ".json") {
			file, diags = parser.ParseJSON(src, name)
		} else {
			file, diags = parser.ParseHCL(src, name)
		}
		if diags.HasErrors() {
			return nil, fmt.Errorf("reading %s: %s", name, diags.Error())
		}
		attrs, diags := file.Body.JustAttributes()
		if diags.HasErrors() {
			return nil, fmt.Errorf("reading %s: %s", name, diags.Error())
		}
		maps.Copy(values, attrs)
	}
	return values, nil
}
