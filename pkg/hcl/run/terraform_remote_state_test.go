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

package run

import (
	"testing"

	"github.com/pulumi/pulumi/sdk/v3/go/property"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLowerRemoteStateInvoke(t *testing.T) {
	t.Parallel()

	localReq := InvokeRequest{
		Token: localReferenceToken,
		Args: property.NewMap(map[string]property.Value{
			"backend": property.New("local"),
			"config": property.New(map[string]property.Value{
				"path":          property.New("state.tfstate"),
				"workspace_dir": property.New("envs"),
			}),
		}),
	}

	t.Run("local backend translates config to invoke args", func(t *testing.T) {
		t.Parallel()
		got, err := lowerRemoteStateInvoke(remoteStateType, localReq)
		require.NoError(t, err)
		assert.Equal(t, InvokeRequest{
			Token: localReferenceToken,
			Args: property.NewMap(map[string]property.Value{
				"path":         property.New("state.tfstate"),
				"workspaceDir": property.New("envs"),
			}),
		}, got)
	})

	t.Run("unsupported backend is rejected", func(t *testing.T) {
		t.Parallel()
		_, err := lowerRemoteStateInvoke(remoteStateType, InvokeRequest{
			Args: property.NewMap(map[string]property.Value{"backend": property.New("s3")}),
		})
		require.EqualError(t, err,
			`terraform_remote_state: only the "local" backend is currently supported, got "s3"`)
	})

	t.Run("other data sources are untouched", func(t *testing.T) {
		t.Parallel()
		got, err := lowerRemoteStateInvoke("some_other_source", localReq)
		require.NoError(t, err)
		assert.Equal(t, localReq, got)
	})
}
