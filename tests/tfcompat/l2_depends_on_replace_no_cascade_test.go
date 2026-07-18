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

// TestL2DependsOnReplaceNoCascade proves that a depends_on ordering
// dependency does not propagate replacement.
//
// `dependent` names the counted `pool` resource in depends_on and has only
// literal inputs. Between the two stages pool[0]'s ForceNew `force` changes,
// so pool[0] is replaced. depends_on carries no values, so `dependent` must
// be left alone — its inputs cannot have changed. Without explicit (empty)
// per-property dependency metadata, the engine assumes every input depends
// on every dependency and its delete-before-replace cascade spuriously
// replaces `dependent`.
func TestL2DependsOnReplaceNoCascade(t *testing.T) {
	t.Parallel()
	tfcompat.RunCase(t, "l2_depends_on_replace_no_cascade", tfcompat.Case{
		Providers: []tfcompat.Provider{
			{Name: "replacer", Factory: providers.ReplacerProvider},
		},
	})
}
