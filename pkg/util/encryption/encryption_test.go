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

package encryption

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDisabledPassthrough(t *testing.T) {
	t.Parallel()
	enc := StateEncryptionDisabled()

	out, err := enc.EncryptState([]byte(`{"version": 4}`))
	require.NoError(t, err)
	assert.Equal(t, `{"version": 4}`, string(out))

	out, status, err := enc.DecryptState([]byte(`{"version": 4}`))
	require.NoError(t, err)
	assert.Equal(t, StatusSatisfied, status)
	assert.Equal(t, `{"version": 4}`, string(out))
}

func TestIsEncryptionPayload(t *testing.T) {
	t.Parallel()

	ok, err := IsEncryptionPayload([]byte(`{"encryption_version": "v0"}`))
	require.NoError(t, err)
	assert.True(t, ok)

	ok, err = IsEncryptionPayload([]byte(`{"version": 4}`))
	require.NoError(t, err)
	assert.False(t, ok)

	_, err = IsEncryptionPayload([]byte(`not json`))
	assert.Error(t, err)
}
