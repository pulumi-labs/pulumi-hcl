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

// TestL2DataSourcePreconditionFail asserts a failing precondition on a data
// source blocks the read on both runtimes with the configured message.
func TestL2DataSourcePreconditionFail(t *testing.T) {
	t.Parallel()
	tfcompat.RunCase(t, "l2_data_source_precondition_fail", tfcompat.Case{
		Providers: []tfcompat.Provider{
			{Name: "simple", Factory: providers.SimpleProvider},
		},
		Stages: []tfcompat.Stage{{
			Mode:      tfcompat.StageApply,
			ExpectErr: "DATA_PRECONDITION_VIOLATED",
		}},
	})
}

// TestL2DataSourceConditionPass asserts passing precondition and postcondition
// blocks on a data source do not block the read on either runtime.
func TestL2DataSourceConditionPass(t *testing.T) {
	t.Parallel()
	tfcompat.RunCase(t, "l2_data_source_condition_pass", tfcompat.Case{
		Providers: []tfcompat.Provider{
			{Name: "simple", Factory: providers.SimpleProvider},
		},
	})
}
