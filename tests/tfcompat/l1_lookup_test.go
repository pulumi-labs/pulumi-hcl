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
)

// OpenTofu's `lookup(map, key, default)` returns the map's element type, so a
// returned default is converted to that element type. pulumi-hcl previously
// returned the default verbatim, so `lookup(tomap({a="1"}), "missing", 30)`
// produced the number 30 instead of the string "30".
func TestL1Lookup(t *testing.T) {
	t.Parallel()
	tfcompat.RunCase(t, "l1_lookup", tfcompat.Case{})
}
