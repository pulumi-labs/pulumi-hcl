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

// rsadecrypt should accept an OpenSSH-format RSA private key, as OpenTofu does
// via ssh.ParseRawPrivateKey. pulumi-hcl only parses PKCS#1/PKCS#8 keys, so it
// fails to parse the key and errors instead of decrypting.
func TestL1RsadecryptOpenssh(t *testing.T) {
	t.Parallel()
	tfcompat.RunCase(t, "l1_rsadecrypt_openssh", tfcompat.Case{})
}
