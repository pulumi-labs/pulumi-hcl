#!/usr/bin/env bash
#
# Runs one shard of the test suite. The shards partition `go test ./...` by
# construction:
#
#   - unit runs every package except tests/tfcompat and tests/smoke, and
#     skips TestLanguage (which only exists in cmd/pulumi-language-hcl).
#   - conformance-0 runs the TestLanguage subtests whose name ends in
#     s, t, or l -- measured to be ~half the suite's runtime -- and
#     conformance-1 runs the exact complement, so any new subtest lands in
#     exactly one of the two.
#   - conformance-large runs only l2-large-string (excluded from both
#     conformance shards: it ends in "g" so conformance-0 never matches it,
#     and conformance-1 skips it explicitly).
#   - tfcompat-N partitions the package's tests by their position in
#     `go test -list` order, so every test runs in exactly one shard.
#   - smoke runs tests/smoke.
set -euo pipefail

shard="${1:?usage: test-shard.sh <shard>}"

GOTEST=(go test -v -race -coverpkg=./... -coverprofile=coverage.out)

case "$shard" in
  unit)
    # shellcheck disable=SC2046
    "${GOTEST[@]}" -skip '^TestLanguage$' \
      $(go list ./... | grep -v -e '/tests/tfcompat$' -e '/tests/smoke$')
    ;;
  conformance-0)
    "${GOTEST[@]}" -run '^TestLanguage$/[stl]$' ./cmd/pulumi-language-hcl
    ;;
  conformance-1)
    "${GOTEST[@]}" -run '^TestLanguage$' \
      -skip '^TestLanguage$/([stl]$|^l2-large-string$)' ./cmd/pulumi-language-hcl
    ;;
  conformance-large)
    # l2-large-string pushes a ~100MB value through the language host and
    # takes minutes under the race detector. Every code path it exercises
    # also runs race-instrumented on small values in the other conformance
    # shards, so this one value-size stress test runs without -race.
    go test -v -coverpkg=./... -coverprofile=coverage.out \
      -run '^TestLanguage$/^l2-large-string$' ./cmd/pulumi-language-hcl
    ;;
  tfcompat-[0-9])
    index="${shard#tfcompat-}"
    total=3
    if ((index >= total)); then
      echo "unknown shard: $shard (only tfcompat-0..$((total - 1)) exist)" >&2
      exit 1
    fi
    tests="$(go test -list '.*' ./tests/tfcompat |
      grep '^Test' |
      awk -v i="$index" -v n="$total" 'NR % n == i' |
      paste -sd'|' -)"
    "${GOTEST[@]}" -run "^($tests)$" ./tests/tfcompat
    ;;
  smoke)
    "${GOTEST[@]}" ./tests/smoke
    ;;
  *)
    echo "unknown shard: $shard" >&2
    exit 1
    ;;
esac
