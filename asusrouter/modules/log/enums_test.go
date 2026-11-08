package log

import "testing"

// Ported from tests/modules/log/test_enums.py.

func TestFieldValues(t *testing.T) {
	cases := map[ARLogField]string{
		ARLogFieldEntryCount:  "entry_count",
		ARLogFieldEntryList:   "entry_list",
		ARLogFieldLastEntry:   "last_entry",
		ARLogFieldProgramList: "program_list",
	}
	for field, want := range cases {
		if string(field) != want {
			t.Errorf("%s = %q, want %q", field.Name(), field, want)
		}
	}
}

func TestFieldUnknownFallback(t *testing.T) {
	if got := ARLogFieldFromValue("nope"); got != ARLogFieldUnknown {
		t.Errorf("got %q", got)
	}
}

func TestProgramKnownValues(t *testing.T) {
	cases := []struct {
		value string
		want  ARProgram
	}{
		{"miniupnpd", ARProgramMiniupnp},
		{"kernel", ARProgramKernel},
		{"dnsmasq-dhcp", ARProgramDnsmasqDhcp},
		{"dhcp client", ARProgramDhcpClient},
		{"hour_monitor", ARProgramHourMonitor},
		{"disk_monitor", ARProgramDiskMonitor},
		{"httpd", ARProgramHttpDaemon},
		{"roamast", ARProgramRoamingAssistant},
	}
	for _, tc := range cases {
		if got := ARProgramFromValue(tc.value); got != tc.want {
			t.Errorf("%q: got %q, want %q", tc.value, got, tc.want)
		}
	}
}

func TestProgramUnknownValues(t *testing.T) {
	for _, value := range []string{"ARK", "nosuchd", ""} {
		if got := ARProgramFromValue(value); got != ARProgramUnknown {
			t.Errorf("%q: got %q", value, got)
		}
	}
}
