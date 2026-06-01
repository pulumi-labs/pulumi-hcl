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

// Package tracing is the in-tree replacement for opentofu's
// internal/tracing package. Vendored opentofu code is rewritten by
// regen.sh to import this package instead. We expose only the surface
// the vendored module-fetching code uses, returning OpenTelemetry no-op
// values so calls are valid but do nothing.
package tracing

import (
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
)

var noopTracer = noop.NewTracerProvider().Tracer("pulumi-hcl-noop")

// Tracer returns a no-op tracer.
func Tracer() trace.Tracer {
	return noopTracer
}

// SpanAttributes packages attributes into a SpanStartOption.
func SpanAttributes(attrs ...attribute.KeyValue) trace.SpanStartEventOption {
	return trace.WithAttributes(attrs...)
}
