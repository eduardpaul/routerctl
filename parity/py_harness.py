"""Python side of the Python <-> Go parity harness.

Two commands:

  capture  Connect to a live device, fetch every default source once and
           record every wire query (endpoint, payload, status, body) into
           a session file (JSON lines).
  replay   Re-run the same pipeline offline against a session file and
           print every source result in the canonical JSON form.

The Go port has the same seam (`Transport.Query`) and the same canonical
form, so `go run ./cmd/parity replay <session>` must print the very same
document. `parity/check.sh` diffs the two.

Read-only: the harness never calls actions; sources are fetched with
their defaults (no `refresh`), exactly like a normal poll.
"""

from __future__ import annotations

import argparse
import asyncio
import base64
import dataclasses
from datetime import date, datetime, time, timedelta
import enum
import ipaddress
import json
import math
import os
from pathlib import Path
import re
import sys
from typing import Any

from asusrouter import AsusRouter
from asusrouter.connection import Connection
from asusrouter.error import AsusRouter404Error
from asusrouter.tools.security.sensitive import ARSensitive, Sensitive

# Fixed instant used for replay so time-derived values are deterministic
FROZEN_NOW = datetime(2026, 1, 1, 12, 0, 0, tzinfo=None)


# ---------------------------------------------------------------------------
# Canonical form
# ---------------------------------------------------------------------------


def _key(value: Any) -> str:
    """Render a mapping key as a string."""

    canon = canonical(value)
    if isinstance(canon, str):
        return canon
    if isinstance(canon, bool):
        return "true" if canon else "false"
    if canon is None:
        return "null"
    return json.dumps(canon, sort_keys=True, separators=(",", ":"))


def _number(value: float) -> Any:
    """Normalize a float: integral -> int, else rounded to 6 places."""

    if math.isnan(value) or math.isinf(value):
        return str(value)
    if value == int(value) and abs(value) < 1e15:
        return int(value)
    return round(value, 6)


def _isoformat(value: datetime | date | time) -> str:
    return value.isoformat()


def _public_properties(obj: Any) -> dict[str, Any]:
    """Collect public properties and attributes of a plain object."""

    out: dict[str, Any] = {}
    for name in dir(type(obj)):
        if name.startswith("_"):
            continue
        attr = getattr(type(obj), name, None)
        if isinstance(attr, property):
            try:
                out[name] = getattr(obj, name)
            except Exception:  # noqa: BLE001
                out[name] = None
    for name, val in getattr(obj, "__dict__", {}).items():
        if not name.startswith("_") and not callable(val):
            out.setdefault(name, val)
    return out


def canonical(value: Any) -> Any:  # noqa: C901, PLR0911, PLR0912
    """Convert any library value into the canonical JSON-able form.

    Rules (mirrored in Go `parity/canonical`):
      - enums -> their value (str or int)
      - sensitive identifiers (MAC, IP, SSID, ...) -> their raw str
      - dataclass -> object of its fields; NamedTuple -> object of fields
      - other objects -> object of public properties/attributes
      - datetime/date/time -> isoformat; timedelta -> seconds
      - mapping -> object with stringified, sorted keys
      - list/tuple -> list; set -> sorted list
      - float -> int when integral, else rounded to 6 places
    """

    if value is None or isinstance(value, (bool, str)):
        return value
    if isinstance(value, enum.Enum):
        return canonical(value.value)
    if isinstance(value, int):
        return value
    if isinstance(value, float):
        return _number(value)
    if isinstance(value, Sensitive):
        return canonical(value.value)
    if isinstance(value, ARSensitive):
        # Reveal the raw value: parity compares data, not its redaction
        if isinstance(getattr(type(value), "value", None), property):
            return canonical(value.value)
        return str(value)
    if isinstance(value, (datetime, date, time)):
        return _isoformat(value)
    if isinstance(value, timedelta):
        return _number(value.total_seconds())
    if isinstance(
        value,
        (
            ipaddress.IPv4Address,
            ipaddress.IPv6Address,
            ipaddress.IPv4Interface,
            ipaddress.IPv6Interface,
            ipaddress.IPv4Network,
            ipaddress.IPv6Network,
        ),
    ):
        return str(value)
    if isinstance(value, (bytes, bytearray)):
        return base64.b64encode(bytes(value)).decode("ascii")
    if dataclasses.is_dataclass(value) and not isinstance(value, type):
        return {
            f.name: canonical(getattr(value, f.name))
            for f in dataclasses.fields(value)
            if not f.name.startswith("_")
        }
    if isinstance(value, tuple) and hasattr(value, "_asdict"):
        return {k: canonical(v) for k, v in value._asdict().items()}
    if isinstance(value, dict):
        return {
            _key(k): canonical(v)
            for k, v in sorted(value.items(), key=lambda kv: _key(kv[0]))
        }
    if isinstance(value, (list, tuple)):
        return [canonical(v) for v in value]
    if isinstance(value, (set, frozenset)):
        return sorted(
            (canonical(v) for v in value),
            key=lambda v: json.dumps(v, sort_keys=True),
        )
    return {
        k: canonical(v) for k, v in sorted(_public_properties(value).items())
    }


def dumps(value: Any) -> str:
    """Serialize a canonical value deterministically."""

    return json.dumps(
        canonical(value), sort_keys=True, indent=1, ensure_ascii=False
    )


# ---------------------------------------------------------------------------
# Session capture / replay at the Connection.async_query seam
# ---------------------------------------------------------------------------


_TS = re.compile(r"(^|&)ts=\d+")


def normalize_payload(payload: str | None) -> str | None:
    """Normalize a payload for matching (mirrored in Go `parity`).

    - `ts=<unix seconds>` diagnostics stamps -> `ts=*`
    - `hook=` lists are order-insensitive -> sorted by item
    """

    if payload is None:
        return None
    payload = _TS.sub(r"\1ts=*", payload)
    if payload.startswith("hook="):
        items = payload[len("hook=") :].split(";")
        payload = "hook=" + ";".join(sorted(items))
    return payload


class Session:
    """Recorded wire queries keyed by (endpoint, payload).

    Replayed in recording order; the last answer repeats.
    """

    def __init__(self, records: list[dict[str, Any]]) -> None:
        """Index the records by endpoint and normalized payload."""

        self._queues: dict[tuple[str, str | None], list[dict[str, Any]]] = {}
        for rec in records:
            key = (rec["endpoint"], normalize_payload(rec.get("payload")))
            self._queues.setdefault(key, []).append(rec)
        self._cursor: dict[tuple[str, str | None], int] = {}

    def answer(self, endpoint: str, payload: str | None) -> dict[str, Any]:
        """Return the next recorded reply, or a 404 when none matches."""

        key = (endpoint, normalize_payload(payload))
        queue = self._queues.get(key)
        if not queue:
            if os.environ.get("PARITY_DEBUG"):
                print(f"replay miss: {endpoint} {payload}", file=sys.stderr)
            return {"status": 404, "content": ""}
        idx = self._cursor.get(key, 0)
        self._cursor[key] = idx + 1
        return queue[min(idx, len(queue) - 1)]


def _install_recorder(records: list[dict[str, Any]]) -> None:
    original = Connection.async_query

    async def recording(
        self, endpoint, payload=None, headers=None, request_type=None, **kw
    ):  # noqa: ANN001
        try:
            args = (
                {} if request_type is None else {"request_type": request_type}
            )
            status, hdrs, content = await original(
                self, endpoint, payload=payload, headers=headers, **args
            )
        except AsusRouter404Error:
            records.append(
                {
                    "endpoint": endpoint.value,
                    "payload": payload,
                    "status": 404,
                    "content": "",
                }
            )
            raise
        records.append(
            {
                "endpoint": endpoint.value,
                "payload": payload,
                "status": status,
                "content": content,
            }
        )
        return status, hdrs, content

    Connection.async_query = recording  # type: ignore[method-assign]


def _install_replay(session: Session) -> None:
    async def connect(self, *args: Any, **kwargs: Any) -> bool:  # noqa: ANN001
        self._connected = True
        self._token = "replay"  # noqa: S105 (replay stand-in, no device)
        return True

    async def disconnect(self, *args: Any, **kwargs: Any) -> bool:  # noqa: ANN001
        self._connected = False
        return True

    async def query(
        self, endpoint, payload=None, headers=None, request_type=None, **kw
    ):  # noqa: ANN001
        rec = session.answer(endpoint.value, payload)
        if rec["status"] == 404:
            raise AsusRouter404Error(f"Endpoint {endpoint} not found")
        return rec["status"], {}, rec["content"]

    Connection.async_connect = connect  # type: ignore[method-assign]
    Connection.async_disconnect = disconnect  # type: ignore[method-assign]
    Connection.async_query = query  # type: ignore[method-assign]


async def _run_pipeline(router: AsusRouter) -> dict[str, Any]:
    """Connect and fetch every default source once, in name order."""

    results: dict[str, Any] = {}
    await router.async_connect()
    results["_identity"] = canonical(router.description)
    for source in router._default_sources():  # noqa: SLF001
        name = type(source).__name__
        try:
            data = await router.async_fetch_data(source, force=True)
        except Exception as ex:  # noqa: BLE001
            results[name] = {"_error": type(ex).__name__}
            continue
        content = None
        if isinstance(data, dict):
            content = next(iter(data.values()), None)
        results[name] = canonical(content)
    await router.async_disconnect()
    return results


def cmd_capture(args: argparse.Namespace) -> int:
    """Record a session from the device named by AR_HOST/AR_USER/AR_PASS."""

    records: list[dict[str, Any]] = []
    _install_recorder(records)
    router = AsusRouter(
        hostname=os.environ["AR_HOST"],
        username=os.environ["AR_USER"],
        password=os.environ["AR_PASS"],
        use_ssl=args.ssl,
    )

    async def run() -> None:
        try:
            await _run_pipeline(router)
        finally:
            await router.async_close()

    asyncio.run(run())
    out = Path(args.session)
    out.parent.mkdir(parents=True, exist_ok=True)
    with out.open("w", encoding="utf-8") as fh:
        for rec in records:
            fh.write(json.dumps(rec, ensure_ascii=False) + "\n")
    print(f"captured {len(records)} queries -> {out}", file=sys.stderr)
    return 0


def load_session(path: str | Path) -> Session:
    """Read a session file (JSON lines)."""

    with Path(path).open(encoding="utf-8") as fh:
        return Session([json.loads(line) for line in fh if line.strip()])


def cmd_replay(args: argparse.Namespace) -> int:
    """Replay a session and print every source in the canonical form."""

    _install_replay(load_session(args.session))
    router = AsusRouter(
        hostname="replay.invalid",
        username="admin",
        password="replay",  # noqa: S106 (replay stand-in, no device)
    )

    async def run() -> dict[str, Any]:
        try:
            return await _run_pipeline(router)
        finally:
            await router.async_close()

    results = asyncio.run(run())
    sys.stdout.write(
        json.dumps(results, sort_keys=True, indent=1, ensure_ascii=False)
        + "\n"
    )
    return 0


def main() -> int:
    """Run the capture or replay command."""

    parser = argparse.ArgumentParser(description=__doc__)
    sub = parser.add_subparsers(dest="cmd", required=True)
    cap = sub.add_parser("capture", help="record a live session")
    cap.add_argument("session")
    cap.add_argument("--ssl", action="store_true")
    cap.set_defaults(func=cmd_capture)
    rep = sub.add_parser("replay", help="replay a session to canonical JSON")
    rep.add_argument("session")
    rep.set_defaults(func=cmd_replay)
    args = parser.parse_args()
    return args.func(args)


if __name__ == "__main__":
    raise SystemExit(main())
