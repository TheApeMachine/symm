#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

export GOFLAGS="${GOFLAGS:--ldflags=-checklinkname=0}"
export GOWORK="${GOWORK:-$ROOT/go.work}"

GO_MODULES=(
	"$ROOT/../datura"
	"$ROOT/../nomagique"
	"$ROOT"
)

require_path() {
	local path="$1"
	local description="$2"

	if [[ ! -e "$path" ]]; then
		printf 'missing %s: %s\n' "$description" "$path" >&2
		exit 1
	fi
}

check_gofmt() {
	local files=()
	local module

	for module in "${GO_MODULES[@]}"; do
		require_path "$module/go.mod" "Go module"
		while IFS= read -r -d '' file; do
			files+=("$file")
		done < <(find "$module" -name '*.go' -not -path '*/.git/*' -not -path '*/vendor/*' -print0)
	done

	if ((${#files[@]} == 0)); then
		printf 'no Go files found\n' >&2
		exit 1
	fi

	local unformatted
	unformatted="$(gofmt -l "${files[@]}")"
	if [[ -n "$unformatted" ]]; then
		printf 'gofmt required:\n%s\n' "$unformatted" >&2
		exit 1
	fi
}

go_test_modules() {
	local args=("$@")
	local module

	for module in "${GO_MODULES[@]}"; do
		(
			cd "$module"
			go test "${args[@]}" ./...
		)
	done
}

frontend_verify() {
	(
		cd "$ROOT/frontend"
		pnpm install --frozen-lockfile
		pnpm typecheck
		pnpm test
		pnpm lint
		pnpm build
	)
}

check_gofmt
go_test_modules
go_test_modules -race
frontend_verify
