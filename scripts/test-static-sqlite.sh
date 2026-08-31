#!/usr/bin/env bash
set -euo pipefail

root="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
cd "$root"

tmpdir="$(mktemp -d)"
trap 'rm -rf "$tmpdir"' EXIT

failed=0
for edition in default gentoo; do
	args=()
	if [[ "$edition" == gentoo ]]; then
		args=(-tags gentoo)
	fi

	echo "running static Linux amd64 database tests ($edition)"
	if ! CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go test -count=1 "${args[@]}" ./internal/database; then
		failed=1
	fi

	echo "cross-compiling static Linux arm64 database tests ($edition)"
	if ! CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go test -c "${args[@]}" -o "$tmpdir/database-$edition-arm64.test" ./internal/database; then
		failed=1
	fi
done

exit "$failed"
