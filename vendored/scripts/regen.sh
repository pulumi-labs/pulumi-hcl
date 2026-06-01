#!/usr/bin/env bash
# Regenerate vendored OpenTofu communicator code at the given upstream SHA.
#
# Usage: regen.sh <opentofu-sha>
#
# Invoked via `go generate ./vendored`; the SHA is the argument in
# vendored/doc.go's `//go:generate` directive. The output under
# vendored/communicator/ is a deterministic function of (SHA, this script).
# CI verifies that by re-running this script and asserting no git diff.

set -euo pipefail

if [[ $# -ne 1 ]]; then
  echo "usage: $0 <opentofu-sha>" >&2
  exit 2
fi

SHA="$1"
if [[ ! "$SHA" =~ ^[0-9a-f]{40}$ ]]; then
  echo "error: expected a 40-character lowercase hex SHA, got: $SHA" >&2
  exit 2
fi

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
VENDORED_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
COMMUNICATOR_DIR="$VENDORED_DIR/communicator"
GETMODULES_DIR="$VENDORED_DIR/getmodules"
COPY_DIR="$VENDORED_DIR/copy"

MODULE="github.com/pulumi-labs/pulumi-hcl"
UPSTREAM_MODULE="github.com/opentofu/opentofu"
UPSTREAM_COMMUNICATOR_PKG="$UPSTREAM_MODULE/internal/communicator"
UPSTREAM_PROVISIONERS_PKG="$UPSTREAM_MODULE/internal/provisioners"
UPSTREAM_LOGGING_PKG="$UPSTREAM_MODULE/internal/logging"
UPSTREAM_GETMODULES_PKG="$UPSTREAM_MODULE/internal/getmodules"
UPSTREAM_COPY_PKG="$UPSTREAM_MODULE/internal/copy"
UPSTREAM_HTTPCLIENT_PKG="$UPSTREAM_MODULE/internal/httpclient"
UPSTREAM_TRACING_TRACEATTRS_PKG="$UPSTREAM_MODULE/internal/tracing/traceattrs"
UPSTREAM_TRACING_PKG="$UPSTREAM_MODULE/internal/tracing"

VENDORED_COMMUNICATOR_PKG="$MODULE/vendored/communicator"
VENDORED_PROVISIONERS_PKG="$MODULE/pkg/provisioner/provisioners"
# shared/ lives outside the vendored tree because we replace the upstream
# implementation (which depended on configschema) with a smaller in-tree one.
VENDORED_SHARED_PKG="$MODULE/pkg/provisioner/communicator/shared"

VENDORED_GETMODULES_PKG="$MODULE/vendored/getmodules"
VENDORED_COPY_PKG="$MODULE/vendored/copy"
# httpclient/tracing are in-tree shims (Apache-2.0): we only need a no-op
# subset of the upstream surface and want to avoid the otelhttp / logging
# transitive dep graph.
SHIM_HTTPCLIENT_PKG="$MODULE/pkg/util/httpclient"
SHIM_TRACING_TRACEATTRS_PKG="$MODULE/pkg/util/tracing/traceattrs"
SHIM_TRACING_PKG="$MODULE/pkg/util/tracing"

# Stash the SHA we're regenerating against; useful for CI diagnostics.
echo "regen: target SHA $SHA" >&2

# Fetch upstream tarball into a temp directory.
TMPDIR="$(mktemp -d)"
trap 'rm -rf "$TMPDIR"' EXIT

TARBALL="$TMPDIR/opentofu.tar.gz"
URL="https://codeload.github.com/opentofu/opentofu/tar.gz/$SHA"
echo "regen: downloading $URL" >&2
curl --fail --silent --show-error --location --output "$TARBALL" "$URL"

# Extract only the subtrees we care about.
EXTRACT_ROOT="$TMPDIR/extract"
mkdir -p "$EXTRACT_ROOT"
# The tarball top-level is opentofu-<SHA>/. Strip it.
tar -xzf "$TARBALL" -C "$EXTRACT_ROOT" --strip-components=1 \
  "opentofu-$SHA/internal/communicator" \
  "opentofu-$SHA/internal/getmodules" \
  "opentofu-$SHA/internal/copy"

SRC="$EXTRACT_ROOT/internal/communicator"
GETMODULES_SRC="$EXTRACT_ROOT/internal/getmodules"
COPY_SRC="$EXTRACT_ROOT/internal/copy"
for d in "$SRC" "$GETMODULES_SRC" "$COPY_SRC"; do
  if [[ ! -d "$d" ]]; then
    echo "error: expected $d to exist after extraction" >&2
    exit 1
  fi
done

# Wipe the existing vendored tree so removed-upstream files don't linger.
rm -rf "$COMMUNICATOR_DIR"
mkdir -p "$COMMUNICATOR_DIR"

# Copy upstream tree.
cp -R "$SRC"/. "$COMMUNICATOR_DIR"/

# Drop content we don't want to ship:
#   - tests (we run our own integration tests against the public API)
#   - upstream mock (we own our own mocks)
#   - WinRM if the SHA predates its removal (we don't support WinRM)
#   - shared/ — replaced by an in-tree shim (see VENDORED_SHARED_PKG)
find "$COMMUNICATOR_DIR" -type f -name '*_test.go' -delete
find "$COMMUNICATOR_DIR" -type f -name 'communicator_mock.go' -delete
rm -rf "$COMMUNICATOR_DIR/winrm"
rm -rf "$COMMUNICATOR_DIR/shared"

# Rewrite import paths. Perl is portable across macOS and Linux (BSD vs GNU
# sed -i quirks); pin to it explicitly.
#
# Note the ordering: rewrite the shared package before the broader
# communicator prefix, since shared lives under communicator/ upstream.
find "$COMMUNICATOR_DIR" -type f -name '*.go' -print0 \
  | xargs -0 perl -pi \
      -e "s|\"$UPSTREAM_COMMUNICATOR_PKG/shared\"|\"$VENDORED_SHARED_PKG\"|g;" \
      -e "s|\"$UPSTREAM_COMMUNICATOR_PKG|\"$VENDORED_COMMUNICATOR_PKG|g;" \
      -e "s|\"$UPSTREAM_PROVISIONERS_PKG\"|\"$VENDORED_PROVISIONERS_PKG\"|g;"

# Strip the upstream side-effect logging import. It configures global log
# defaults we don't want vendored code to set; our hosting process owns
# logging configuration. Drop any line that imports it (typically a blank
# `_` import) and any immediately-preceding blank line inside the import
# group; gofmt later normalizes residual whitespace.
find "$COMMUNICATOR_DIR" -type f -name '*.go' -print0 \
  | LOGGING_PKG="$UPSTREAM_LOGGING_PKG" xargs -0 perl -ni \
      -e 'print unless /^\s*_\s+"\Q$ENV{LOGGING_PKG}\E"\s*$/;'

# Format. gofmt is deterministic for a given Go version.
gofmt -w "$COMMUNICATOR_DIR"

# ---------------------------------------------------------------------------
# vendored/copy: upstream internal/copy verbatim, just with the package
# import path rewritten.
# ---------------------------------------------------------------------------
rm -rf "$COPY_DIR"
mkdir -p "$COPY_DIR"
cp -R "$COPY_SRC"/. "$COPY_DIR"/
find "$COPY_DIR" -type f -name '*_test.go' -delete
gofmt -w "$COPY_DIR"

# ---------------------------------------------------------------------------
# vendored/getmodules: upstream internal/getmodules with the OCI getter
# stripped (it pulls a heavy containerd/OCI dep graph we don't need for
# Terraform module sources) and replaced by an in-tree stub. httpclient and
# tracing imports are rewritten to in-tree no-op shims.
# ---------------------------------------------------------------------------
rm -rf "$GETMODULES_DIR"
mkdir -p "$GETMODULES_DIR"
cp -R "$GETMODULES_SRC"/. "$GETMODULES_DIR"/

# Drop tests and the upstream OCI getter (replaced by oci_stub.go below).
find "$GETMODULES_DIR" -type f -name '*_test.go' -delete
rm -f "$GETMODULES_DIR/oci_getter.go"

# Strip the OCI block from installer.go: the upstream wires
# `getters["oci"] = &ociDistributionGetter{...}` from env.OCIRepositoryStore.
# Without the OCI getter neither type exists, so remove the assignment and
# the comment that precedes it, and remove the OCIRepositoryStore method
# from PackageFetcherEnvironment and noopPackageFetcherEnvironment.
perl -i -0pe '
  s|\n\s*// The OCI Distribution getter[^\n]*\n(\s*//[^\n]*\n)*\s*getters\["oci"\] = &ociDistributionGetter\{[^}]*\}\n||s;
  s|\n\s*OCIRepositoryStore\(ctx context\.Context, registryDomainName, repositoryPath string\) \(OCIRepositoryStore, error\)\n||s;
  s|\n// OCIRepositoryStore implements PackageFetcherEnvironment\.\nfunc \(n noopPackageFetcherEnvironment\) OCIRepositoryStore\([^)]*\) \(OCIRepositoryStore, error\) \{[^}]*\}\n||s;
' "$GETMODULES_DIR/installer.go"

# Strip the "oci" key from goGetterGetters in getter.go and the OCI
# decompressor-media-type map (it referenced types only declared in
# oci_getter.go).
perl -i -0pe '
  s|\n\s*"oci":\s*nil,[^\n]*\n|\n|g;
  s|\n// This is a map from media types as used in OCI descriptors[\s\S]*?\nvar goGetterDecompressorMediaTypes = map\[string\]string\{[\s\S]*?\}\n||s;
' "$GETMODULES_DIR/getter.go"

# Write a minimal in-tree stub providing the two OCI types still referenced
# by installer.go's interface signature (after stripping the method body the
# interface itself is empty, but the named type still has to exist for any
# external callers).
# The OCI surgery above removed the only use of `fmt` in installer.go
# (the noopPackageFetcherEnvironment.OCIRepositoryStore method body). Drop
# the now-unused import so gofmt doesn't refuse to format the file.
perl -i -ne 'print unless /^\s*"fmt"\s*$/' "$GETMODULES_DIR/installer.go"

cat > "$GETMODULES_DIR/oci_stub.go" <<'EOF'
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
EOF

# Rewrite import paths in getmodules. Order matters: tracing/traceattrs
# must come before tracing/ to avoid prefix shadowing.
find "$GETMODULES_DIR" -type f -name '*.go' -print0 \
  | xargs -0 perl -pi \
      -e "s|\"$UPSTREAM_COPY_PKG\"|\"$VENDORED_COPY_PKG\"|g;" \
      -e "s|\"$UPSTREAM_HTTPCLIENT_PKG\"|\"$SHIM_HTTPCLIENT_PKG\"|g;" \
      -e "s|\"$UPSTREAM_TRACING_TRACEATTRS_PKG\"|\"$SHIM_TRACING_TRACEATTRS_PKG\"|g;" \
      -e "s|\"$UPSTREAM_TRACING_PKG\"|\"$SHIM_TRACING_PKG\"|g;"

gofmt -w "$GETMODULES_DIR"

# Sanity check: the vendored trees must build against their shims.
echo "regen: go build ./vendored/..." >&2
(cd "$VENDORED_DIR/.." && go build ./vendored/...)

# Summary.
comm_count=$(find "$COMMUNICATOR_DIR" -type f | wc -l | tr -d ' ')
gm_count=$(find "$GETMODULES_DIR" -type f | wc -l | tr -d ' ')
copy_count=$(find "$COPY_DIR" -type f | wc -l | tr -d ' ')
echo "regen: wrote $comm_count files under vendored/communicator/, $gm_count under vendored/getmodules/, $copy_count under vendored/copy/" >&2
