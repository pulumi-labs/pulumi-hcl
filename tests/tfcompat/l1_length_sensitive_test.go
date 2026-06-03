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

// TestL1LengthSensitive exercises `length`'s mark propagation. OpenTofu carries
// the sensitivity of its argument onto the returned count for every supported
// type, including tuples and objects; pulumi-hcl must do the same rather than
// dropping the mark.
func TestL1LengthSensitive(t *testing.T) {
	t.Parallel()
	tfcompat.RunCase(t, "l1_length_sensitive", tfcompat.Case{})
}
