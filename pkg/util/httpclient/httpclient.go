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

// Package httpclient is the in-tree replacement for opentofu's
// internal/httpclient package. Vendored opentofu code is rewritten by
// regen.sh to import this package instead. We only implement the surface
// the vendored module-fetching code actually uses.
package httpclient

import (
	"context"
	"net/http"
)

// New returns an HTTP client suitable for use by go-getter. Upstream wires
// OpenTelemetry into the transport; we keep it simple — our hosting process
// owns observability concerns.
func New(_ context.Context) *http.Client {
	return &http.Client{}
}
