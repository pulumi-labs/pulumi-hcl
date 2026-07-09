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

package tfexec

import "sync"

// ImportStore stands in for the cloud the in-memory test providers lack, so
// that an import-time Read has somewhere to recover attributes from: every
// provider instance in a case shares the test process, and with it a per-case
// store written on Create/Update. Per-case scoping keeps the fixed resource
// ids of parallel cases from colliding.
type ImportStore struct {
	m sync.Map // storeKey -> map[string]any
}

func NewImportStore() *ImportStore {
	return &ImportStore{}
}

type storeKey struct {
	typeName, id string
}

func (s *ImportStore) put(typeName, id string, attrs map[string]any) {
	if s == nil || id == "" {
		return
	}
	s.m.Store(storeKey{typeName, id}, attrs)
}

func (s *ImportStore) get(typeName, id string) map[string]any {
	if s == nil || id == "" {
		return nil
	}
	if v, ok := s.m.Load(storeKey{typeName, id}); ok {
		attrs, _ := v.(map[string]any)
		return attrs
	}
	return nil
}

func (s *ImportStore) delete(typeName, id string) {
	if s == nil || id == "" {
		return
	}
	s.m.Delete(storeKey{typeName, id})
}
