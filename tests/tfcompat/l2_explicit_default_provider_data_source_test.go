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

	"github.com/pulumi-labs/pulumi-hcl/tests/testutil/tfcompat"
	"github.com/pulumi-labs/pulumi-hcl/tests/testutil/tfcompat/providers"
)

// TestL2ExplicitDefaultProviderDataSource pins a data source to the default
// (un-aliased) provider with a bare-name `provider = simple` reference — the
// data-source sibling of TestL2ExplicitDefaultProvider, and the shape that the
// terraform-google-modules private-cluster submodule hits via
// `data "google_compute_zones" "available" { provider = google }`.
func TestL2ExplicitDefaultProviderDataSource(t *testing.T) {
	t.Parallel()
	tfcompat.RunCase(t, "l2_explicit_default_provider_data_source", tfcompat.Case{
		Providers: []tfcompat.Provider{
			{Name: "simple", Factory: providers.SimpleProvider},
		},
	})
}
