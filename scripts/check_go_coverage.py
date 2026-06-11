#!/usr/bin/env python3
"""Fail when Go coverage is below the configured minimum."""

from __future__ import annotations

import re
import subprocess
import sys
from pathlib import Path


TOTAL_RE = re.compile(r"^total:\s+\(statements\)\s+([0-9]+(?:\.[0-9]+)?)%$")


def main() -> int:
    if len(sys.argv) != 3:
        print("usage: check_go_coverage.py <coverprofile> <min_percent>", file=sys.stderr)
        return 2

    coverprofile = Path(sys.argv[1])
    if not coverprofile.is_file():
        print(f"coverage profile not found: {coverprofile}", file=sys.stderr)
        return 2

    try:
        minimum = float(sys.argv[2])
    except ValueError:
        print(f"invalid min_percent: {sys.argv[2]}", file=sys.stderr)
        return 2

    result = subprocess.run(
        ["go", "tool", "cover", f"-func={coverprofile}"],
        check=False,
        text=True,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
    )
    if result.returncode != 0:
        print(result.stderr, file=sys.stderr, end="")
        return result.returncode

    total = None
    for line in result.stdout.splitlines():
        match = TOTAL_RE.match(line.strip())
        if match:
            total = float(match.group(1))
            break

    if total is None:
        print("could not find total coverage in go tool cover output", file=sys.stderr)
        return 2

    print(f"go coverage total: {total:.1f}% (minimum {minimum:.1f}%)")
    if total + 1e-9 < minimum:
        print(f"go coverage below minimum: {total:.1f}% < {minimum:.1f}%", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
