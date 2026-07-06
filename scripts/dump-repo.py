#!/usr/bin/env python3
"""
Write a single text file aggregating repository source files for offline review.

Includes only selected extensions, skips Go tests and TypeScript declaration
files (*.d.ts), skips dotfiles/dotdirs, named noise files, and common
dependency and build artifact directories.
"""

from __future__ import annotations

import argparse
import os
import sys
from pathlib import Path


SKIP_DIRECTORY_NAMES: frozenset[str] = frozenset(
    {
        "__pycache__",
        "build",
        "coverage",
        "dist",
        "node_modules",
        "target",
        "vendor",
        "venv",
        ".claude",
        ".cursor",
        ".vscode",
        ".idea",
        ".pytest_cache",
        ".ruff_cache",
        ".mypy_cache",
        ".venv",
        ".pnpm-store",
        "bin",
        "runs",
        "scripts",
    }
)

SKIP_FILE_NAMES: frozenset[str] = frozenset(
    {
        "routeTree.gen.ts",
        "vite.config.ts",
        "_test.go",
        ".test.ts",
        ".test.tsx"
    }
)

ALLOWED_SUFFIXES: tuple[str, ...] = (
    ".go",
    ".ts",
    ".tsx",
    ".yaml",
    ".yml",
    ".metal",
    ".mm",
    ".m",
    ".h"
)


def repository_root(start: Path) -> Path:
    current = start.resolve()

    for candidate in (current, *current.parents):
        if (candidate / ".git").exists():
            return candidate

    return current


def collect_relative_paths(root: Path) -> list[str]:
    matches: list[str] = []

    for dir_path, dir_names, file_names in os.walk(root, topdown=True):
        dir_names[:] = sorted(
            directory_name
            for directory_name in dir_names
            if not directory_name.startswith(".")
            and directory_name not in SKIP_DIRECTORY_NAMES
        )

        for file_name in sorted(file_names):
            if file_name.startswith("."):
                continue

            if file_name in SKIP_FILE_NAMES:
                continue

            if not file_name.endswith(ALLOWED_SUFFIXES):
                continue

            if file_name.endswith(".d.ts"):
                continue

            if file_name.endswith("_test.go"):
                continue

            absolute_file = Path(dir_path) / file_name
            relative_file = absolute_file.relative_to(root)

            matches.append(relative_file.as_posix())

    matches.sort()
    return matches


def path_matches_prefix(relative_posix: str, prefix: str) -> bool:
    normalized_prefix = prefix.rstrip("/")

    if relative_posix == normalized_prefix:
        return True

    return relative_posix.startswith(f"{normalized_prefix}/")


def filter_relative_paths(
    paths: list[str],
    include_prefixes: list[str],
    exclude_prefixes: list[str],
) -> list[str]:
    filtered_paths = paths

    if include_prefixes:
        filtered_paths = [
            relative_posix
            for relative_posix in filtered_paths
            if any(
                path_matches_prefix(relative_posix, prefix)
                for prefix in include_prefixes
            )
        ]

    if exclude_prefixes:
        filtered_paths = [
            relative_posix
            for relative_posix in filtered_paths
            if not any(
                path_matches_prefix(relative_posix, prefix)
                for prefix in exclude_prefixes
            )
        ]

    return filtered_paths


def build_path_trie(paths: list[str]) -> dict[str, dict[str, object] | None]:
    trie: dict[str, dict[str, object] | None] = {}

    for posix_path in paths:
        components = posix_path.split("/")
        node = trie

        for index, component in enumerate(components):
            is_leaf = index == len(components) - 1

            if component not in node:
                node[component] = None if is_leaf else {}

            entry = node[component]

            if is_leaf:
                break

            assert isinstance(entry, dict)
            node = entry

    return trie


def format_directory_tree(trie: dict[str, dict[str, object] | None]) -> str:
    lines: list[str] = ["Directory Structure:", "", "└── ./"]

    def walk(node: dict[str, dict[str, object] | None], prefix: str) -> None:
        keys = sorted(node.keys())

        for index, name in enumerate(keys):
            is_last = index == len(keys) - 1
            branch = "└── " if is_last else "├── "
            lines.append(f"{prefix}{branch}{name}")
            child = node[name]

            if isinstance(child, dict) and child:
                extension = "    " if is_last else "│   "
                walk(child, prefix + extension)

    walk(trie, "    ")
    return "\n".join(lines)


def file_delimiter(display_path: str) -> str:
    return f"\n---\n\n---\nFile: {display_path}\n---\n\n"


def write_aggregate(output_path: Path, tree_text: str, root: Path, paths: list[str]) -> None:
    with output_path.open("w", encoding="utf-8", newline="\n") as sink:
        sink.write(tree_text)
        sink.write("\n\n")

        for index, relative_posix in enumerate(paths):
            display_path = f"/{relative_posix}"
            sink.write(file_delimiter(display_path))

            absolute_path = root / relative_posix
            raw_bytes = absolute_path.read_bytes()

            try:
                contents = raw_bytes.decode("utf-8")
            except UnicodeDecodeError as decode_error:
                raise RuntimeError(
                    f"dump-repo: cannot decode UTF-8 for {absolute_path}: {decode_error}"
                ) from decode_error

            sink.write(contents)

            if index < len(paths) - 1 and not contents.endswith("\n"):
                sink.write("\n")


def resolve_repository_root(explicit_root: Path | None) -> Path:
    if explicit_root is None:
        return repository_root(Path.cwd())

    resolved_root = explicit_root.resolve()

    if not (resolved_root / ".git").exists():
        raise ValueError(f"dump-repo: not a git repository: {resolved_root}")

    return resolved_root


def parse_arguments() -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description="Aggregate repository source files into a single text dump.",
    )
    parser.add_argument(
        "output",
        nargs="?",
        default="code-dump.txt",
        help="Output file path (default: code-dump.txt)",
    )
    parser.add_argument(
        "repo_root",
        nargs="?",
        default=None,
        help="Repository root (default: git root from current directory)",
    )
    parser.add_argument(
        "--include-prefix",
        action="append",
        default=[],
        dest="include_prefixes",
        metavar="PREFIX",
        help="Include only paths under PREFIX (repeatable)",
    )
    parser.add_argument(
        "--exclude-prefix",
        action="append",
        default=[],
        dest="exclude_prefixes",
        metavar="PREFIX",
        help="Exclude paths under PREFIX (repeatable)",
    )

    return parser.parse_args()


def main() -> int:
    arguments = parse_arguments()
    output_path = Path(arguments.output)

    if output_path.exists() and output_path.is_dir():
        print(f"dump-repo: output path {output_path} is a directory", file=sys.stderr)

        return 1

    explicit_root = Path(arguments.repo_root) if arguments.repo_root else None

    try:
        root = resolve_repository_root(explicit_root)
    except ValueError as value_error:
        print(value_error, file=sys.stderr)

        return 1

    paths = collect_relative_paths(root)
    paths = filter_relative_paths(
        paths,
        arguments.include_prefixes,
        arguments.exclude_prefixes,
    )

    if not paths:
        print("dump-repo: no matching files found", file=sys.stderr)

        return 1

    trie = build_path_trie(paths)
    tree_text = format_directory_tree(trie)
    write_aggregate(output_path.resolve(), tree_text, root, paths)

    print(f"dump-repo: wrote {len(paths)} files to {output_path.resolve()}")

    return 0


if __name__ == "__main__":
    raise SystemExit(main())
