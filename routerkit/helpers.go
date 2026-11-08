package routerkit

// Helpers shared by drivers and the CLI (a compatible extension of the
// contract in driver.go).

import (
	"fmt"
	"net"
	"strings"
)

// FindAction returns the spec of a named action.
func FindAction(specs []ActionSpec, name string) (ActionSpec, bool) {
	for _, s := range specs {
		if s.Name == name {
			return s, true
		}
	}
	return ActionSpec{}, false
}

// ParseOnOff reads on/off style values (on, off, true, false, 1, 0,
// yes, no, enable, disable).
func ParseOnOff(v string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "on", "true", "1", "yes", "enable", "enabled":
		return true, nil
	case "off", "false", "0", "no", "disable", "disabled":
		return false, nil
	}
	return false, fmt.Errorf("%q is not on/off", v)
}

// NormalizeMAC returns a lower-case, colon-separated MAC, or "" when the
// value is not a MAC address.
func NormalizeMAC(s string) string {
	s = strings.TrimSpace(s)
	if len(s) == 12 && !strings.ContainsAny(s, ":-.") {
		var b strings.Builder
		for i := 0; i < 12; i += 2 {
			if i > 0 {
				b.WriteByte(':')
			}
			b.WriteString(s[i : i+2])
		}
		s = b.String()
	}
	hw, err := net.ParseMAC(s)
	if err != nil || len(hw) != 6 {
		return ""
	}
	return hw.String()
}

// ConnectionLabel normalizes a connection name into the Client.Connection
// vocabulary ("wired", "2.4ghz", "5ghz", "6ghz", "wifi", "").
func ConnectionLabel(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "wired", "ethernet", "lan", "eth":
		return "wired"
	case "2g", "2.4g", "2.4ghz", "2g1", "2g2", "2.4":
		return "2.4ghz"
	case "5g", "5ghz", "5g1", "5g2", "5":
		return "5ghz"
	case "6g", "6ghz", "6g1", "6g2", "6":
		return "6ghz"
	case "wifi", "wireless", "wlan", "wl":
		return "wifi"
	}
	return ""
}
