package probe

import "testing"

func TestScrubText(t *testing.T) {
	cases := []struct{ text, expected string }{
		// Nothing to hide
		{"start NTP update", "start NTP update"},
		{"", ""},
		// MAC, both separators and both cases
		{"Assoc AA:BB:CC:DD:EE:FF done", "Assoc " + RedactedMAC + " done"},
		{"deauth aa-bb-cc-dd-ee-ff", "deauth " + RedactedMAC},
		// IPv4
		{"bound 192.168.1.15/24", "bound " + RedactedIP + "/24"},
		{"connect(10.0.0.1:2869)", "connect(" + RedactedIP + ":2869)"},
		// IPv6
		{"peer 2001:0db8:0000:0000:0000:ff00:0042:8329 up", "peer " + RedactedIP + " up"},
		// A time of day is not an address
		{"restarted at 12:43:08 today", "restarted at 12:43:08 today"},
		// Several identifiers in one message
		{"AA:BB:CC:DD:EE:FF asked 192.168.1.1", RedactedMAC + " asked " + RedactedIP},
		// Python's Unicode \b and \d (checked against CPython)
		{"éAA:BB:CC:DD:EE:FF", "éAA" + RedactedIP},
		{"→AA:BB:CC:DD:EE:FF ip 1.2.3.4é", "→" + RedactedMAC + " ip 1.2.3.4é"},
		{"ip ١٩٢.١٦٨.١.١ ok", "ip " + RedactedIP + " ok"},
	}
	for _, c := range cases {
		if got := ScrubText(c.text); got != c.expected {
			t.Errorf("ScrubText(%q) = %q, want %q", c.text, got, c.expected)
		}
	}
}

func TestScrubTextIdempotent(t *testing.T) {
	once := ScrubText("client AA:BB:CC:DD:EE:FF at 192.168.1.5")
	if ScrubText(once) != once {
		t.Fatal("not idempotent")
	}
}
