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

// TestL2ProviderFunction exercises provider-defined functions
// (provider::pfx::concat_str) end to end: a call with literal arguments, a
// call whose argument is another resource's computed id, a call in an output,
// and a null argument to a null-allowing parameter. The protocol recorder
// compares the CallFunction operations both runtimes issue.
func TestL2ProviderFunction(t *testing.T) {
	t.Parallel()
	tfcompat.RunCase(t, "l2_provider_function", tfcompat.Case{
		Providers: []tfcompat.Provider{
			{Name: "pfx", PFFactory: providers.PFXProvider},
		},
	})
}

// TestL2ProviderFunctionUnknownPreview holds preview to plan behavior: a
// function argument fed by a not-yet-created resource's id is unknown, so
// neither runtime may call the function during the preview walk.
func TestL2ProviderFunctionUnknownPreview(t *testing.T) {
	t.Parallel()
	tfcompat.RunCase(t, "l2_provider_function_unknown_preview", tfcompat.Case{
		Providers: []tfcompat.Provider{
			{Name: "pfx", PFFactory: providers.PFXProvider},
		},
		Stages: []tfcompat.Stage{{Mode: tfcompat.StagePreview}},
	})
}

// TestL2ProviderFunctionError compares error propagation: the function
// returns an error for first == "boom" and both runtimes must fail with it.
func TestL2ProviderFunctionError(t *testing.T) {
	t.Parallel()
	tfcompat.RunCase(t, "l2_provider_function_error", tfcompat.Case{
		Providers: []tfcompat.Provider{
			{Name: "pfx", PFFactory: providers.PFXProvider},
		},
		Stages: []tfcompat.Stage{{ExpectErr: "concat_str: intentional failure"}},
	})
}
