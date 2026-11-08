package zte

import (
	"context"
	"fmt"
	"strings"
)

// Source is one native data endpoint: a data tag and the UI page (menu
// view) that must be loaded before the device serves it.
type Source struct {
	Name string `json:"name"` // short name, also accepted by Get
	View string `json:"view"` // menuView tag (menu id)
	Tag  string `json:"tag"`  // menuData tag, with its fixed query parameters
	Help string `json:"help"`
	// Verified: read from a real F8648P (V2.0.12P7N17, "user" account)
	Verified bool `json:"verified"`
}

// UI pages (menu ids) used by the typed reads.
const (
	viewStatusMgr   = "statusMgr"
	viewWAN         = "ethWanStatus"
	viewLocalNet    = "localNetStatus"
	viewWLANBasic   = "wlanBasic"
	viewTopology    = "mmTopology"
	viewHome        = "homePage"
	viewAccount     = "accountMgr"
	viewPONOptical  = "ponopticalinfo"
	probeView       = viewStatusMgr
	tagStatusMgr    = "devmgr_statusmgr_lua.lua"
	tagWAN          = "wan_internetstatus_lua.lua&TypeUplink=2&pageType=1"
	tagLANInfo      = "status_lan_info_lua.lua"
	tagWLANStatus   = "wlan_wlanstatus_lua.lua"
	tagWLANClients  = "wlan_client_stat_lua.lua"
	tagLANDevs      = "accessdev_landevs_lua.lua"
	tagUSBDevs      = "usb_usbdevs_lua.lua"
	tagWLANOnOff    = "wlan_wlanbasiconoff_lua.lua"
	tagWLANRadio    = "wlan_wlanbasicadconf_lua.lua"
	tagWLANSSID     = "wlan_wlansssidconf_lua.lua"
	tagTopology     = "topo_lua.lua"
	tagHomeDevs     = "accessdev_homepage_lua.lua&InstNum=5"
	tagHomeWLAN     = "wlan_homepage_lua.lua&InstNum=5"
	tagLoginTimeout = "web_login_timeout_lua.lua"
	tagPONOptical   = "optical_info_lua.lua"
)

// Sources lists the known data endpoints (read-only GETs).
var Sources = []Source{
	{"devmgr_statusmgr", viewStatusMgr, tagStatusMgr, "device info (model, versions, serial), CPU/memory usage, power-on time", true},
	{"wan_internetstatus", viewWAN, tagWAN, "WAN connections: status, IPv4/IPv6, gateway, DNS, protocol, uptime, PPP counters", true},
	{"status_lan_info", viewLocalNet, tagLANInfo, "Ethernet ports: link, speed, duplex, byte/packet/error counters", true},
	{"wlan_wlanstatus", viewLocalNet, tagWLANStatus, "WLAN SSIDs: enable, security, BSSID, channel in use, counters", true},
	{"wlan_client_stat", viewLocalNet, tagWLANClients, "associated WiFi clients (MAC, IP, RSSI, rates, link time)", true},
	{"accessdev_landevs", viewLocalNet, tagLANDevs, "wired LAN clients (MAC, IP, host name, port)", true},
	{"usb_usbdevs", viewLocalNet, tagUSBDevs, "USB storage devices", false},
	{"wlan_wlanbasiconoff", viewWLANBasic, tagWLANOnOff, "WLAN radio on/off and schedule", true},
	{"wlan_wlanbasicadconf", viewWLANBasic, tagWLANRadio, "WLAN radio settings: channel, bandwidth, standard, TX power, channel lists", true},
	{"wlan_wlansssidconf", viewWLANBasic, tagWLANSSID, "SSID settings (keys are redacted unless KeepSecrets)", true},
	{"topo", viewTopology, tagTopology, "topology / EasyMesh controller (JSON): LAN MAC, IP, client count", true},
	{"accessdev_homepage", viewHome, tagHomeDevs, "home page client summary", true},
	{"wlan_homepage", viewHome, tagHomeWLAN, "home page WLAN summary", true},
	{"web_login_timeout", viewAccount, tagLoginTimeout, "web session idle timeout (minutes)", true},
	{"optical_info", viewPONOptical, tagPONOptical, "PON optical module: RX/TX power, temperature, voltage, bias current (admin accounts; not exposed to \"user\")", false},
}

// tagName strips the fixed query parameters of a tag.
func tagName(tag string) string {
	if i := strings.IndexByte(tag, '&'); i >= 0 {
		return tag[:i]
	}
	return tag
}

// lookupSource resolves a Get argument: a source name, a data tag
// ("devmgr_statusmgr_lua.lua"), or "view/tag" for anything else.
func lookupSource(name string) (Source, error) {
	for _, s := range Sources {
		if name == s.Name || name == tagName(s.Tag) || name == s.Tag {
			return s, nil
		}
	}
	if view, tag, ok := strings.Cut(name, "/"); ok && view != "" && tag != "" {
		if strings.ContainsAny(view+tag, "?# ") {
			return Source{}, fmt.Errorf("zte: invalid source %q", name)
		}
		return Source{Name: name, View: view, Tag: tag}, nil
	}
	return Source{}, fmt.Errorf("%w %q (use a known name or \"view/tag\")", ErrUnknownSource, name)
}

// Get reads one native data endpoint and returns the parsed instances.
// name is a Sources name ("devmgr_statusmgr"), its tag
// ("devmgr_statusmgr_lua.lua"), or "view/tag" for an endpoint not in
// Sources (e.g. "statusMgr/devmgr_statusmgr_lua.lua"). Only GETs are
// sent. Credential-like values are blanked unless Options.KeepSecrets.
func (c *Client) Get(ctx context.Context, name string) (*Response, error) {
	src, err := lookupSource(name)
	if err != nil {
		return nil, err
	}
	rs, err := c.fetch(ctx, src.View, src.Tag)
	if err != nil {
		return nil, err
	}
	return rs[0], nil
}
