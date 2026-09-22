#!/usr/bin/env python3
"""Run the examples from README.md and check their output.

The Usage section documents every tool by example: a `$ command` line followed
by the output that command is supposed to produce. This script extracts those
pairs and runs them, so the README cannot quietly drift away from the tools.

Only fenced blocks inside the `## Usage` section that actually contain a
command prompt are considered, which leaves the installation examples alone.
Within a block a line starting with `#` is a caption, not output, and ends the
example before it.

Commands run with bash in a scratch directory per block, with the repository
root (the two shell scripts) and ./build (where README.md, the test workflow and
the release workflow build yson-convert and ysonq into) on PATH.
"""

from __future__ import annotations

import difflib
import os
import subprocess
import sys
import tempfile
from pathlib import Path

REPO = Path(__file__).resolve().parent.parent
PROMPT = "$ "
SECTION = "Usage"
FENCE = "```"


def blocks(text: str) -> list[str]:
    """The fenced code blocks that belong to the `## Usage` section."""
    found: list[str] = []
    section = ""
    current: list[str] | None = None

    for line in text.splitlines():
        if current is None and line.startswith("## "):
            section = line[3:].strip()
            continue
        if line.startswith(FENCE):
            if current is None:
                current = []
            else:
                if section == SECTION:
                    found.append("\n".join(current))
                current = None
            continue
        if current is not None and section == SECTION:
            current.append(line)

    return found


def examples(block: str) -> list[tuple[str, str]]:
    """The (command, expected output) pairs of one block."""
    found: list[list | None] = []

    for line in block.splitlines():
        if line.startswith(PROMPT):
            found.append([line[len(PROMPT) :], []])
        elif line.startswith("#"):
            # a caption such as "# Compact form"; it separates two examples
            found.append(None)
        elif found and found[-1] is not None:
            found[-1][1].append(line)

    return [(pair[0], "\n".join(pair[1]).rstrip()) for pair in found if pair is not None]


def main() -> int:
    readme = Path(sys.argv[1]) if len(sys.argv) > 1 else REPO / "README.md"
    by_block = [examples(block) for block in blocks(readme.read_text())]
    total = sum(len(e) for e in by_block)

    if not total:
        print(f"no examples found in {readme}, is the {SECTION} section there?", file=sys.stderr)
        return 1

    env = dict(os.environ)
    env["PATH"] = os.pathsep.join([str(REPO), str(REPO / "build"), env.get("PATH", "")])

    failures = 0
    for pairs in by_block:
        # one scratch directory per block: the yson-format example creates the
        # file it formats, and nothing should leak between blocks
        with tempfile.TemporaryDirectory() as scratch:
            for cmd, expected in pairs:
                proc = subprocess.run(
                    ["bash", "-c", cmd], cwd=scratch, env=env, capture_output=True, text=True
                )
                actual = proc.stdout.rstrip()

                if proc.returncode == 0 and actual == expected:
                    print(f"ok    {cmd}")
                    continue

                failures += 1
                print(f"FAIL  {cmd}")
                if proc.returncode != 0:
                    print(f"      exit {proc.returncode}: {proc.stderr.strip()}")
                    if proc.returncode == 127:
                        print("      (is the tool built? see ./build in README.md)")
                for line in difflib.unified_diff(
                    expected.splitlines(),
                    actual.splitlines(),
                    fromfile="README.md",
                    tofile="actual",
                    lineterm="",
                ):
                    print(f"      {line}")

    print(f"\n{total - failures}/{total} examples match")
    return 1 if failures else 0


if __name__ == "__main__":
    sys.exit(main())
