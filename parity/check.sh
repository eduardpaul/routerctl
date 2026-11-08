#!/usr/bin/env bash
# Replay recorded sessions through the Python and the Go library and
# compare their canonical output source by source.
#
#   parity/check.sh [session.jsonl ...]
#
# Default: every parity/{local,fixtures}/*/session.jsonl that already has a
# py.json golden next to it, i.e. the ASUS sessions this harness has seen
# (the same set the Go replay tests use). Pass a new session explicitly
# once to create its py.json.
set -euo pipefail
root="$(cd "$(dirname "$0")/.." && pwd)"
cd "$root"
if [ ! -f third_party/asusrouter-py/pyproject.toml ]; then
	echo "the Python library is missing: run git submodule update --init" >&2
	exit 1
fi

sessions=("$@")
if [ ${#sessions[@]} -eq 0 ]; then
	for s in parity/local/*/session.jsonl parity/fixtures/*/session.jsonl; do
		[ -e "$s" ] || continue
		if [ -e "$(dirname "$s")/py.json" ]; then
			sessions+=("$s")
		else
			echo "skipping $s (no py.json yet: pass it explicitly to check it)" >&2
		fi
	done
fi
if [ ${#sessions[@]} -eq 0 ]; then
	echo "no sessions found (record one with: uv run --project parity python parity/py_harness.py capture parity/local/<device>/session.jsonl)" >&2
	exit 1
fi

uv run --project parity python parity/check_layout.py
(cd asusrouter && go build -o "$root/parity/.parity-go" ./cmd/parity)
status=0
for s in "${sessions[@]}"; do
	dir="$(dirname "$s")"
	echo "== $s"
	uv run --project parity python parity/py_harness.py replay "$s" > "$dir/py.json"
	parity/.parity-go replay "$s" > "$dir/go.json"
	uv run --project parity python parity/compare.py "$dir/py.json" "$dir/go.json" || status=1
done
exit $status
