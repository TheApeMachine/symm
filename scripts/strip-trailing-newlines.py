#!/usr/bin/env python3
"""
Remove the last three newline bytes from each file that ends with three or more.

Leaves one trailing newline when the file previously had four (the usual
accidental blank-line run). Files with fewer than three trailing newlines are
unchanged.
"""

from __future__ import annotations

import argparse
import sys
from pathlib import Path


def trailing_newline_count(data: bytes) -> int:
    count = 0

    for byte in reversed(data):
        if byte != ord("\n"):
            break

        count += 1

    return count


def strip_three_trailing_newlines(data: bytes) -> bytes | None:
    trailing = trailing_newline_count(data)

    if trailing < 3:
        return None

    return data[: len(data) - 3]


def process_path(path: Path) -> bool:
    raw = path.read_bytes()
    trimmed = strip_three_trailing_newlines(raw)

    if trimmed is None:
        return False

    path.write_bytes(trimmed)
    return True


def parse_args(argv: list[str]) -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description=(
            "Remove the last three trailing newline bytes when a file has "
            "three or more at EOF."
        ),
    )
    parser.add_argument(
        "paths",
        nargs="*",
        help="Files to process (reads stdin paths when omitted)",
    )

    return parser.parse_args(argv)


def input_paths(args: argparse.Namespace) -> list[Path]:
    if args.paths:
        return [Path(raw) for raw in args.paths]

    return [Path(line.strip()) for line in sys.stdin if line.strip()]


def main(argv: list[str]) -> int:
    args = parse_args(argv)
    changed = 0

    for path in input_paths(args):
        if not path.is_file():
            print(f"strip-trailing-newlines: skip missing file {path}", file=sys.stderr)
            continue

        if process_path(path):
            changed += 1
            print(path)

    return 0


if __name__ == "__main__":
    raise SystemExit(main(sys.argv[1:]))
