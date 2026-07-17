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

package tfcompat_test

import (
	"testing"

	"github.com/pulumi-labs/pulumi-hcl/tests/testutil/sshd"
	"github.com/pulumi-labs/pulumi-hcl/tests/testutil/tfcompat"
	"github.com/pulumi-labs/pulumi-hcl/tests/testutil/tfcompat/providers"
)

// TestL2Provisioner_FileEmptyContent exercises a file provisioner whose
// `content` is an explicit empty string. OpenTofu treats the attribute as set
// (non-null) and uploads an empty file; pulumi-hcl treats empty-string content
// as unset and rejects the config, so the apply fails where OpenTofu succeeds.
func TestL2Provisioner_FileEmptyContent(t *testing.T) {
	t.Parallel()
	c := sshd.Start(t.Context(), t)

	tfcompat.RunCase(t, "l2_provisioner_file_empty_content", tfcompat.Case{
		Providers: []tfcompat.Provider{
			{Name: "simple", Factory: providers.SimpleProvider},
		},
		Config: sshConfig(c, nil),
	})
}
