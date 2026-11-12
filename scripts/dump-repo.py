#!/usr/bin/env python3
"""
Write a single text file aggregating repository source files for offline review.

Includes only selected extensions, skips Go tests and TypeScript declaration
files (*.d.ts), skips dotfiles/dotdirs, named noise files, and common
dependency and build artifact directories.

Also appends the local replace module github.com/theapemachine/nomagique,
but only packages that appear in this project's go list -deps closure.
"""

from __future__ import annotations

import argparse
import os
import subprocess
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
        ".github",
        "bin",
        "runs",
        "scripts",
        "telemetry",
        "specs",
        "tests",
        "advisor",
        "tools",
        "logic",
        "fluid-3d",
        "influence",
        "hindsight",
        "frontend",
    }
)

SKIP_FILE_NAMES: frozenset[str] = frozenset(
    {
        "routeTree.gen.ts",
        "vite.config.ts",
        ".test.ts",
        ".test.tsx",
        ".bench.ts",
        ".pnpm-lock.yaml",
        "AGENTS.md",
        "pnpm-lock.yaml",
        ".md",
    }
)

ALLOWED_SUFFIXES: tuple[str, ...] = (
    ".go",
    ".ts",
    ".tsx",
    ".yml",
    ".metal",
    ".mm",
    ".m",
    ".h",
    "cuh",
    ".cu",
    ".inc",
)

NOMAGIQUE_MODULE = "github.com/theapemachine/nomagique"


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

            if any(skip_file_name in file_name for skip_file_name in SKIP_FILE_NAMES):
                continue

            if not file_name.endswith(ALLOWED_SUFFIXES):
                continue

            if file_name.endswith(".d.ts"):
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


def run_go_list(root: Path, *args: str) -> str:
    completed = subprocess.run(
        ["go", "list", *args],
        cwd=root,
        check=False,
        capture_output=True,
        text=True,
    )

    if completed.returncode != 0:
        detail = completed.stderr.strip() or completed.stdout.strip() or "go list failed"
        raise RuntimeError(f"dump-repo: {detail}")

    return completed.stdout


def nomagique_module_dir(root: Path) -> Path | None:
    try:
        output = run_go_list(root, "-m", "-f", "{{.Dir}}", NOMAGIQUE_MODULE)
    except RuntimeError:
        return None

    line = output.strip().splitlines()
    if not line:
        return None

    path = Path(line[0].strip())
    if not path.is_dir():
        return None

    return path.resolve()


def used_nomagique_package_dirs(root: Path) -> list[Path]:
    template = (
        "{{if .Module}}"
        "{{if eq .Module.Path \"" + NOMAGIQUE_MODULE + "\"}}"
        "{{.Dir}}"
        "{{end}}"
        "{{end}}"
    )

    output = run_go_list(root, "-deps", "-f", template, "./...")
    dirs = sorted(
        {
            Path(line.strip()).resolve()
            for line in output.splitlines()
            if line.strip()
        }
    )

    return [path for path in dirs if path.is_dir()]


def collect_nomagique_dump_files(
    root: Path,
) -> list[tuple[str, Path]]:
    module_dir = nomagique_module_dir(root)
    if module_dir is None:
        print(
            f"dump-repo: {NOMAGIQUE_MODULE} not on the module graph; skipping",
            file=sys.stderr,
        )
        return []

    files: list[tuple[str, Path]] = []

    for package_dir in used_nomagique_package_dirs(root):
        try:
            relative_dir = package_dir.relative_to(module_dir)
        except ValueError:
            continue

        for entry in sorted(package_dir.iterdir()):
            if not entry.is_file():
                continue

            name = entry.name
            if name.startswith("."):
                continue

            if not name.endswith(".go"):
                continue

            if name.endswith("_test.go"):
                continue

            if relative_dir.as_posix() == ".":
                display = f"nomagique/{name}"
            else:
                display = f"nomagique/{relative_dir.as_posix()}/{name}"

            files.append((display, entry.resolve()))

    files.sort(key=lambda item: item[0])
    return files


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


def write_aggregate(
    output_path: Path,
    tree_text: str,
    entries: list[tuple[str, Path]],
) -> None:
    with output_path.open("w", encoding="utf-8", newline="\n") as sink:
        sink.write(tree_text)
        sink.write("\n\n")

        for index, (display_path, absolute_path) in enumerate(entries):
            sink.write(file_delimiter(f"/{display_path}"))

            raw_bytes = absolute_path.read_bytes()

            try:
                contents = raw_bytes.decode("utf-8")
            except UnicodeDecodeError as decode_error:
                raise RuntimeError(
                    f"dump-repo: cannot decode UTF-8 for {absolute_path}: {decode_error}"
                ) from decode_error

            sink.write(contents)

            if index < len(entries) - 1 and not contents.endswith("\n"):
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
    parser.add_argument(
        "--skip-nomagique",
        action="store_true",
        help="Do not append used nomagique packages",
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

    relative_paths = collect_relative_paths(root)
    relative_paths = filter_relative_paths(
        relative_paths,
        arguments.include_prefixes,
        arguments.exclude_prefixes,
    )

    entries: list[tuple[str, Path]] = [
        (relative_posix, (root / relative_posix).resolve())
        for relative_posix in relative_paths
    ]

    if not arguments.skip_nomagique:
        try:
            nomagique_entries = collect_nomagique_dump_files(root)
        except RuntimeError as runtime_error:
            print(runtime_error, file=sys.stderr)
            return 1

        entries.extend(nomagique_entries)

    if not entries:
        print("dump-repo: no matching files found", file=sys.stderr)

        return 1

    display_paths = [display for display, _ in entries]
    trie = build_path_trie(display_paths)
    tree_text = format_directory_tree(trie)
    write_aggregate(output_path.resolve(), tree_text, entries)

    nomagique_count = sum(1 for display, _ in entries if display.startswith("nomagique/"))
    print(
        f"dump-repo: wrote {len(entries)} files"
        f" ({nomagique_count} nomagique) to {output_path.resolve()}"
    )

    return 0


if __name__ == "__main__":
    raise SystemExit(main())
