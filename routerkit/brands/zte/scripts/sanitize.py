#!/usr/bin/env python3
"""Build a sanitized httprec session of a ZTE web UI from raw captures.

    python3 -I sanitize.py RAW_DIR OUT [--login-page PAGE] [--model MODEL]

RAW_DIR is a capture made while logged in (keep it under the git-ignored
`testdata/local/<device>/`), laid out as:

    RAW_DIR/login.json             reply to the login POST
    RAW_DIR/data/<tag>.xml         reply to `menuData&_tag=<tag>` (XML or
                                   JSON, despite the extension)

OUT is the session (JSON lines, the `netkit/httprec` format) that
`ztefake.Session()` replays, for example
`ztefake/testdata/<model>/<firmware>/session.jsonl`.

Only the data tags are taken from the capture, field by field:

- MACs -> 02:00:5e:00:xx:xx, consistent across the session;
- the LAN (192.168/16) stays; ISP-side IPv4 addresses are remapped
  (public ones to 203.0.113.x, 10/8 to 10.0.0.x, 172.16/12 to
  172.16.0.x, CGNAT 100.64/10 to 100.64.0.x); every IPv6 address goes
  to 2001:db8::/32 or a short fe80:: address;
- serials, the PPPoE user and ISP names get fixed placeholders, SSIDs
  and host names numbered ones; secrets and every value listed in a
  reply's `<encode>` element are emptied.

The UI pages are replaced by minimal pages that carry a fake page token,
and the login exchanges the capture lacks are synthesized. Run
`leakscan.py` on the result before committing it.
"""

from __future__ import annotations

import argparse
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
    r"(<ParaName>)([^<]*)(</ParaName>\s*<ParaValue>)([^<]*)(</ParaValue>)"
)
ENCODE_RE = re.compile(r"<encode>([^<]*)</encode>")
SECRET_RE = re.compile(r"(?i)(pass|pwd|key|psk|secret|token|loid|auth_?code)")
# Port and default SSID labels ("LAN1", "SSID5") are the same everywhere
PORT_LABEL_RE = re.compile(r"(LAN|SSID|WAN)\d+")

# ParaName -> fixed placeholder. The serials keep only the public ZTE
# vendor prefix.
SERIALS = {"SerialNumber": "ZTEE00000000001", "Sn": "ZTEG00000001"}
FIXED = {
    "UserName": "user@example.invalid",
    "PPPoEACName": "example-ac",
    "PPPoeServiceName": "example-service",
    "OnuAlias": "example-onu",
}

LAN = ipaddress.ip_network("192.168.0.0/16")
ISP_PRIVATE = [
    (ipaddress.ip_network("10.0.0.0/8"), "10.0.0.%d"),
    (ipaddress.ip_network("172.16.0.0/12"), "172.16.0.%d"),
    (ipaddress.ip_network("100.64.0.0/10"), "100.64.0.%d"),
]

PAGE_TOKEN = "SanitizedPageToken0001"  # noqa: S105 (a placeholder)
SESS_TOKEN = "SanitizedSessToken0001"  # noqa: S105 (a placeholder)
LOGIN_SALT = "12345678"
COOKIES = ["SID=REDACTED; PATH=/; HttpOnly; SameSite=strict"]
NOT_FOUND = (
    "<html>\n    <head>\n        <title>404 Not Found</title>\n"
    "    </head>\n<body>404</body></html>"
)
HTML = "text/html; charset=utf-8"
JSON = "application/json; charset=utf-8"
XML = "text/xml; charset=utf-8"

# The pages the driver loads (menuView) and the data tags each one owns,
# in the driver's order
VIEWS = [
    ("statusMgr", ["devmgr_statusmgr_lua.lua"]),
    ("ethWanStatus", ["wan_internetstatus_lua.lua&TypeUplink=2&pageType=1"]),
    (
        "localNetStatus",
        [
            "status_lan_info_lua.lua",
            "wlan_wlanstatus_lua.lua",
            "wlan_client_stat_lua.lua",
            "accessdev_landevs_lua.lua",
        ],
    ),
    (
        "wlanBasic",
        [
            "wlan_wlanbasiconoff_lua.lua",
            "wlan_wlanbasicadconf_lua.lua",
            "wlan_wlansssidconf_lua.lua",
        ],
    ),
    ("mmTopology", ["topo_lua.lua"]),
    (
        "homePage",
        [
            "accessdev_homepage_lua.lua&InstNum=5",
            "wlan_homepage_lua.lua&InstNum=5",
        ],
    ),
    ("accountMgr", ["web_login_timeout_lua.lua"]),
]


class Sanitizer:
    """Consistent replacements across every reply of one session."""

    def __init__(self) -> None:
        """Start with empty replacement tables."""

        self.macs: dict[str, str] = {}
        self.v4: dict[str, str] = {}
        self.v6: dict[str, str] = {}
        self.names: dict[str, str] = {}
        self.ssids: dict[str, str] = {}

    def mac(self, mac: str) -> str:
        """Map a MAC, keeping its separator and letter case."""

        sep = ":" if ":" in mac else "-"
        key = mac.lower().replace("-", ":")
        if key in ("00:00:00:00:00:00", "ff:ff:ff:ff:ff:ff"):
            return mac
        if key not in self.macs:
            n = len(self.macs) + 1
            self.macs[key] = f"02:00:5e:00:{n >> 8:02x}:{n & 0xFF:02x}"
        out = self.macs[key].replace(":", sep)
        return out.upper() if mac.isupper() else out

    def ipv4(self, text: str) -> str:
        """Map an ISP-side IPv4 address; keep the LAN and special ones."""

        try:
            ip = ipaddress.IPv4Address(text)
        except ValueError:
            return text
        if (
            ip in LAN
            or ip.is_unspecified
            or ip.is_loopback
            or ip.is_link_local
            or ip.is_multicast
            or text.startswith("255.")
        ):
            return text
        if text not in self.v4:
            n = len(self.v4) + 1
            self.v4[text] = next(
                (fmt % n for net, fmt in ISP_PRIVATE if ip in net),
                f"203.0.113.{10 + n}",
            )
        return self.v4[text]

    def ipv6(self, text: str) -> str:
        """Map an IPv6 address to fe80::1xxx or 2001:db8::/32."""

        try:
            ip = ipaddress.IPv6Address(text)
        except ValueError:
            return text
        if ip.is_unspecified or ip.is_loopback:
            return text
        key = ip.exploded
        if key not in self.v6:
            n = len(self.v6) + 1
            if ip.is_link_local:
                self.v6[key] = f"fe80::{0x1000 + n:x}"
            else:
                self.v6[key] = f"2001:db8:{0x100 + n:x}::{n:x}"
        return self.v6[key]

    @staticmethod
    def numbered(table: dict[str, str], prefix: str, value: str) -> str:
        """Map a name to `<prefix>-<n>`, consistently."""

        if value == "":
            return value
        if value not in table:
            table[value] = f"{prefix}-{len(table) + 1}"
        return table[value]

    def text(self, text: str) -> str:
        """Map the MACs, ISP-side IPv4 and every IPv6 address in any text."""

        text = MAC_RE.sub(lambda m: self.mac(m.group(1)), text)
        text = IPV4_RE.sub(lambda m: self.ipv4(m.group(1)), text)

        def v6(m: re.Match[str]) -> str:
            found = m.group(1)
            if found.count(":") < 2 or not re.search(r"[0-9A-Fa-f]", found):
                return found
            return self.ipv6(found)

        return IPV6_RE.sub(v6, text)

    def xml(self, text: str) -> str:
        """Sanitize a `<ParaName>`/`<ParaValue>` reply."""

        encoded: set[str] = set()
        for m in ENCODE_RE.finditer(text):
            encoded.update(x.strip() for x in m.group(1).split(","))

        def pair(m: re.Match[str]) -> str:
            name, value = m.group(2), m.group(4)
            if value == "":
                pass
            elif name in encoded or SECRET_RE.search(name):
                value = ""
            elif name in SERIALS:
                value = SERIALS[name]
            elif name in FIXED:
                value = FIXED[name]
            elif name == "ESSID":
                value = self.numbered(self.ssids, "Example-SSID", value)
            elif name == "HostName" or (
                name == "AliasName" and not PORT_LABEL_RE.fullmatch(value)
            ):
                value = self.numbered(self.names, "host", value)
            return m.group(1) + name + m.group(3) + value + m.group(5)

        return self.text(PAIR_RE.sub(pair, text))

    def json(self, text: str) -> str:
        """Sanitize a JSON reply (the topology)."""

        def walk(x: Any, key: str = "") -> Any:
            if isinstance(x, dict):
                return {k: walk(v, k) for k, v in x.items()}
            if isinstance(x, list):
                return [walk(v, key) for v in x]
            if isinstance(x, str):
                if SECRET_RE.search(key):
                    return ""
                if key in ("DeviceName", "HostName") and not x.startswith(
                    "ZTE:"
                ):
                    return self.numbered(self.names, "host", x)
                return self.text(x)
            return x

        return json.dumps(walk(json.loads(text)), separators=(",", ":"))


def js_escape(s: str) -> str:
    """Escape a string the way the UI writes `_sessionTmpToken`."""

    return "".join(f"\\x{ord(c):02x}" for c in s)


def html_entities(s: str) -> str:
    """Encode every character as a decimal entity (the login title)."""

    return "".join(f"&#{ord(c)};" for c in s)


def exchange(  # noqa: PLR0913 (one argument per httprec field)
    method: str,
    url: str,
    status: int,
    ctype: str,
    body: str,
    *,
    req: str = "",
    cookies: bool = False,
) -> dict[str, Any]:
    """Return one recorded request and its reply."""

    header: dict[str, list[str]] = {"Content-Type": [ctype]}
    if cookies:
        header["Set-Cookie"] = COOKIES
    e: dict[str, Any] = {
        "method": method,
        "url": url,
        "status": status,
        "header": header,
        "response": body,
    }
    if req:
        e["body"] = req
    return e


def login_exchanges(raw: Path, login_page: Path) -> list[dict[str, Any]]:
    """Return the home page and the synthesized login exchanges."""

    login = json.loads((raw / "login.json").read_text(encoding="utf-8"))
    login = {
        k: SESS_TOKEN if k == "sess_token" else v for k, v in login.items()
    }
    entry = {
        "lockingTime": 0,
        "loginErrMsg": "",
        "promptMsg": "",
        "sess_token": SESS_TOKEN,
    }
    return [
        exchange(
            "GET",
            "/",
            200,
            HTML,
            login_page.read_text(encoding="utf-8"),
            cookies=True,
        ),
        exchange(
            "GET",
            "/?_type=loginData&_tag=login_entry",
            200,
            JSON,
            json.dumps(entry, separators=(",", ":")),
            cookies=True,
        ),
        exchange(
            "GET",
            "/?_type=loginData&_tag=login_token",
            200,
            XML,
            f"<ajax_response_xml_root>{LOGIN_SALT}</ajax_response_xml_root>",
            cookies=True,
        ),
        exchange(
            "POST",
            "/?_type=loginData&_tag=login_entry",
            200,
            JSON,
            json.dumps(login, separators=(",", ":")),
            req="Password=REDACTED&Username=REDACTED"
            "&_sessionTOKEN=REDACTED&action=login",
            cookies=True,
        ),
    ]


def build(raw: Path, login_page: Path, model: str) -> tuple[list, Sanitizer]:
    """Build every exchange of the session, in the driver's order."""

    s = Sanitizer()
    token = js_escape(PAGE_TOKEN)
    out = login_exchanges(raw, login_page)
    out.append(
        exchange(
            "GET",
            "/",
            200,
            HTML,
            f"<html><title>{html_entities(model)}</title><script>"
            f'var _sessionTmpToken = "";\n_sessionTmpToken = "{token}";\n'
            "function LogOff(){}</script></html>",
        )
    )
    for view, tags in VIEWS:
        out.append(
            exchange(
                "GET",
                f"/?_type=menuView&_tag={view}&Menu3Location=0",
                200,
                HTML,
                f'<div id="{view}"><script>\n_sessionTmpToken = "{token}";\n'
                "</script></div>",
            )
        )
        for tag in tags:
            name = tag.split("&")[0]
            body = (raw / "data" / f"{name}.xml").read_text(encoding="utf-8")
            is_json = body.lstrip().startswith("{")
            out.append(
                exchange(
                    "GET",
                    f"/?_type=menuData&_tag={tag}",
                    200,
                    JSON if is_json else XML,
                    s.json(body) if is_json else s.xml(body),
                )
            )
    # A "user" account cannot open the PON page
    out.append(
        exchange(
            "GET",
            "/?_type=menuView&_tag=ponopticalinfo&Menu3Location=0",
            404,
            HTML,
            NOT_FOUND,
        )
    )
    out.append(
        exchange(
            "POST",
            "/?_type=loginData&_tag=logout_entry",
            200,
            JSON,
            '{"need_refresh":true}',
            req="IF_LogOff=1&_sessionTOKEN=REDACTED",
            cookies=True,
        )
    )
    return out, s


def main() -> int:
    """Write the sanitized session."""

    here = Path(__file__).resolve().parent
    parser = argparse.ArgumentParser(
        description=__doc__, formatter_class=argparse.RawTextHelpFormatter
    )
    parser.add_argument("raw", type=Path, help="raw capture directory")
    parser.add_argument("out", type=Path, help="session file to write")
    parser.add_argument(
        "--login-page",
        type=Path,
        default=here.parent / "ztefake" / "testdata" / "login_page.html",
        help="login page served for GET / (default: the ztefake fixture)",
    )
    parser.add_argument(
        "--model",
        default="F8648P",
        help="model name in the main page title (default: F8648P)",
    )
    args = parser.parse_args()

    out, s = build(args.raw, args.login_page, args.model)
    args.out.parent.mkdir(parents=True, exist_ok=True)
    with args.out.open("w", encoding="utf-8") as fh:
        for e in out:
            fh.write(json.dumps(e, ensure_ascii=False) + "\n")
    print(
        f"wrote {len(out)} exchanges; mapped {len(s.macs)} MACs, "
        f"{len(s.v4)} IPv4, {len(s.v6)} IPv6, {len(s.names)} names, "
        f"{len(s.ssids)} SSIDs"
    )
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
