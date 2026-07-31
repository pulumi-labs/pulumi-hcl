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

	"github.com/pulumi/pulumi-hcl/tests/testutil/tfcompat"
	"github.com/pulumi/pulumi-hcl/tests/testutil/tfcompat/providers"
)

// A `data` block whose required argument depends on a not-yet-created
// resource must behave the same way under `pulumi preview` as it does
// under `tofu plan`: the data read is deferred and preview succeeds.
// Before the fix, pulumi preview invoked the provider with the unknown
// `query` and the provider rejected it with "Missing required argument".
func TestL2DataSourceUnknownInputPreview(t *testing.T) {
	t.Parallel()
	tfcompat.RunCase(t, "l2_data_source_unknown_input_preview", tfcompat.Case{
		Providers: []tfcompat.Provider{
			{Name: "simple", Factory: providers.SimpleProvider},
		},
		Stages: []tfcompat.Stage{{
			Mode: tfcompat.StagePreview,
		}},
	})
}
