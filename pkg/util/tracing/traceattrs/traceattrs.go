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

// Package traceattrs is the in-tree replacement for opentofu's
// internal/tracing/traceattrs package. Vendored opentofu code is rewritten
// by regen.sh to import this package; we expose only the attribute
// builders the vendored module-fetching code actually uses.
package traceattrs

import "go.opentelemetry.io/otel/attribute"

// URLFull returns an attribute representing an absolute URL.
func URLFull(val string) attribute.KeyValue {
	return attribute.String("url.full", val)
}
