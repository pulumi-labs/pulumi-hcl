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

MODULE="github.com/pulumi-labs/pulumi-hcl"
UPSTREAM_MODULE="github.com/opentofu/opentofu"
UPSTREAM_COMMUNICATOR_PKG="$UPSTREAM_MODULE/internal/communicator"
UPSTREAM_PROVISIONERS_PKG="$UPSTREAM_MODULE/internal/provisioners"
UPSTREAM_LOGGING_PKG="$UPSTREAM_MODULE/internal/logging"

VENDORED_COMMUNICATOR_PKG="$MODULE/vendored/communicator"
VENDORED_PROVISIONERS_PKG="$MODULE/pkg/provisioner/provisioners"
# shared/ lives outside the vendored tree because we replace the upstream
# implementation (which depended on configschema) with a smaller in-tree one.
VENDORED_SHARED_PKG="$MODULE/pkg/provisioner/communicator/shared"

# Stash the SHA we're regenerating against; useful for CI diagnostics.
echo "regen: target SHA $SHA" >&2

# Fetch upstream tarball into a temp directory.
TMPDIR="$(mktemp -d)"
trap 'rm -rf "$TMPDIR"' EXIT

TARBALL="$TMPDIR/opentofu.tar.gz"
URL="https://codeload.github.com/opentofu/opentofu/tar.gz/$SHA"
echo "regen: downloading $URL" >&2
curl --fail --silent --show-error --location --output "$TARBALL" "$URL"

# Extract only the subtree we care about.
EXTRACT_ROOT="$TMPDIR/extract"
mkdir -p "$EXTRACT_ROOT"
# The tarball top-level is opentofu-<SHA>/. Strip it.
tar -xzf "$TARBALL" -C "$EXTRACT_ROOT" --strip-components=1 \
  "opentofu-$SHA/internal/communicator"

SRC="$EXTRACT_ROOT/internal/communicator"
if [[ ! -d "$SRC" ]]; then
  echo "error: expected $SRC to exist after extraction" >&2
  exit 1
fi

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

# Sanity check: the vendored tree must build against the shim package.
echo "regen: go build ./vendored/communicator/..." >&2
(cd "$VENDORED_DIR/.." && go build ./vendored/communicator/...)

# Summary.
file_count=$(find "$COMMUNICATOR_DIR" -type f | wc -l | tr -d ' ')
echo "regen: wrote $file_count files under vendored/communicator/" >&2
