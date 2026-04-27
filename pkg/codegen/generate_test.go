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

package codegen

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestOrderedSourceFiles(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		source map[string]string
		want   []string
	}{
		{
			name:   "empty",
			source: map[string]string{},
			want:   []string{},
		},
		{
			name:   "main only",
			source: map[string]string{"main.pp": ""},
			want:   []string{"main.pp"},
		},
		{
			name: "main first, then alphabetical",
			source: map[string]string{
				"outputs.pp":   "",
				"main.pp":      "",
				"variables.pp": "",
			},
			want: []string{"main.pp", "outputs.pp", "variables.pp"},
		},
		{
			name: "no main, alphabetical",
			source: map[string]string{
				"outputs.pp":   "",
				"variables.pp": "",
			},
			want: []string{"outputs.pp", "variables.pp"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.want, orderedSourceFiles(tc.source))
		})
	}
}

func TestPackageDeclarationsByFile(t *testing.T) {
	t.Parallel()

	source := map[string]string{
		"main.pp": `resource "res" "aws:s3:Bucket" {
}
`,
		"output.pp": `package "output" {
  baseProviderName    = "output"
  baseProviderVersion = "23.0.0"
}
`,
		"providers.pp": `package "aws" {
  baseProviderName    = "aws"
  baseProviderVersion = "1.0.0"
}

package "random" {
  baseProviderName    = "random"
  baseProviderVersion = "2.0.0"
}
`,
	}

	got := packageDeclarationsByFile(source)
	want := map[string]map[string]bool{
		"output.pp":    {"output": true},
		"providers.pp": {"aws": true, "random": true},
	}
	assert.Equal(t, want, got)
}

func TestOutputFileName(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "main.hcl", outputFileName("main.pp"))
	assert.Equal(t, "output.hcl", outputFileName("output.pp"))
	assert.Equal(t, "no-ext.hcl", outputFileName("no-ext"))
}
