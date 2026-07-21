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
	"path/filepath"
	"testing"

	"github.com/pulumi-labs/pulumi-hcl/pkg/hcl/packages"
	"github.com/pulumi/pulumi/sdk/v3/go/property"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLowerRemoteStateInvoke(t *testing.T) {
	t.Parallel()

	localReq := InvokeRequest{
		Token: packages.LocalReferenceToken,
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
		got, _, err := lowerRemoteStateInvoke(packages.RemoteStateType, localReq)
		require.NoError(t, err)
		assert.Equal(t, InvokeRequest{
			Token: packages.LocalReferenceToken,
			Args: property.NewMap(map[string]property.Value{
				"path":         property.New("state.tfstate"),
				"workspaceDir": property.New("envs"),
			}),
		}, got)
	})

	t.Run("remote backend defers to getRemoteReference", func(t *testing.T) {
		t.Parallel()
		got, _, err := lowerRemoteStateInvoke(packages.RemoteStateType, InvokeRequest{
			Token: packages.LocalReferenceToken,
			Args: property.NewMap(map[string]property.Value{
				"backend": property.New("remote"),
				"config": property.New(map[string]property.Value{
					"organization": property.New("acme"),
					"workspaces":   property.New(map[string]property.Value{"name": property.New("prod")}),
				}),
			}),
		})
		require.NoError(t, err)
		assert.Equal(t, InvokeRequest{
			Token: packages.RemoteReferenceToken,
			Args: property.NewMap(map[string]property.Value{
				"organization": property.New("acme"),
				"workspaces":   property.New(map[string]property.Value{"name": property.New("prod")}),
			}),
		}, got)
	})

	t.Run("unsupported backend is rejected", func(t *testing.T) {
		t.Parallel()
		_, _, err := lowerRemoteStateInvoke(packages.RemoteStateType, InvokeRequest{
			Args: property.NewMap(map[string]property.Value{"backend": property.New("s3")}),
		})
		require.EqualError(t, err,
			`terraform_remote_state: backend "s3" is not supported (supported backends: local, remote)`)
	})

	t.Run("config fields the local backend ignores are rejected", func(t *testing.T) {
		t.Parallel()
		_, _, err := lowerRemoteStateInvoke(packages.RemoteStateType, InvokeRequest{
			Args: property.NewMap(map[string]property.Value{
				"backend": property.New("local"),
				"config": property.New(map[string]property.Value{
					"path":   property.New("state.tfstate"),
					"bucket": property.New("oops"),
				}),
			}),
		})
		require.EqualError(t, err,
			`terraform_remote_state: the local backend does not read config field(s) [bucket] (supported: path, workspace_dir)`)
	})

	t.Run("config fields the remote backend ignores are rejected", func(t *testing.T) {
		t.Parallel()
		_, _, err := lowerRemoteStateInvoke(packages.RemoteStateType, InvokeRequest{
			Args: property.NewMap(map[string]property.Value{
				"backend": property.New("remote"),
				"config": property.New(map[string]property.Value{
					"organization": property.New("acme"),
					"bucket":       property.New("b"),
				}),
			}),
		})
		require.EqualError(t, err,
			`terraform_remote_state: backend "remote" does not read config field(s) [bucket] `+
				`(supported: hostname, organization, token, workspaces)`)
	})

	t.Run("defaults is returned for the result overlay", func(t *testing.T) {
		t.Parallel()
		defaultsVal := map[string]property.Value{"number": property.New(99.0)}
		_, defaults, err := lowerRemoteStateInvoke(packages.RemoteStateType, InvokeRequest{
			Args: property.NewMap(map[string]property.Value{
				"backend":  property.New("local"),
				"config":   property.New(map[string]property.Value{"path": property.New("state.tfstate")}),
				"defaults": property.New(defaultsVal),
			}),
		})
		require.NoError(t, err)
		assert.Equal(t, property.NewMap(defaultsVal), defaults)
	})

	t.Run("non-object defaults is rejected", func(t *testing.T) {
		t.Parallel()
		_, _, err := lowerRemoteStateInvoke(packages.RemoteStateType, InvokeRequest{
			Args: property.NewMap(map[string]property.Value{
				"backend":  property.New("local"),
				"defaults": property.New("oops"),
			}),
		})
		require.EqualError(t, err, `terraform_remote_state: "defaults" must be an object`)
	})

	t.Run("workspace combines with the workspaces prefix", func(t *testing.T) {
		t.Parallel()
		got, _, err := lowerRemoteStateInvoke(packages.RemoteStateType, InvokeRequest{
			Args: property.NewMap(map[string]property.Value{
				"backend":   property.New("remote"),
				"workspace": property.New("prod"),
				"config": property.New(map[string]property.Value{
					"organization": property.New("acme"),
					"workspaces":   property.New(map[string]property.Value{"prefix": property.New("vpc-")}),
				}),
			}),
		})
		require.NoError(t, err)
		assert.Equal(t, InvokeRequest{
			Token: packages.RemoteReferenceToken,
			Args: property.NewMap(map[string]property.Value{
				"organization": property.New("acme"),
				"workspaces":   property.New(map[string]property.Value{"name": property.New("vpc-prod")}),
			}),
		}, got)
	})

	t.Run("workspace without a prefix is rejected", func(t *testing.T) {
		t.Parallel()
		_, _, err := lowerRemoteStateInvoke(packages.RemoteStateType, InvokeRequest{
			Args: property.NewMap(map[string]property.Value{
				"backend":   property.New("remote"),
				"workspace": property.New("prod"),
				"config":    property.New(map[string]property.Value{"organization": property.New("acme")}),
			}),
		})
		require.EqualError(t, err,
			`terraform_remote_state: workspace requires config.workspaces.prefix`)
	})

	t.Run("workspace=default with workspaces.name is allowed", func(t *testing.T) {
		t.Parallel()
		got, _, err := lowerRemoteStateInvoke(packages.RemoteStateType, InvokeRequest{
			Args: property.NewMap(map[string]property.Value{
				"backend":   property.New("remote"),
				"workspace": property.New("default"),
				"config": property.New(map[string]property.Value{
					"organization": property.New("acme"),
					"workspaces":   property.New(map[string]property.Value{"name": property.New("staging")}),
				}),
			}),
		})
		require.NoError(t, err)
		assert.Equal(t, InvokeRequest{
			Token: packages.RemoteReferenceToken,
			Args: property.NewMap(map[string]property.Value{
				"organization": property.New("acme"),
				"workspaces":   property.New(map[string]property.Value{"name": property.New("staging")}),
			}),
		}, got)
	})

	t.Run("non-default workspace with workspaces.name is rejected", func(t *testing.T) {
		t.Parallel()
		_, _, err := lowerRemoteStateInvoke(packages.RemoteStateType, InvokeRequest{
			Args: property.NewMap(map[string]property.Value{
				"backend":   property.New("remote"),
				"workspace": property.New("prod"),
				"config": property.New(map[string]property.Value{
					"organization": property.New("acme"),
					"workspaces":   property.New(map[string]property.Value{"name": property.New("staging")}),
				}),
			}),
		})
		require.EqualError(t, err,
			`terraform_remote_state: workspace "prod" is invalid with config.workspaces.name (only "default" is)`)
	})

	t.Run("local workspace resolves to a path under workspace_dir", func(t *testing.T) {
		t.Parallel()
		got, _, err := lowerRemoteStateInvoke(packages.RemoteStateType, InvokeRequest{
			Args: property.NewMap(map[string]property.Value{
				"backend":   property.New("local"),
				"workspace": property.New("staging"),
				"config": property.New(map[string]property.Value{
					"path":          property.New("state.tfstate"),
					"workspace_dir": property.New("envs"),
				}),
			}),
		})
		require.NoError(t, err)
		assert.Equal(t, InvokeRequest{
			Token: packages.LocalReferenceToken,
			Args: property.NewMap(map[string]property.Value{
				"path": property.New(filepath.Join("envs", "staging", "terraform.tfstate")),
			}),
		}, got)
	})

	t.Run("local workspace defaults workspace_dir to terraform.tfstate.d", func(t *testing.T) {
		t.Parallel()
		got, _, err := lowerRemoteStateInvoke(packages.RemoteStateType, InvokeRequest{
			Args: property.NewMap(map[string]property.Value{
				"backend":   property.New("local"),
				"workspace": property.New("staging"),
			}),
		})
		require.NoError(t, err)
		assert.Equal(t, InvokeRequest{
			Token: packages.LocalReferenceToken,
			Args: property.NewMap(map[string]property.Value{
				"path": property.New(filepath.Join("terraform.tfstate.d", "staging", "terraform.tfstate")),
			}),
		}, got)
	})

	t.Run("local workspace=default reads path as if unset", func(t *testing.T) {
		t.Parallel()
		got, _, err := lowerRemoteStateInvoke(packages.RemoteStateType, InvokeRequest{
			Args: property.NewMap(map[string]property.Value{
				"backend":   property.New("local"),
				"workspace": property.New("default"),
				"config": property.New(map[string]property.Value{
					"path":          property.New("state.tfstate"),
					"workspace_dir": property.New("envs"),
				}),
			}),
		})
		require.NoError(t, err)
		assert.Equal(t, InvokeRequest{
			Token: packages.LocalReferenceToken,
			Args: property.NewMap(map[string]property.Value{
				"path":         property.New("state.tfstate"),
				"workspaceDir": property.New("envs"),
			}),
		}, got)
	})

	t.Run("other data sources are untouched", func(t *testing.T) {
		t.Parallel()
		got, _, err := lowerRemoteStateInvoke("some_other_source", localReq)
		require.NoError(t, err)
		assert.Equal(t, localReq, got)
	})
}

func TestApplyRemoteStateDefaults(t *testing.T) {
	t.Parallel()

	ret := property.NewMap(map[string]property.Value{
		"outputs": property.New(map[string]property.Value{
			"greeting": property.New("hello"),
		}),
	})

	t.Run("no defaults returns the result unchanged", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, ret, applyRemoteStateDefaults(property.Map{}, ret))
	})

	t.Run("defaults fill absent outputs and state wins on overlap", func(t *testing.T) {
		t.Parallel()
		defaults := property.NewMap(map[string]property.Value{
			"greeting": property.New("DEFAULT"),
			"number":   property.New(99.0),
		})
		assert.Equal(t, property.NewMap(map[string]property.Value{
			"outputs": property.New(map[string]property.Value{
				"greeting": property.New("hello"),
				"number":   property.New(99.0),
			}),
		}), applyRemoteStateDefaults(defaults, ret))
	})
}

func TestRemoteStateResult(t *testing.T) {
	t.Parallel()

	args := property.NewMap(map[string]property.Value{
		"backend": property.New("local"),
		"config":  property.New(map[string]property.Value{"path": property.New("remote.tfstate")}),
	})
	ret := property.NewMap(map[string]property.Value{
		"outputs": property.New(map[string]property.Value{"greeting": property.New("hello")}),
	})

	assert.Equal(t, property.NewMap(map[string]property.Value{
		"outputs":   property.New(map[string]property.Value{"greeting": property.New("hello")}),
		"backend":   property.New("local"),
		"config":    property.New(map[string]property.Value{"path": property.New("remote.tfstate")}),
		"workspace": property.Value{},
		"defaults":  property.Value{},
	}), remoteStateResult(args, property.Map{}, ret))
}
