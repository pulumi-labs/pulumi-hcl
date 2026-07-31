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

// Package encryption is an in-tree shim for the subset of OpenTofu's
// internal/encryption surface that vendored/statefile consumes. regen.sh
// rewrites the upstream import to point here so the vendored parser does not
// drag in the full encryption dependency graph (key providers, cloud auth).
// pulumi-hcl only reads unencrypted state; encrypted payloads are detected
// and rejected by the vendored reader via IsEncryptionPayload.
package encryption

import "encoding/json"

// EncryptionStatus mirrors the upstream type of the same name.
type EncryptionStatus int

const (
	StatusUnknown   EncryptionStatus = 0
	StatusSatisfied EncryptionStatus = 1
	StatusMigration EncryptionStatus = 2
)

// StateEncryption mirrors the two methods of the upstream interface that the
// state-file reader and writer call.
type StateEncryption interface {
	EncryptState([]byte) ([]byte, error)
	DecryptState([]byte) ([]byte, EncryptionStatus, error)
}

// StateEncryptionDisabled returns the passthrough implementation, matching
// upstream's disabled-encryption semantics.
func StateEncryptionDisabled() StateEncryption { return stateDisabled{} }

type stateDisabled struct{}

func (stateDisabled) EncryptState(plainState []byte) ([]byte, error) { return plainState, nil }

func (stateDisabled) DecryptState(encryptedState []byte) ([]byte, EncryptionStatus, error) {
	return encryptedState, StatusSatisfied, nil
}

// IsEncryptionPayload reports whether the document is an encrypted-state
// envelope, using the same sigil as upstream: a non-empty
// `encryption_version` key.
func IsEncryptionPayload(data []byte) (bool, error) {
	var envelope struct {
		Version string `json:"encryption_version"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		return false, err
	}
	return envelope.Version != "", nil
}
