#!/usr/bin/env python3
"""Slice a ChatGPT-style symm text archive back into source files.

Usage:
    python tools/slice_symm_archive.py symm.txt ./symm
"""
from __future__ import annotations

import argparse
import pathlib
import re

FILE_MARKER = re.compile(r"^---\nFile: (/[^\n]+)\n---\n", re.MULTILINE)


def clean_content(raw: str) -> str:
    lines = raw.rstrip("\n").splitlines()

    while lines and lines[-1] == "---":
        lines.pop()

    return "\n".join(lines).rstrip("\n") + "\n"


def slice_archive(source: pathlib.Path, output: pathlib.Path) -> int:
    text = source.read_text()
    matches = list(FILE_MARKER.finditer(text))

    if not matches:
        raise SystemExit(f"no file markers found in {source}")

    output.mkdir(parents=True, exist_ok=True)

    for index, match in enumerate(matches):
        relative = pathlib.PurePosixPath(match.group(1).lstrip("/"))
        destination = output / pathlib.Path(*relative.parts)
        start = match.end()
        end = matches[index + 1].start() if index + 1 < len(matches) else len(text)
        destination.parent.mkdir(parents=True, exist_ok=True)
        destination.write_text(clean_content(text[start:end]))

    return len(matches)


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("source", type=pathlib.Path)
    parser.add_argument("output", type=pathlib.Path)
    args = parser.parse_args()

    count = slice_archive(args.source, args.output)
    print(f"wrote {count} files to {args.output}")


if __name__ == "__main__":
    main()
