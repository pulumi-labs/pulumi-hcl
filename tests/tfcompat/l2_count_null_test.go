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

// A count meta-argument that evaluates to null. OpenTofu rejects it with a
// clean diagnostic ("count" must not be null). pulumi-hcl's EvaluateCount
// checks IsKnown (null is known) but never IsNull, converts the null to a null
// number, and calls AsBigFloat() on it, which panics ("value is null") and
// crashes the language host instead of returning a diagnostic.
func TestL2CountNull(t *testing.T) {
	t.Parallel()
	tfcompat.RunCase(t, "l2_count_null", tfcompat.Case{
		Providers: []tfcompat.Provider{
			{Name: "simple", Factory: providers.SimpleProvider},
		},
		Stages: []tfcompat.Stage{
			{Mode: tfcompat.StageApply, ExpectErr: "null"},
		},
	})
}
