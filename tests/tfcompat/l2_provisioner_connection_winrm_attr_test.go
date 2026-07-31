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
	"strconv"
	"testing"

	"github.com/pulumi/pulumi-hcl/tests/testutil/sshd"
	"github.com/pulumi/pulumi-hcl/tests/testutil/tfcompat"
	"github.com/pulumi/pulumi-hcl/tests/testutil/tfcompat/providers"
)

// OpenTofu validates a connection block against a superset schema that
// includes WinRM-only attributes, and the SSH communicator ignores the ones
// it does not use. pulumi-hcl decodes against a spec that omits them and
// rejects the argument, so an SSH connection carrying a WinRM attribute
// applies in OpenTofu but errors in pulumi-hcl.
func TestL2ProvisionerConnectionWinRMAttr(t *testing.T) {
	t.Parallel()
	c := sshd.Start(t.Context(), t)
	tfcompat.RunCase(t, "l2_provisioner_connection_winrm_attr", tfcompat.Case{
		Providers: []tfcompat.Provider{
			{Name: "simple", Factory: providers.SimpleProvider},
		},
		Config: map[string]string{
			"host":     c.Host,
			"port":     strconv.Itoa(c.Port),
			"user":     c.User,
			"password": c.Password,
		},
	})
}
