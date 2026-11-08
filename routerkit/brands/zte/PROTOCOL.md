# ZTE ZXHN F8648P web UI protocol

Reverse engineered from the shipped page JavaScript of an F8648P (hardware V2.0,
software V2.0.12P7N17, Spanish UI, ISP DIGI), logged in with the `user` account
(`curRight = 2`). Only GETs, plus the login and logout requests, were sent to the device.

## Transport

- HTTP on port 80 (HTTPS also answers, with a self-signed certificate). Plain
  `Server:` header, `X-Frame-Options`, `nosniff`.
- Session cookie `SID` (HttpOnly, SameSite=strict), set on the first `GET /`, plus
  `_TESTCOOKIESUPPORT=1`. Login replaces the `SID`; logout expires it.
- The UI sends `X-Requested-With: XMLHttpRequest` on every data request. The
  client does too.

## Login (flow "b")

From `g_loginToken()` and `$(document).ready` in the login page:

1. `GET /`: the login page (sets cookies). Its `<title>` is the HTML-entity-encoded
   model name, `&#70;&#56;&#54;&#52;&#56;&#80;` (= `F8648P`).
2. `GET /?_type=loginData&_tag=login_entry` returns JSON
   `{"lockingTime":0,"loginErrMsg":"","promptMsg":"","sess_token":"<24 chars>"}`.
   `lockingTime > 0` means the login is locked for that many seconds.
3. `GET /?_type=loginData&_tag=login_token` returns
   `<ajax_response_xml_root>84054044</ajax_response_xml_root>`, a random salt (8 digits).
4. `POST /?_type=loginData&_tag=login_entry` with form-urlencoded fields
   `action=login`, `Password=hex(sha256(password + salt))`, `Username=<user>`,
   `_sessionTOKEN=<sess_token from step 2>`. `sha256` is the js-sha256 library
   (hex digest of the UTF-8 bytes).
   - success: `{"sess_token":"…","login_need_refresh":true}`
   - failure: `loginErrMsg` / `promptMsg` set, `lockingTime` > 0 when locked
5. The UI then reloads `GET /`, which now returns the main page (menu tree, home page)
   with `_sessionTmpToken = "\x4a\x6f…"` (a `\xNN`-escaped JS string).

Logout: `POST /?_type=loginData&_tag=logout_entry` with `IF_LogOff=1&_sessionTOKEN=<current
_sessionTmpToken>` returns `{"need_refresh":true}` and expires the cookie.

## Tokens

`_sessionTmpToken` rotates. Every page (menuView) response assigns a new one in an
inline script, and the UI uses the latest value for:

- `_sessionTOKEN` in POSTs (logout, configuration)
- decrypting values listed in a response's `<encode>` element (see below)

## Page context (important quirk)

Data is served by `GET /?_type=menuData&_tag=<name>_lua.lua[&params]`, but **only after
the page that owns the tag was loaded** with `GET /?_type=menuView&_tag=<menu id>&Menu3Location=0`
in the same session. Several tags can be read after one page load, and they stay
readable until another page is loaded. Reading a tag outside its page answers:

```xml
<ajax_response_xml_root>
    <IF_ERRORSTR>SessionTimeout</IF_ERRORSTR>
    <IF_ERRORPARAM>SUCC</IF_ERRORPARAM>
    <IF_ERRORTYPE>SUCC</IF_ERRORTYPE>
</ajax_response_xml_root>
```

That is the same answer as for an expired session. A menuView that does not exist, or that
the account cannot see, returns an HTML `404 Not Found` page, as does any menuView after
logout. The client tells these cases apart by probing `statusMgr`: if it loads, the session
is alive and the page or tag is "not available" (cached). If it doesn't, the client logs in
again once.

The data tags of a page are listed in `<address><span class="form-action">…</span></address>`
elements in the page HTML.

## Response format

```xml
<ajax_response_xml_root>
  <IF_ERRORPARAM>SUCC</IF_ERRORPARAM><IF_ERRORTYPE>SUCC</IF_ERRORTYPE>
  <IF_ERRORSTR>SUCC</IF_ERRORSTR><IF_ERRORID>0</IF_ERRORID>
  <OBJ_DEVINFO_ID>
    <Instance>
      <ParaName>_InstID</ParaName><ParaValue>IGD</ParaValue>
      <ParaName>ModelName</ParaName><ParaValue>F8648P</ParaValue>
      …
    </Instance>
  </OBJ_DEVINFO_ID>
  <encode>Pwd</encode>
</ajax_response_xml_root>
```

- Each object (`OBJ_*`, also `ID_WAN_COMFIG`) holds zero or more `<Instance>` elements
  of alternating `ParaName`/`ParaValue`. An empty object is `<OBJ_X></OBJ_X>`.
- Values use numeric entities (`&#32;` for a space).
- `topo_lua.lua` returns **JSON** instead:
  `{"controllerData":{"MacAddr":…,"IpAddr":…,"accdevCount":2,…},"agentlay1":{"MGET_INST_NUM":0},…}`.
  The UI's `getParaValueInXML()` also accepts JSON of the form `{"OBJ":{"Instance":[{…}]}}`.
- **Encoded values**: parameters named in `<encode>` (for example `KeyPassphrase`, `WEPKey`,
  `Pwd`) are `base64(AES-256-CBC(value))`, zero-padded, with key `SHA256(token)` and IV
  `SHA256(reverse(token))[:16]`, where token is the `_sessionTmpToken` of the page just
  loaded (`getDecodeValue()`). The WAN `UserName` and `Password` are encrypted the same
  way (verified offline on `UserName`) without being listed. The library blanks every
  credential-like parameter unless `Options.KeepSecrets` is set.

## Endpoints (all verified read-only GETs unless noted)

| Page (menuView) | Data tag | Content |
|---|---|---|
| `statusMgr` | `devmgr_statusmgr_lua.lua` | `OBJ_DEVINFO_ID` (ModelName, ManuFacturer, ManuFacturerOui, SoftwareVer, SoftwareVerExtent, HardwareVer, BootVer, VerDate, SerialNumber, OnuAlias), `OBJ_CPUMEMUSAGE_ID` (CpuUsage1..4, MemUsage %), `OBJ_POWERONTIME_ID` (PowerOnTime s), `OBJ_SN_INFO_ID` (Sn: PON/GPON serial) |
| `ethWanStatus` | `wan_internetstatus_lua.lua&TypeUplink=2&pageType=1` | `ID_WAN_COMFIG`, one instance per WAN connection: WANCName, ConnStatus/ConnStatus6, ConnError, TransType (PPPoE), linkMode (PPP/IP), Addressingtype (Static/DHCP), IpMode, IPAddress, SubnetMask, GateWay, DNS1..3, Gua1..3, Pd/PdLen, Gateway6, Dns1v6..3v6, WorkIFMac, VLANID, MTU, IsDefGW, IsNAT, StrServList, UpTime/UpTimeV6 (s), Rx/TxBytes, Rx/TxPackets (PPP only), UserName/Password (encrypted) |
| `localNetStatus` | `status_lan_info_lua.lua` | `OBJ_PON_PORT_BASIC_STATUS_ID`, Ethernet ports DEV.ETH.IF1..5 (the UI names them LAN1..LAN5): Status (0 up, 1 down), Speed (Mbit/s), Duplex, In/Out Bytes/Pkts/Unicast/Multicast/Error/Discard |
| `localNetStatus` | `wlan_wlanstatus_lua.lua` | `OBJ_WLANAP_ID` (SSID1..8: Enable, ESSID, BeaconType, auth/encryption, WLANViewName → radio), `OBJ_WLANCONFIGDRV_ID` (Bssid, ChannelInUsed, BandWidthInUsed, TotalBytes/Packets Sent/Received), `OBJ_WLANSETTING_ID` (RD1 2.4GHz, RD2 5GHz) |
| `localNetStatus` | `wlan_client_stat_lua.lua` | `OBJ_WLAN_AD_ID` associated stations (fields from the UI template: MACAddress, IPAddress, IPV6Address, HostName, AliasName = SSIDn, RSSI, SNR, NOISE, TxRate, RxRate, LinkTime, ConnectionTime, CurrentMode, MCS, BAND, RX/TXBytes, RX/TXPackets). It was empty on the test unit (WiFi off), so value formats are unverified. `OBJ_WLANAP_ID` maps alias → ESSID |
| `localNetStatus` | `accessdev_landevs_lua.lua` | `OBJ_ACCESSDEV_ID` wired clients: MACAddress, IPAddress, IPV6Address, HostName, AliasName (LAN1..5) |
| `localNetStatus` | `usb_usbdevs_lua.lua` | USB storage (empty on the test unit) |
| `wlanBasic` | `wlan_wlanbasiconoff_lua.lua` | `OBJ_WLANSETTING_ID` RadioStatus per band, `OBJ_WLANTIMECFG_ID`/`OBJ_WLANTIME_ID` schedule |
| `wlanBasic` | `wlan_wlanbasicadconf_lua.lua` | `OBJ_WLANSETTING_ID` per radio: Band, RadioStatus, Channel, AutoChannelEnabled, BandWidth, Standard, TxPower, CountryCode, …; `OBJ_CHANNEL_ID` channel lists (about 160 instances, 57 KB) |
| `wlanBasic` | `wlan_wlansssidconf_lua.lua` | `OBJ_WLANAP_ID` (Enable, ESSID, ESSIDHideEnable, MaxUserNum, …), `OBJ_WLANPSK_ID` (KeyPassphrase, encoded), `OBJ_WLANWEPKEY_ID` (WEPKey, encoded), `OBJ_MAP_MASTER_ID` |
| `mmTopology` | `topo_lua.lua` | JSON: controllerData (DeviceName `ZTE:F8648P`, MacAddr = LAN MAC, IpAddr, SoftwareVer, accdevCount), agentlay1/2 (EasyMesh agents) |
| `homePage` | `accessdev_homepage_lua.lua&InstNum=5`, `wlan_homepage_lua.lua&InstNum=5` | home page summaries (also `firewall_homepage_lua.lua`, `usb_homepage_lua.lua`, `voip_homepage_lua.lua`, not read) |
| `accountMgr` | `web_login_timeout_lua.lua` | `OBJ_USERIF_ID` Timeout (minutes, 5 on the test unit). The same page has `devauth_accountmgr_lua.lua` (accounts), deliberately not read |
| `ponopticalinfo` | `optical_info_lua.lua` | **unverified**: the PON optical page is not in the `user` account's menu (the menuView returns 404). The id and tag are the ones other ZTE F6xx/F8xx firmwares use for admin accounts; the parser matches RxPower/TxPower/Temp/Volt/Current loosely |

Other endpoints seen in the JS, not used: `/?_type=hiddenData&_tag=accessdev_data&DeveiceType=ETH|WLAN|ALL`
(access device list) and `/?_type=hiddenData&_tag=sntp_data` (clock).

Menu ids visible to `user` include `homePage`, `mmTopology`, `ethWanStatus`, `Wan3gStatus`, `firewall`,
`filterCriteria`, `dmz`, `portForwarding`, `parentCtrl`, `ddns`, `sntp`, `localNetStatus`,
`wlanBasic`, `wlanAdvanced`, `wps`, `wlanStaScanAP`, `wifibandsteer`, `lanMgrIpv4`, `lanMgrIpv6`,
`ftp`, `upnp`, `dms`, `samba`, `dns`, `usbfunccfg`, `voipStatus`, `statusMgr`, `rebootAndReset`
and `accountMgr` (the full tree is the `menuTreeJSON` variable of the main page).

## Configuration writes (actions): known, NOT implemented, untested

Every configuration change goes through `dataPost()`:

- `POST` to the page's data tag URL with the form serialized by `InitialPostData(actionType)`:
  `IF_ACTION=<Apply|…>&<field>=<value>…&_sessionTOKEN=<_sessionTmpToken>`. Multi-instance forms use
  `field:index` element ids. Fields marked `encode="1"` are AES-encrypted with a random
  key/IV, which is then sent RSA-encrypted (`encodeKey()`).
- Header `Check: <RSA-PKCS#1 v1.5(pubkey, hex(sha256(body)))>` (base64, JSEncrypt), with
  the RSA-2048 public key embedded in the main page (`asyEncode()`), because
  `commConf.IntegCheck` is true.

Actions seen in the UI: WLAN radio on/off and schedule (`wlan_wlanbasiconoff_lua.lua`, page
`wlanBasic`), SSID/radio settings, reboot and factory reset (page `rebootAndReset`:
`devmgr_restartmgr`, `db_resetmgr`), password change (`accountMgr`), language switch
(`hiddenData&_tag=switchlang_entry`), and mode switch (`loginData&_tag=modeswitch_entry`). None is
implemented: the forms are stateful (the apply handlers re-post the whole multi-instance
form, including the schedule), and they could not be verified without changing the ISP
gateway. A future implementation must mark reboot and reset as `routerkit.Destructive`.

## Limits and quirks

- **One web session**: the device allows a single logged-in web session. Holding it
  can lock out the owner's browser, so the client logs out on `Close`/`Logout`.
- **Idle timeout**: `Timeout` in `web_login_timeout_lua.lua` (5 min here). After it,
  data tags answer `SessionTimeout` and pages answer 404. The client logs in again once,
  automatically.
- **Lockout**: after a few failed logins (threshold not tested), `login_entry`
  reports `lockingTime` (seconds) and a `promptMsg`, and the UI disables the form. The
  client never retries a rejected login on its own (the error is sticky until
  `Login` is called), and it honors `lockingTime`.
- Page HTML is large (14–190 KB). The client caches the current page and only reloads it
  when it switches pages, so repeated reads of the same page cost one request each.
- `CpuUsage1..4`: one value per core. `MemUsage`: percent used.

## Captures and fixtures

Raw captures of a real device hold the password digest, cookies, session
tokens, MACs, serials, SSIDs, host names, the PPPoE user and ISP-side
addresses. They live only in the git-ignored `testdata/local/<device>/`
(here `testdata/local/zte-f8648p/`: `raw/login.json`, `raw/data/<tag>.xml`
for every `menuData` reply, plus pages, headers and the cookie jar). What
gets committed is built from them by the two scripts in
[`scripts/`](scripts/) (stdlib only; run them with `python3 -I`):

```
cd routerkit/brands/zte
python3 -I scripts/sanitize.py testdata/local/zte-f8648p/raw \
    ztefake/testdata/F8648P/V2.0.12P7N17/session.jsonl
python3 -I scripts/leakscan.py testdata/local/zte-f8648p \
    ztefake/testdata/F8648P/V2.0.12P7N17/session.jsonl \
    ztefake/testdata/*.xml ztefake/testdata/*.json ../../drivers/zte/testdata/*/*.json
```

- `sanitize.py` keeps only the data tags, field by field (MACs to
  `02:00:5e:00:xx:xx`, ISP-side IPv4 to documentation or remapped private
  ranges, every IPv6 to `2001:db8::/32` or a short `fe80::`, serials and
  ISP names to fixed placeholders, SSIDs and host names to numbered ones,
  secrets and `<encode>` values emptied), replaces the UI pages by
  minimal pages that carry a fake page token, and synthesizes the login
  answers the capture lacks. `--login-page` and `--model` adapt it to
  another model.
- `leakscan.py` harvests every private value of the raw capture (in
  every spelling: MAC separators, IPv6 forms and /48 prefixes, serial
  prefixes, decoded page tokens) and fails when one shows up in the
  scanned files, or when they hold a MAC or address outside the ranges
  the sanitizer maps to. Run it on the session and on every golden or
  fixture built from it before committing.

The replayed session is `ztefake.Session()`; the drivers' conformance
test runs the suite over it.
