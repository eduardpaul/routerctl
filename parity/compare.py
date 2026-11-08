"""Compare two canonical parity documents (Python vs Go) source by source.

    python parity/compare.py py.json go.json

Prints a scoreboard and the first differences per source; exits non-zero
when any source differs. Numbers compare with a 1e-6 relative tolerance.
"""

from __future__ import annotations

import json
import math
from pathlib import Path
import sys
from typing import Any


def diff(
    path: str, want: Any, got: Any, out: list[str], limit: int = 8
) -> None:
    """Append the differences between `want` and `got` to `out`.

    Stops once `out` holds `limit` lines.
    """

    if len(out) >= limit:
        return
    if isinstance(want, dict) and isinstance(got, dict):
        _diff_dict(path, want, got, out, limit)
    elif isinstance(want, list) and isinstance(got, list):
        _diff_list(path, want, got, out, limit)
    elif _is_number(want) and _is_number(got):
        if not math.isclose(want, got, rel_tol=1e-6, abs_tol=1e-9):
            out.append(f"{path}: want {want}, got {got}")
    elif want != got or type(want) is not type(got):
        out.append(f"{path}: want {short(want)}, got {short(got)}")


def _diff_dict(
    path: str,
    want: dict[str, Any],
    got: dict[str, Any],
    out: list[str],
    limit: int,
) -> None:
    for key in sorted(set(want) | set(got)):
        if key not in got:
            out.append(
                f"{path}.{key}: missing in Go (want {short(want[key])})"
            )
        elif key not in want:
            out.append(f"{path}.{key}: extra in Go ({short(got[key])})")
        else:
            diff(f"{path}.{key}", want[key], got[key], out, limit)


def _diff_list(
    path: str, want: list[Any], got: list[Any], out: list[str], limit: int
) -> None:
    if len(want) != len(got):
        out.append(f"{path}: want {len(want)} items, got {len(got)}")
    for i, (w, g) in enumerate(zip(want, got)):
        diff(f"{path}[{i}]", w, g, out, limit)


def _is_number(value: Any) -> bool:
    """Int or float, but not bool (a bool is an int in Python)."""

    return isinstance(value, (int, float)) and not isinstance(value, bool)


def short(value: Any) -> str:
    """Render a value as JSON, cut to 100 characters."""

    text = json.dumps(value, ensure_ascii=False)
    return text if len(text) <= 100 else text[:100] + "..."


def main() -> int:
    """Compare the two documents named on the command line."""

    with Path(sys.argv[1]).open(encoding="utf-8") as fh:
        py = json.load(fh)
    with Path(sys.argv[2]).open(encoding="utf-8") as fh:
        go = json.load(fh)
    failed = 0
    for name in sorted(set(py) | set(go)):
        out: list[str] = []
        if name not in go:
            out.append("source missing in Go")
        elif name not in py:
            out.append("source missing in Python")
        else:
            diff("$", py[name], go[name], out)
        status = "ok  " if not out else "DIFF"
        failed += bool(out)
        print(f"{status} {name}")
        for line in out:
            print(f"       {line}")
    total = len(set(py) | set(go))
    print(f"\n{total - failed}/{total} sources match")
    return 1 if failed else 0


if __name__ == "__main__":
    raise SystemExit(main())
