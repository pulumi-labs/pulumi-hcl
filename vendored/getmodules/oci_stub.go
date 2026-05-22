// Copyright 2026, Pulumi Corporation.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.

// Stub for the upstream OCI getter, which is stripped by regen.sh because
// pulumi-hcl does not need to fetch modules from OCI registries and the
// upstream implementation pulls a large containerd/OCI dep graph.

package getmodules

// OCIRepositoryStore is a placeholder so PackageFetcherEnvironment
// implementations from outside this package still type-check. The fetcher
// never invokes any methods on it because the upstream oci getter is gone.
type OCIRepositoryStore interface{}
