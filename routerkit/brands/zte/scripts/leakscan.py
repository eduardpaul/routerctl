#!/usr/bin/env python3
"""Scan files for private data harvested from raw ZTE captures.

    python3 -I leakscan.py CAPTURE_DIR FILE...

CAPTURE_DIR is the git-ignored capture of a device (for example
`testdata/local/<device>/`, raw replies, headers and cookie jar
included). The scan collects every MAC, ISP-side IPv4, IPv6 (and /48
prefix or link-local interface ID), serial (and its first eight
characters), host name, SSID, user name, secret, cookie and session
token found there. It then exits 1 when any of them occurs, in any
spelling, in one of the FILEs, or when a FILE holds a MAC or address
outside the ranges `sanitize.py` maps to. Run it on a sanitized session
and on every golden file built from it before committing them.
"""

from __future__ import annotations

import argparse
import contextlib
import ipaddress
import json
from pathlib import Path
import re
from typing import Any

MAC_RE = re.compile(
    r"(?<![0-9A-Fa-f:])"
    r"([0-9A-Fa-f]{2}([:-])(?:[0-9A-Fa-f]{2}\2){4}[0-9A-Fa-f]{2})"
    r"(?![0-9A-Fa-f:])"
)
IPV4_RE = re.compile(r"(?<![\d.])(\d{1,3}(?:\.\d{1,3}){3})(?![\d.])")
IPV6_RE = re.compile(
    r"(?<![0-9A-Fa-f:])"
    r"([0-9A-Fa-f]{0,4}(?::[0-9A-Fa-f]{0,4}){2,7})"
    r"(?![0-9A-Fa-f:])"
)
PAIR_RE = re.compile(
    r"<ParaName>([^<]*)</ParaName>\s*<ParaValue>([^<]*)</ParaValue>"
)
PAGE_TOKEN_RE = re.compile(r'_sessionTmpToken = "((?:\\x[0-9a-fA-F]{2})+)"')
COOKIE_RE = re.compile(
    r"(?i)(?:set-cookie:\s*|\t)(SID|_[A-Z0-9]+)[=\t]([^;\s]+)"
)
SESS_TOKEN_RE = re.compile(r'"sess_token"\s*:\s*"([^"]+)"')
# ParaNames / JSON keys whose values are private (addresses are found by
# the MAC and IP patterns instead)
SENSITIVE = re.compile(
    r"(?i)^(serialnumber|sn|hostname|host_name|aliasname|essid|ssid|bssid"
    r"|username|user_name|password|keypassphrase|wepkey|macaddress|mac"
    r"|workifmac|pppoeacname|pppoeservicename|onualias|serial_number"
    r"|pon_serial|serial)$|pass|pwd|psk|secret|token"
)
SERIAL = re.compile(r"(?i)^(serialnumber|sn|serial_number|pon_serial|serial)$")
# Values that are the same on every device
COMMON = {"true", "false", "none", "null", "succ", "auto", "zte", "f8648p"}
PORT_LABEL_RE = re.compile(r"(SSID|LAN|WAN)\d+")
# JSON keys that hold the model, not a private value
MODEL_KEYS = {
    "model",
    "manufacturer",
    "manufacturer_oui",
    "devicename",
    "device_name",
}
# What sanitize.py maps ISP-side addresses to
ALLOWED_V4 = [
    ipaddress.ip_network(n)
    for n in (
        "192.0.2.0/24",
        "198.51.100.0/24",
        "203.0.113.0/24",
        "10.0.0.0/24",
        "172.16.0.0/24",
        "100.64.0.0/24",
    )
]
LAN = ipaddress.ip_network("192.168.0.0/16")
DOC_V6 = ipaddress.ip_network("2001:db8::/32")
NULL_MACS = ("00:00:00:00:00:00", "ff:ff:ff:ff:ff:ff")
EUI64_5E = ["0200", "5eff"]  # EUI-64 groups 5-6 of a 02:00:5e MAC
SERIAL_PREFIX = 8  # beyond the 4-character vendor ID


def is_isp_v4(s: str) -> bool:
    """Report an address outside the LAN (192.168/16).

    Public, ISP-side private (10/8, 172.16/12) and shared (CGNAT
    100.64/10) addresses all count.
    """

    try:
        ip = ipaddress.IPv4Address(s)
    except ValueError:
        return False
    return not (
        ip in LAN
        or ip.is_unspecified
        or ip.is_loopback
        or ip.is_link_local
        or ip.is_multicast
        or s.startswith("255.")
    )


def is_short_link_local(ip: ipaddress.IPv6Address) -> bool:
    """Report fe80::1 and the like, which are the same everywhere."""

    return ip.is_link_local and int(ip) & ((1 << 64) - 1) < 0x10000


class Harvest:
    """The private values found in a capture, with where they came from."""

    def __init__(self) -> None:
        """Start empty."""

        self.tokens: dict[str, str] = {}

    def add(self, tok: str, why: str) -> None:
        """Record a value, unless it is too short or the same everywhere."""

        tok = tok.strip()
        if len(tok) < 4 or tok.lower() in COMMON:
            return
        if PORT_LABEL_RE.fullmatch(tok):
            return
        self.tokens.setdefault(tok, why)

    def text(self, text: str, why: str) -> None:
        """Harvest the MACs and addresses of any text."""

        for m in MAC_RE.finditer(text):
            mac = m.group(1).lower().replace("-", ":")
            if mac in NULL_MACS:
                continue
            self.add(mac, why + " mac")
            self.add(mac.replace(":", ""), why + " mac")
            self.add(mac.replace(":", "-"), why + " mac")
        for m in IPV4_RE.finditer(text):
            if is_isp_v4(m.group(1)):
                self.add(m.group(1), why + " ipv4")
        for m in IPV6_RE.finditer(text):
            try:
                ip = ipaddress.IPv6Address(m.group(1))
            except ValueError:
                continue
            if ip.is_unspecified or ip.is_loopback or is_short_link_local(ip):
                continue
            self.add(m.group(1), why + " ipv6")
            self.add(ip.compressed, why + " ipv6")
            self.add(ip.exploded, why + " ipv6")
            if not ip.is_link_local:
                hx = ip.exploded.split(":")
                prefix = ":".join(h.lstrip("0") or "0" for h in hx[:3])
                self.add(prefix, why + " ipv6 /48")
            else:
                self.add(ip.exploded.split(":", 4)[4], why + " ipv6 iid")

    def value(self, key: str, value: str, why: str) -> None:
        """Harvest the value of a sensitive field."""

        self.add(value, why + " " + key)
        if SERIAL.match(key) and len(value) > SERIAL_PREFIX:
            self.add(value[:SERIAL_PREFIX], why + " " + key + " prefix")

    def json(self, x: Any, key: str, why: str) -> None:
        """Harvest the sensitive fields of a JSON document."""

        if isinstance(x, dict):
            for k, v in x.items():
                self.json(v, k, why)
        elif isinstance(x, list):
            for v in x:
                self.json(v, key, why)
        elif isinstance(x, str) and SENSITIVE.search(key) and x:
            if key.lower() in MODEL_KEYS or x.startswith("ZTE:"):
                return
            self.value(key, x, why)

    def file(self, path: Path, why: str) -> None:
        """Harvest one capture file (reply, headers or cookie jar)."""

        text = path.read_text(encoding="utf-8", errors="replace")
        self.text(text, why)
        for m in PAIR_RE.finditer(text):
            name, value = m.group(1), m.group(2)
            if value and SENSITIVE.search(name):
                self.value(name, value, why)
        for m in PAGE_TOKEN_RE.finditer(text):
            raw = bytes.fromhex(m.group(1).replace("\\x", ""))
            self.add(raw.decode("latin-1"), why + " page token")
            self.add(m.group(1), why + " page token (escaped)")
        for m in COOKIE_RE.finditer(text):
            self.add(m.group(2), why + " cookie " + m.group(1))
        if path.suffix == ".json" or text.lstrip().startswith("{"):
            with contextlib.suppress(ValueError):
                self.json(json.loads(text), "", why)
        for m in SESS_TOKEN_RE.finditer(text):
            self.add(m.group(1), why + " sess_token")


def found(tok: str, why: str, low: str) -> bool:
    """Report whether a harvested value occurs in a (lower-cased) text.

    Addresses match on hex, colon and dot boundaries (fe80::1 is not in
    fe80::1001); anything else matches as a substring.
    """

    t = tok.lower()
    if any(w in why for w in (" mac", " ipv4", " ipv6")):
        pattern = r"(?<![0-9a-f:.])" + re.escape(t) + r"(?![0-9a-f:.])"
        return re.search(pattern, low) is not None
    return t in low


def allowed_v6(ip: ipaddress.IPv6Address) -> bool:
    """Report an IPv6 address sanitize.py may leave in a session."""

    return (
        ip.is_unspecified
        or ip.is_loopback
        or ip in DOC_V6
        or is_short_link_local(ip)
        # EUI-64 interface ID of a 02:00:5e MAC
        or (ip.is_link_local and ip.exploded.split(":")[4:6] == EUI64_5E)
    )


def scan(path: Path, tokens: dict[str, str]) -> int:
    """Print every leak in one file and return their number."""

    text = path.read_text(encoding="utf-8")
    low = text.lower()
    bad = 0
    for tok, why in tokens.items():
        if found(tok, why, low):
            print(f"LEAK {path}: {tok[:6] + '...'!r} (from {why})")
            bad += 1
    for m in MAC_RE.finditer(text):
        mac = m.group(1).lower().replace("-", ":")
        if not mac.startswith("02:00:5e:") and mac not in NULL_MACS:
            print(f"LEAK {path}: MAC outside 02:00:5e:* ({mac[:8]}...)")
            bad += 1
    for m in IPV4_RE.finditer(text):
        addr = m.group(1)
        if is_isp_v4(addr) and not any(
            ipaddress.IPv4Address(addr) in n for n in ALLOWED_V4
        ):
            print(f"LEAK {path}: public IPv4 outside the documentation ranges")
            bad += 1
    for m in IPV6_RE.finditer(text):
        try:
            ip = ipaddress.IPv6Address(m.group(1))
        except ValueError:
            continue
        if not allowed_v6(ip):
            print(
                f"LEAK {path}: IPv6 outside 2001:db8::/32 / short fe80:: "
                f"({str(ip)[:9]}...)"
            )
            bad += 1
    return bad


def main() -> int:
    """Harvest the capture, then scan every file."""

    parser = argparse.ArgumentParser(
        description=__doc__, formatter_class=argparse.RawTextHelpFormatter
    )
    parser.add_argument("capture", type=Path, help="raw capture directory")
    parser.add_argument("files", type=Path, nargs="+", help="files to scan")
    args = parser.parse_args()

    harvest = Harvest()
    for path in sorted(p for p in args.capture.rglob("*") if p.is_file()):
        harvest.file(path, path.relative_to(args.capture).as_posix())
    print(
        f"harvested {len(harvest.tokens)} sensitive tokens from {args.capture}"
    )
    bad = sum(scan(f, harvest.tokens) for f in args.files)
    print(f"{bad} leak(s)")
    return 1 if bad else 0


if __name__ == "__main__":
    raise SystemExit(main())
