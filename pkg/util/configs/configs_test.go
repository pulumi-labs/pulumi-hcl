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

package configs

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseProviderConfigCompactStr(t *testing.T) {
	t.Parallel()

	addr, diags := ParseProviderConfigCompactStr("aws")
	assert.False(t, diags.HasErrors())
	assert.Equal(t, "aws", addr.LocalName)
	assert.Empty(t, addr.Alias)

	addr, diags = ParseProviderConfigCompactStr("aws.east")
	assert.False(t, diags.HasErrors())
	assert.Equal(t, "aws", addr.LocalName)
	assert.Equal(t, "east", addr.Alias)

	_, diags = ParseProviderConfigCompactStr("aws.east.extra")
	assert.True(t, diags.HasErrors(), "extra traversal parts")

	_, diags = ParseProviderConfigCompactStr(`aws["east"]`)
	assert.True(t, diags.HasErrors(), "index step instead of attribute")

	_, diags = ParseProviderConfigCompactStr("not a traversal")
	assert.True(t, diags.HasErrors(), "unparsable input")
}
