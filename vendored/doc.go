// Copyright 2026, Pulumi Corporation.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.

// Package vendored holds third-party code copied verbatim from upstream
// projects and re-imported under our module path. Files under
// vendored/communicator/ and vendored/hcl2shim/ are MPL-2.0 and must keep
// their original headers; see
// vendored/LICENSE-MPL-2.0 and vendored/NOTICE.
//
// Do not edit vendored files by hand. Regenerate with `go generate ./vendored`.
// CI enforces that running the regen script produces no diff.
//
//go:generate ./scripts/regen.sh 5bd7f2897483e8c932f6ce56e339d418324ed65d
package vendored
