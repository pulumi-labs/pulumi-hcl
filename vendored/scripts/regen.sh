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
IPADDR_DIR="$VENDORED_DIR/ipaddr"
HCL2SHIM_DIR="$VENDORED_DIR/hcl2shim"
STATEFILE_DIR="$VENDORED_DIR/statefile"
STATES_DIR="$VENDORED_DIR/states"
ADDRS_DIR="$VENDORED_DIR/addrs"
TFDIAGS_DIR="$VENDORED_DIR/tfdiags"
MARKS_DIR="$VENDORED_DIR/marks"
CHECKS_DIR="$VENDORED_DIR/checks"
LEGACY_HCL2SHIM_DIR="$VENDORED_DIR/legacy/hcl2shim"
VERSION_DIR="$VENDORED_DIR/version"

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

UPSTREAM_STATEFILE_PKG="$UPSTREAM_MODULE/internal/states/statefile"
UPSTREAM_STATES_PKG="$UPSTREAM_MODULE/internal/states"
UPSTREAM_ADDRS_PKG="$UPSTREAM_MODULE/internal/addrs"
UPSTREAM_TFDIAGS_PKG="$UPSTREAM_MODULE/internal/tfdiags"
UPSTREAM_MARKS_PKG="$UPSTREAM_MODULE/internal/lang/marks"
UPSTREAM_CHECKS_PKG="$UPSTREAM_MODULE/internal/checks"
UPSTREAM_LEGACY_HCL2SHIM_PKG="$UPSTREAM_MODULE/internal/legacy/hcl2shim"
UPSTREAM_ENCRYPTION_PKG="$UPSTREAM_MODULE/internal/encryption"
UPSTREAM_CONFIGS_PKG="$UPSTREAM_MODULE/internal/configs"
UPSTREAM_VERSION_PKG="$UPSTREAM_MODULE/version"

VENDORED_STATEFILE_PKG="$MODULE/vendored/statefile"
VENDORED_STATES_PKG="$MODULE/vendored/states"
VENDORED_ADDRS_PKG="$MODULE/vendored/addrs"
VENDORED_TFDIAGS_PKG="$MODULE/vendored/tfdiags"
VENDORED_MARKS_PKG="$MODULE/vendored/marks"
VENDORED_CHECKS_PKG="$MODULE/vendored/checks"
VENDORED_LEGACY_HCL2SHIM_PKG="$MODULE/vendored/legacy/hcl2shim"
VENDORED_VERSION_PKG="$MODULE/vendored/version"
# encryption/configs are in-tree shims (Apache-2.0): statefile only needs a
# passthrough StateEncryption plus the encrypted-payload sigil check, and one
# compact provider-address parser, and the upstream packages behind those
# drag in the key-provider / configuration-loader dependency graphs.
SHIM_ENCRYPTION_PKG="$MODULE/pkg/util/encryption"
SHIM_CONFIGS_PKG="$MODULE/pkg/util/configs"

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
  "opentofu-$SHA/internal/copy" \
  "opentofu-$SHA/internal/ipaddr" \
  "opentofu-$SHA/internal/configs/hcl2shim" \
  "opentofu-$SHA/internal/states" \
  "opentofu-$SHA/internal/addrs" \
  "opentofu-$SHA/internal/tfdiags" \
  "opentofu-$SHA/internal/lang/marks" \
  "opentofu-$SHA/internal/checks" \
  "opentofu-$SHA/internal/legacy/hcl2shim" \
  "opentofu-$SHA/version"

SRC="$EXTRACT_ROOT/internal/communicator"
GETMODULES_SRC="$EXTRACT_ROOT/internal/getmodules"
COPY_SRC="$EXTRACT_ROOT/internal/copy"
IPADDR_SRC="$EXTRACT_ROOT/internal/ipaddr"
HCL2SHIM_SRC="$EXTRACT_ROOT/internal/configs/hcl2shim"
STATES_SRC="$EXTRACT_ROOT/internal/states"
STATEFILE_SRC="$EXTRACT_ROOT/internal/states/statefile"
ADDRS_SRC="$EXTRACT_ROOT/internal/addrs"
TFDIAGS_SRC="$EXTRACT_ROOT/internal/tfdiags"
MARKS_SRC="$EXTRACT_ROOT/internal/lang/marks"
CHECKS_SRC="$EXTRACT_ROOT/internal/checks"
LEGACY_HCL2SHIM_SRC="$EXTRACT_ROOT/internal/legacy/hcl2shim"
VERSION_SRC="$EXTRACT_ROOT/version"
for d in "$SRC" "$GETMODULES_SRC" "$COPY_SRC" "$IPADDR_SRC" "$HCL2SHIM_SRC" \
         "$STATES_SRC" "$STATEFILE_SRC" "$ADDRS_SRC" "$TFDIAGS_SRC" \
         "$MARKS_SRC" "$CHECKS_SRC" "$LEGACY_HCL2SHIM_SRC" "$VERSION_SRC"; do
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
# vendored/ipaddr: upstream internal/ipaddr verbatim. It is a self-contained
# fork of a subset of the Go standard "net" package that retains the pre-Go-1.17
# IPv4 parsing behavior (leading-zero octets are decimal, not rejected), which
# the cidr* functions rely on for OpenTofu parity. It only imports the standard
# library, so no import rewriting is needed.
# ---------------------------------------------------------------------------
rm -rf "$IPADDR_DIR"
mkdir -p "$IPADDR_DIR"
cp -R "$IPADDR_SRC"/. "$IPADDR_DIR"/
find "$IPADDR_DIR" -type f -name '*_test.go' -delete
gofmt -w "$IPADDR_DIR"

# ---------------------------------------------------------------------------
# vendored/hcl2shim: only the override-merge body from upstream
# internal/configs/hcl2shim. merge_body.go implements the body that gives
# override files their "attributes replace, blocks shadow" semantics, and
# util.go holds the two schema helpers it calls. The rest of the package
# (mock value composition, synthetic bodies) is unused here. It imports only
# hcl and cty, so no import rewriting is needed.
# ---------------------------------------------------------------------------
rm -rf "$HCL2SHIM_DIR"
mkdir -p "$HCL2SHIM_DIR"
cp "$HCL2SHIM_SRC/merge_body.go" "$HCL2SHIM_SRC/util.go" "$HCL2SHIM_DIR"/
gofmt -w "$HCL2SHIM_DIR"

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


# ---------------------------------------------------------------------------
# vendored/{statefile,states,addrs,tfdiags,marks,checks,legacy/hcl2shim,version}:
# OpenTofu's state-file parser and its dependency closure, with the heavy
# tails cut:
#   - internal/encryption and internal/configs become in-tree shims
#     (pkg/util/encryption, pkg/util/configs): statefile only needs a
#     passthrough StateEncryption + the encrypted-payload sigil, and one
#     compact provider-address parser.
#   - states loses its getproviders-facing ProviderRequirements method and
#     the checks.State write-side helpers (NewCheckResults,
#     RecordCheckResults), so internal/getproviders and the config-coupled
#     parts of internal/checks stay out; checks keeps only its Status types.
#   - legacy/hcl2shim loses values.go (configschema-coupled); flatmap.go's
#     one dependency on it, UnknownVariableValue, moves to a stub.
#   - state_string.go (debug rendering, legacy/hcl2shim-coupled) is dropped.
# ---------------------------------------------------------------------------
for d in "$STATEFILE_DIR" "$STATES_DIR" "$ADDRS_DIR" "$TFDIAGS_DIR" \
         "$MARKS_DIR" "$CHECKS_DIR" "$LEGACY_HCL2SHIM_DIR" "$VERSION_DIR"; do
  rm -rf "$d"
  mkdir -p "$d"
done

# states: root files only (statefile/ and statemgr/ are subdirectories; only
# statefile is vendored, separately below).
find "$STATES_SRC" -maxdepth 1 -type f -name '*.go' -exec cp {} "$STATES_DIR"/ \;
cp -R "$STATEFILE_SRC"/. "$STATEFILE_DIR"/
cp -R "$ADDRS_SRC"/. "$ADDRS_DIR"/
cp -R "$TFDIAGS_SRC"/. "$TFDIAGS_DIR"/
cp -R "$MARKS_SRC"/. "$MARKS_DIR"/
cp "$CHECKS_SRC/status.go" "$CHECKS_SRC/status_string.go" "$CHECKS_DIR"/
cp "$LEGACY_HCL2SHIM_SRC/flatmap.go" "$LEGACY_HCL2SHIM_SRC/doc.go" "$LEGACY_HCL2SHIM_DIR"/
cp "$VERSION_SRC/version.go" "$VERSION_SRC/VERSION" "$VERSION_DIR"/

for d in "$STATEFILE_DIR" "$STATES_DIR" "$ADDRS_DIR" "$TFDIAGS_DIR" "$MARKS_DIR"; do
  find "$d" -type f -name '*_test.go' -delete
  rm -rf "$d/testdata"
done

# states surgery: drop the debug renderer and the two dependency tails.
rm -f "$STATES_DIR/state_string.go"
perl -i -0pe '
  s|\n// ProviderRequirements[^\n]*\n(//[^\n]*\n)*func \(s \*State\) ProviderRequirements\(\) getproviders\.Requirements \{.*?\n\}\n||s;
' "$STATES_DIR/state.go"
perl -i -ne 'print unless m|"github\.com/opentofu/opentofu/internal/getproviders"|' "$STATES_DIR/state.go"
perl -i -0pe '
  s|\n// NewCheckResults[^\n]*\n(//[^\n]*\n)*func NewCheckResults\(source \*checks\.State\) \*CheckResults \{.*?\n\}\n||s;
' "$STATES_DIR/checks.go"
perl -i -0pe '
  s|\n// RecordCheckResults[^\n]*\n(//[^\n]*\n)*func \(s \*SyncState\) RecordCheckResults\(checkState \*checks\.State\) \{.*?\n\}\n||s;
' "$STATES_DIR/sync.go"
perl -i -ne 'print unless m|"github\.com/opentofu/opentofu/internal/checks"|' "$STATES_DIR/sync.go"

# legacy/hcl2shim: flatmap.go references UnknownVariableValue, declared in the
# dropped values.go; restate the constant in a stub.
cat > "$LEGACY_HCL2SHIM_DIR/values_stub.go" <<'EOF'
// Copyright 2026, Pulumi Corporation.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.

// Stub for the dropped values.go, whose remaining functions depend on
// internal/configs/configschema; flatmap.go only needs this constant.

package hcl2shim

// UnknownVariableValue is a sentinel value that can be used to denote
// that the value of a variable is unknown at this time, matching the
// value and meaning of the constant of the same name upstream.
const UnknownVariableValue = "74D93920-ED26-11E3-AC10-0800200C9A66"
EOF

# Rewrite import paths. Order matters: statefile before states (path prefix),
# and the specific internal/... packages before any broader rewrites.
find "$STATEFILE_DIR" "$STATES_DIR" "$ADDRS_DIR" "$TFDIAGS_DIR" "$MARKS_DIR" \
     "$CHECKS_DIR" "$LEGACY_HCL2SHIM_DIR" -type f -name '*.go' -print0 \
  | xargs -0 perl -pi \
      -e "s|\"$UPSTREAM_STATEFILE_PKG\"|\"$VENDORED_STATEFILE_PKG\"|g;" \
      -e "s|\"$UPSTREAM_STATES_PKG\"|\"$VENDORED_STATES_PKG\"|g;" \
      -e "s|\"$UPSTREAM_ADDRS_PKG\"|\"$VENDORED_ADDRS_PKG\"|g;" \
      -e "s|\"$UPSTREAM_TFDIAGS_PKG\"|\"$VENDORED_TFDIAGS_PKG\"|g;" \
      -e "s|\"$UPSTREAM_MARKS_PKG\"|\"$VENDORED_MARKS_PKG\"|g;" \
      -e "s|\"$UPSTREAM_CHECKS_PKG\"|\"$VENDORED_CHECKS_PKG\"|g;" \
      -e "s|\"$UPSTREAM_LEGACY_HCL2SHIM_PKG\"|\"$VENDORED_LEGACY_HCL2SHIM_PKG\"|g;" \
      -e "s|\"$UPSTREAM_GETMODULES_PKG\"|\"$VENDORED_GETMODULES_PKG\"|g;" \
      -e "s|\"$UPSTREAM_ENCRYPTION_PKG\"|\"$SHIM_ENCRYPTION_PKG\"|g;" \
      -e "s|\"$UPSTREAM_CONFIGS_PKG\"|\"$SHIM_CONFIGS_PKG\"|g;" \
      -e "s|\"$UPSTREAM_VERSION_PKG\"|\"$VENDORED_VERSION_PKG\"|g;" \
      -e "s|tfversion \"$UPSTREAM_VERSION_PKG\"|tfversion \"$VENDORED_VERSION_PKG\"|g;"

gofmt -w "$STATEFILE_DIR" "$STATES_DIR" "$ADDRS_DIR" "$TFDIAGS_DIR" \
         "$MARKS_DIR" "$CHECKS_DIR" "$LEGACY_HCL2SHIM_DIR" "$VERSION_DIR"

# Sanity check: the vendored trees must build against their shims.
echo "regen: go build ./vendored/..." >&2
(cd "$VENDORED_DIR/.." && go build ./vendored/...)

# Summary.
comm_count=$(find "$COMMUNICATOR_DIR" -type f | wc -l | tr -d ' ')
gm_count=$(find "$GETMODULES_DIR" -type f | wc -l | tr -d ' ')
copy_count=$(find "$COPY_DIR" -type f | wc -l | tr -d ' ')
ipaddr_count=$(find "$IPADDR_DIR" -type f | wc -l | tr -d ' ')
hcl2shim_count=$(find "$HCL2SHIM_DIR" -type f | wc -l | tr -d ' ')
state_count=$(find "$STATEFILE_DIR" "$STATES_DIR" "$ADDRS_DIR" "$TFDIAGS_DIR" "$MARKS_DIR" "$CHECKS_DIR" "$LEGACY_HCL2SHIM_DIR" "$VERSION_DIR" -type f | wc -l | tr -d ' ')
echo "regen: wrote $comm_count files under vendored/communicator/, $gm_count under vendored/getmodules/, $copy_count under vendored/copy/, $ipaddr_count under vendored/ipaddr/, $hcl2shim_count under vendored/hcl2shim/, $state_count under the state-file trees" >&2
