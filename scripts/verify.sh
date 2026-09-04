#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

export GOFLAGS="${GOFLAGS:--ldflags=-checklinkname=0}"
if [[ -f "$ROOT/go.work" ]]; then
	export GOWORK="${GOWORK:-$ROOT/go.work}"
else
	export GOWORK=off
fi

AVAILABLE_MODULES=()
for mod in "$ROOT/../datura" "$ROOT/../nomagique" "$ROOT"; do
	if [[ -f "$mod/go.mod" ]]; then
		AVAILABLE_MODULES+=("$mod")
	fi
done

GO_MODULES=("${AVAILABLE_MODULES[@]}")

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
		done < <(find "$module" -name '*.go' -not -path '*/.git/*' -not -path '*/vendor/*' -not -path '*/generated/*' -print0)
	done

	if ((${#files[@]} == 0)); then
		printf 'no Go files found\n' >&2
		return 1
	fi

	local unformatted
	unformatted="$(gofmt -l "${files[@]}")"
	if [[ -n "$unformatted" ]]; then
		printf 'gofmt required:\n%s\n' "$unformatted" >&2
		return 1
	fi
	return 0
}

go_test_modules() {
	local args=("$@")
	local module
	local status=0

	for module in "${GO_MODULES[@]}"; do
		(
			cd "$module"
			go test "${args[@]}" ./...
		) || status=$?
	done

	return $status
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

CMD="${1:-all}"
case "$CMD" in
	fmt)
		check_gofmt
		;;
	test)
		go_test_modules
		;;
	race)
		go_test_modules -race
		;;
	frontend)
		frontend_verify
		;;
	all)
		FAILED=()
		if ! check_gofmt; then
			FAILED+=("gofmt")
		fi
		if ! go_test_modules; then
			FAILED+=("go_test")
		fi
		if ! go_test_modules -race; then
			FAILED+=("go_race")
		fi
		if ! frontend_verify; then
			FAILED+=("frontend")
		fi

		if ((${#FAILED[@]} > 0)); then
			printf 'Verification failures in: %s\n' "${FAILED[*]}" >&2
			exit 1
		fi
		;;
	*)
		printf 'unknown command: %s\n' "$CMD" >&2
		exit 1
		;;
esac
