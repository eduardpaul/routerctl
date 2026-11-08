"""Sanitize a recorded session so it can be committed as a fixture.

    uv run --project parity python parity/sanitize.py \
        <in session.jsonl> <out session.jsonl>

Replacements are deterministic and consistent across the whole session
(bodies AND request payloads, including URL-encoded forms), so a
sanitized session still replays: both libraries build their follow-up
requests from the (sanitized) replies.

- MAC addresses -> 02:00:00:xx:xx:xx (locally administered), same
  separator and letter case as the original
- public IPv4 addresses -> 203.0.113.0/24 (TEST-NET-3); private ranges kept
- values of sensitive keys (passwords, keys, PSKs, serials, tokens, SSIDs,
  usernames, DDNS hostnames) -> placeholders of the same kind
- every replaced secret value is then also replaced wherever else it
  appears verbatim (e.g. inside logs)

The script finally verifies that no original secret survives anywhere.
"""

from __future__ import annotations

import hashlib
import ipaddress
import json
from pathlib import Path
import re
import sys
from typing import Any

MAC_RE = re.compile(
    # Not preceded by a hex digit, unless that digit ends a %XX escape
    r"(?:(?<![0-9A-Fa-f])|(?<=%[0-9A-Fa-f]{2}))"
    r"([0-9A-Fa-f]{2})(:|-|%3A|%3a)([0-9A-Fa-f]{2})(?:\2)([0-9A-Fa-f]{2})(?:\2)"
    r"([0-9A-Fa-f]{2})(?:\2)([0-9A-Fa-f]{2})(?:\2)([0-9A-Fa-f]{2})"
    r"(?![0-9A-Fa-f])"
)
IPV4_RE = re.compile(
    r"(?<![\d.])(\d{1,3})\.(\d{1,3})\.(\d{1,3})\.(\d{1,3})(?![\d.])"
)

# key name pattern -> placeholder kind
SENSITIVE_KEYS = [
    (
        re.compile(
            r"(passwd|password|_psk|psk$|_priv|privkey|private|secret|_key$|wpa_psk|token|auth_?key|crypt_key|ca$|cert|_crt|_pin$|pincode)",
            re.I,
        ),
        "secret",
    ),
    (re.compile(r"(serial|_sn$|serial_no)", re.I), "serial"),
    (re.compile(r"ssid", re.I), "ssid"),
    (re.compile(r"(username|_user$|_usr$|account)", re.I), "user"),
    (
        re.compile(
            r"(ddns_hostname|hostname_x|ddns_hostname_old|_fqdn)", re.I
        ),
        "host",
    ),
]
# Secrets embedded in syslog lines, e.g. the ASUS DDNS token exchange
# `auth_string={ "cusid": "...", "userticket": "...", ... }` - the quotes
# may be JSON-escaped (\") inside the reply body, so PAIR_RE misses them
LOG_SECRET_RE = re.compile(
    r"(?P<key>cusid|userticket|devicemac|devicemd5mac|ddnstoken|ticket)"
    r'(?:\\?")?\s*:\s*(?:\\?")(?P<val>[^"\\]{4,})'
)
# UUIDs (speedtest result IDs resolve to a public results page with the
# ISP, server and time; partition UUIDs identify the device)
UUID_RE = re.compile(
    r"(?i)(?<![0-9a-f])[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}(?![0-9a-f])"
)
# Hardware serial numbers printed by the kernel
CHIP_SERIAL_RE = re.compile(r"(serial number:\s*)([0-9A-Fa-f]{8,})")
# JSON "key":"value" and JS key = "value" / key: 'value'
PAIR_RE = re.compile(
    r"""(?P<q>["']?)(?P<key>[A-Za-z0-9_.\-]{2,64})(?P=q)\s*(?P<sep>:|=)\s*(?P<vq>["'])(?P<val>(?:\\.|(?!(?P=vq)).)*)(?P=vq)"""
)
# Client/device names are personal data, but `name` is too generic to
# redact everywhere: only inside client-list / onboarding replies
DEVICE_NAME_KEYS = re.compile(
    r"^(name|nickName|dev_name|hostname|alias|ui_model_name)$"
)
DEVICE_CONTEXT = re.compile(r"clientlist|onboarding|update_clients", re.I)
KEEP_VALUES = {
    "",
    "0",
    "1",
    "none",
    "null",
    "disabled",
    "enabled",
    "N/A",
    "unknown",
}


class Sanitizer:
    """Deterministic replacements, shared by every record of a session."""

    def __init__(self) -> None:
        """Start with empty replacement tables."""

        self.macs: dict[str, str] = {}
        self.ips: dict[str, str] = {}
        self.secrets: dict[str, str] = {}
        self.counters: dict[str, int] = {}
        self.device_context = False

    # -- MAC ---------------------------------------------------------------
    def _mac(self, m: re.Match[str]) -> str:
        octets = [m.group(i) for i in (1, 3, 4, 5, 6, 7)]
        sep = m.group(2)
        key = "".join(octets).lower()
        if key in ("000000000000", "ffffffffffff"):
            return m.group(0)
        if key not in self.macs:
            digest = hashlib.sha256(key.encode()).hexdigest()
            self.macs[key] = "0200" + "00" + digest[:6]
        new = self.macs[key]
        parts = [new[i : i + 2] for i in range(0, 12, 2)]
        upper = any(c.isupper() for c in "".join(octets))
        parts = [p.upper() if upper else p for p in parts]
        return sep.join(parts)

    # -- IPv4 --------------------------------------------------------------
    def _ip(self, m: re.Match[str]) -> str:
        text = m.group(0)
        try:
            addr = ipaddress.IPv4Address(text)
        except ValueError:
            return text
        if (
            addr.is_private
            or addr.is_loopback
            or addr.is_link_local
            or addr.is_multicast
            or addr.is_unspecified
            or addr.is_reserved
            or str(addr).startswith("255.")
        ):
            return text
        # Single-digit dotted quads are firmware versions (3.0.0.4, 3.0.0.6)
        if all(len(part) == 1 for part in text.split(".")):
            return text
        # Keep well-known public resolvers (they are not personal data)
        if text in (
            "1.1.1.1",
            "1.0.0.1",
            "8.8.8.8",
            "8.8.4.4",
            "9.9.9.9",
            "149.112.112.112",
        ):
            return text
        if text not in self.ips:
            self.ips[text] = f"203.0.113.{len(self.ips) % 250 + 1}"
        return self.ips[text]

    # -- secrets by key ------------------------------------------------------
    def _placeholder(self, kind: str, value: str) -> str:
        if value in self.secrets:
            return self.secrets[value]
        n = self.counters.get(kind, 0) + 1
        self.counters[kind] = n
        if kind == "ssid":
            new = f"ssid-{n}"
        elif kind == "user":
            new = f"user{n}"
        elif kind == "host":
            new = f"host{n}.example.com"
        elif kind == "device":
            new = f"device-{n}"
        elif kind == "serial":
            new = f"SERIAL{n:06d}"
        else:
            new = f"redacted-{n}"
        self.secrets[value] = new
        return new

    def _pair(self, m: re.Match[str]) -> str:
        key, val = m.group("key"), m.group("val")
        if val in KEEP_VALUES or len(val) < 3:
            return m.group(0)
        rules = list(SENSITIVE_KEYS)
        if self.device_context:
            rules.append((DEVICE_NAME_KEYS, "device"))
        for pattern, kind in rules:
            if pattern.search(key):
                new = self._placeholder(kind, val)
                start, end = m.span("val")
                base = m.start(0)
                text = m.group(0)
                return text[: start - base] + new + text[end - base :]
        return m.group(0)

    def collect(self, text: str, payload: str | None = None) -> None:
        """First pass: learn secret values from key/value pairs."""

        self.device_context = bool(
            DEVICE_CONTEXT.search(f"{payload or ''} {text[:200]}")
        )
        PAIR_RE.sub(self._pair, text)
        self.device_context = False
        for m in LOG_SECRET_RE.finditer(text):
            self._placeholder("secret", m.group("val"))
        for m in CHIP_SERIAL_RE.finditer(text):
            self._placeholder("serial", m.group(2))

    def apply(self, text: str | None) -> str | None:
        """Second pass: replace everything learned, then MACs and IPs."""

        if text is None:
            return None
        text = PAIR_RE.sub(self._pair, text)
        # Replace learned secrets anywhere else they appear (logs, lists);
        # longest first so substrings do not break longer values
        for old in sorted(self.secrets, key=len, reverse=True):
            if len(old) >= 4:
                text = text.replace(old, self.secrets[old])
        text = MAC_RE.sub(self._mac, text)
        text = self._bare_macs(text)
        text = UUID_RE.sub(self._uuid, text)
        return IPV4_RE.sub(self._ip, text)

    def _uuid(self, m: re.Match[str]) -> str:
        """Map a UUID to a deterministic placeholder of the same case."""

        old = m.group(0).lower()
        uuids = self.__dict__.setdefault("uuids", {})
        if old not in uuids:
            uuids[old] = f"00000000-0000-4000-8000-{len(uuids) + 1:012d}"
        new = uuids[old]
        return new.upper() if m.group(0).isupper() else new

    def _bare_macs(self, text: str) -> str:
        """Replace separator-less forms of every MAC already seen."""

        for old, new in self.macs.items():
            text = re.sub(
                r"(?i)(?<![0-9a-f])" + old + r"(?![0-9a-f])",
                lambda m, new=new: (
                    new.upper() if m.group(0).isupper() else new
                ),
                text,
            )
        return text


def main() -> int:
    """Sanitize the session named first into the file named second."""

    src, dst = Path(sys.argv[1]), Path(sys.argv[2])
    records = [
        json.loads(line)
        for line in src.read_text(encoding="utf-8").splitlines()
        if line.strip()
    ]
    s = Sanitizer()
    for rec in records:
        s.collect(rec.get("content") or "", rec.get("payload"))
    out: list[dict[str, Any]] = []
    for record in records:
        rec = dict(record)
        rec["content"] = s.apply(rec.get("content") or "")
        rec["payload"] = s.apply(rec.get("payload"))
        out.append(rec)
    dst.parent.mkdir(parents=True, exist_ok=True)
    blob = "".join(json.dumps(r, ensure_ascii=False) + "\n" for r in out)

    # Verify: no original secret, MAC or public IP survives
    leaks = [v for v in s.secrets if len(v) >= 4 and v in blob]
    leaks += [
        ip
        for ip in s.ips
        if re.search(r"(?<![\d.])" + re.escape(ip) + r"(?![\d.])", blob)
    ]
    leaks += [u for u in getattr(s, "uuids", {}) if u in blob.lower()]
    low = blob.lower()
    for mac in s.macs:
        for sep in (":", "-", "%3a", ""):
            form = sep.join(mac[i : i + 2] for i in range(0, 12, 2))
            if form in low:
                leaks.append(form)
    if leaks:
        print(
            f"refusing to write: {len(leaks)} values survived sanitizing",
            file=sys.stderr,
        )
        return 1
    dst.write_text(blob, encoding="utf-8")
    print(
        f"{len(out)} records -> {dst} ({len(s.macs)} MACs, "
        f"{len(s.ips)} public IPs, {len(s.secrets)} secret values replaced)",
        file=sys.stderr,
    )
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
