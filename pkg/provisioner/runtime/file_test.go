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

package runtime

import (
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/stretchr/testify/assert"
)

// errConnRequired is the error runFile/runRemoteExec return once argument
// validation has passed but the Spec has no connection block; tests use it as
// a sentinel for "the config was accepted".
const errConnRequired = "connection block required for remote-exec / file provisioners"

func TestRunFileArgValidation(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		config  string
		wantErr string
	}{
		{
			name: "empty content is set",
			config: `
content     = ""
destination = "/tmp/x"
`,
			wantErr: errConnRequired,
		},
		{
			name: "empty source is set",
			config: `
source      = ""
destination = "/tmp/x"
`,
			wantErr: errConnRequired,
		},
		{
			name:    "neither set",
			config:  `destination = "/tmp/x"`,
			wantErr: "file: exactly one of source or content must be set",
		},
		{
			name: "null content is unset",
			config: `
content     = null
destination = "/tmp/x"
`,
			wantErr: "file: exactly one of source or content must be set",
		},
		{
			name: "both set",
			config: `
source      = "a"
content     = "b"
destination = "/tmp/x"
`,
			wantErr: "file: exactly one of source or content must be set",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			body := parseBody(t, tt.config)
			err := Run(t.Context(), &Spec{Type: "file", Config: body}, &hcl.EvalContext{})
			assert.EqualError(t, err, tt.wantErr)
		})
	}
}
