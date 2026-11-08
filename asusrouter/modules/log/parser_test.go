package log

import (
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"
)

// Ported from tests/modules/log/test_parser.py.

const parserRaw = "{\n\"nvram_dump-syslog.log\":Jul 21 15:08:01 kernel: first\n" +
	"Jul 21 15:08:02 wlceventd: second\n}"

func intPtr(v int) *int { return &v }

func strPtr(s string) *string { return &s }

func sameProgram(a, b *ARLogProgram) bool {
	if a == nil || b == nil {
		return a == b
	}
	if a.Name != b.Name || (a.Pid == nil) != (b.Pid == nil) {
		return false
	}
	return a.Pid == nil || *a.Pid == *b.Pid
}

func TestStripEnvelope(t *testing.T) {
	t.Run("wrapper_removed", func(t *testing.T) {
		text := StripEnvelope(parserRaw)
		if !strings.HasPrefix(text, "Jul 21 15:08:01 kernel: first") ||
			strings.Contains(text, "nvram_dump") || strings.HasSuffix(text, "}") {
			t.Errorf("got %q", text)
		}
	})

	t.Run("raw_passthrough", func(t *testing.T) {
		raw := "Jul 21 15:08:01 kernel: first"
		if got := StripEnvelope(raw); got != raw {
			t.Errorf("got %q", got)
		}
	})

	for _, content := range []any{nil, "", "   "} {
		t.Run(fmt.Sprintf("empty_%v", content), func(t *testing.T) {
			if got := StripEnvelope(content); got != "" {
				t.Errorf("got %q", got)
			}
		})
	}

	t.Run("colour_escapes_removed", func(t *testing.T) {
		raw := "Jul 21 15:08:01 kernel: \x1b[0;30;103mWarning: Serdes\x1b[0m"
		if got := StripEnvelope(raw); got != "Jul 21 15:08:01 kernel: Warning: Serdes" {
			t.Errorf("got %q", got)
		}
	})

	t.Run("colour_escapes_reach_the_entry", func(t *testing.T) {
		raw := "Jul 21 15:08:01 kernel: \x1b[0;30;103mWarning: Serdes"
		if got := ParseLog(raw, nil)[0].ContentValue(); got != "Warning: Serdes" {
			t.Errorf("got %q", got)
		}
	})

	t.Run("end_of_buffer_padding_removed", func(t *testing.T) {
		raw := "{\n\"nvram_dump-syslog.log\":Jul 21 15:08:01 ntp: a\n\n}"
		if got := StripEnvelope(raw); got != "Jul 21 15:08:01 ntp: a" {
			t.Errorf("got %q", got)
		}
	})
}

func TestParseLog(t *testing.T) {
	t.Run("wrapped_reply", func(t *testing.T) {
		entries := ParseLog(parserRaw, nil)
		want := []struct{ stamp, program, content string }{
			{"Jul 21 15:08:01", "kernel", "first"},
			{"Jul 21 15:08:02", "wlceventd", "second"},
		}
		if len(entries) != len(want) {
			t.Fatalf("got %d entries", len(entries))
		}
		for i, w := range want {
			e := entries[i]
			if e.TimestampStr != w.stamp || !sameProgram(e.Program, &ARLogProgram{Name: w.program}) ||
				e.ContentValue() != w.content || e.Content.Level != EmptyContent.Level || e.Timestamp != nil {
				t.Errorf("entry %d: got %+v", i, e)
			}
		}
	})

	t.Run("space_padded_day", func(t *testing.T) {
		raw := "Aug  1 02:35:45 ntp: start NTP update\n" +
			"Aug  1 02:36:15 ntp: start NTP update"
		entries := ParseLog(raw, nil)
		if len(entries) != 2 || entries[0].TimestampStr != "Aug  1 02:35:45" ||
			entries[0].ContentValue() != "start NTP update" {
			t.Errorf("got %+v", entries)
		}
	})

	single := []struct {
		name    string
		raw     string
		stamp   string
		program *ARLogProgram
		content string
	}{
		{"timestamp_cut_from_message", "Jul 21 15:08:01 kernel: boot", "Jul 21 15:08:01", &ARLogProgram{Name: "kernel"}, "boot"},
		{"pid_kept_in_message", "Aug  1 02:43:59 miniupnpd[1337]: chain gone", "", &ARLogProgram{Name: "miniupnpd", Pid: intPtr(1337)}, "chain gone"},
		{"service_and_content_split", "Aug  1 02:35:45 ntp: start NTP update", "", &ARLogProgram{Name: "ntp"}, "start NTP update"},
		{"multi_word_service_split", "Jul 22 03:29:38 dhcp client: deconfig", "", &ARLogProgram{Name: "dhcp client"}, "deconfig"},
		{"content_with_colons", "Jul 21 15:08:02 wlceventd: proc(6): wl0: up", "", &ARLogProgram{Name: "wlceventd"}, "proc(6): wl0: up"},
		{"message_without_colon", "Jul 21 15:08:02 just a bare line", "", nil, "just a bare line"},
		{"freeform_syslogd_started", "Aug  3 18:19:46 syslogd started: BusyBox v1.25.1", "", &ARLogProgram{Name: "syslogd"}, "syslogd started: BusyBox v1.25.1"},
		{"freeform_syslogd_exiting", "Aug  1 03:41:52 syslogd exiting", "", &ARLogProgram{Name: "syslogd"}, "syslogd exiting"},
		{"real_syslogd_tag_still_splits", "Aug  1 03:41:52 syslogd: normal message", "", &ARLogProgram{Name: "syslogd"}, "normal message"},
		{"empty_service_name", "Jul 21 15:08:02 : orphan content", "", nil, "orphan content"},
	}
	for _, tc := range single {
		t.Run(tc.name, func(t *testing.T) {
			entries := ParseLog(tc.raw, nil)
			if len(entries) != 1 {
				t.Fatalf("got %d entries", len(entries))
			}
			e := entries[0]
			if tc.stamp != "" && e.TimestampStr != tc.stamp {
				t.Errorf("stamp %q", e.TimestampStr)
			}
			if !sameProgram(e.Program, tc.program) {
				t.Errorf("program %+v, want %+v", e.Program, tc.program)
			}
			if e.ContentValue() != tc.content {
				t.Errorf("content %q", e.ContentValue())
			}
		})
	}

	t.Run("multi_word_service_kept", func(t *testing.T) {
		raw := "Jul 22 03:29:25 WAN(0) Connection: link up " +
			"Jul 22 03:29:38 dhcp client: deconfig"
		entries := ParseLog(raw, nil)
		if len(entries) != 2 || entries[0].Program.Name != "WAN(0) Connection" ||
			entries[1].Program.Name != "dhcp client" {
			t.Errorf("got %+v", entries)
		}
	})

	t.Run("space_joined_records_split", func(t *testing.T) {
		raw := "Jul 22 08:53:29 vpnserver1[20330]: reset " +
			"Jul 22 08:53:31 ARK: ARK is blocked"
		entries := ParseLog(raw, nil)
		if len(entries) != 2 || !sameProgram(entries[0].Program, &ARLogProgram{Name: "vpnserver1", Pid: intPtr(20330)}) ||
			entries[1].ContentValue() != "ARK is blocked" {
			t.Errorf("got %+v", entries)
		}
	})

	t.Run("embedded_newline_absorbed", func(t *testing.T) {
		raw := "Jul 22 08:53:29 vpnserver1[20330]: WARNING bad packet\n" +
			"            <=1768 please ensure\n" +
			"Jul 22 08:53:31 ARK: blocked"
		entries := ParseLog(raw, nil)
		if len(entries) != 2 || !strings.Contains(entries[0].ContentValue(), "please ensure") ||
			entries[1].ContentValue() != "blocked" {
			t.Errorf("got %+v", entries)
		}
	})

	t.Run("order_preserved", func(t *testing.T) {
		entries := ParseLog("Aug  1 02:00:00 ntp: later Jul 31 23:59:59 ntp: earlier", nil)
		if len(entries) != 2 || entries[0].TimestampStr != "Aug  1 02:00:00" ||
			entries[1].TimestampStr != "Jul 31 23:59:59" {
			t.Errorf("got %+v", entries)
		}
	})

	for _, content := range []any{nil, "", "no timestamp here"} {
		t.Run(fmt.Sprintf("no_records_%v", content), func(t *testing.T) {
			if got := ParseLog(content, nil); len(got) != 0 {
				t.Errorf("got %+v", got)
			}
		})
	}

	t.Run("program_shared_between_entries", func(t *testing.T) {
		raw := "Jul 22 08:43:21 ntp: start NTP update\n" +
			"Jul 22 08:43:22 ntp: start NTP update\n" +
			"Jul 22 08:43:23 httpd[123]: other\n"
		entries := ParseLog(raw, nil)
		if entries[0].Program != entries[1].Program || entries[0].Program == entries[2].Program {
			t.Error("programs not shared as expected")
		}
	})

	t.Run("pid_keeps_emitters_apart", func(t *testing.T) {
		entries := ParseLog("Jul 22 08:43:21 httpd[1]: a\nJul 22 08:43:22 httpd[2]: b\n", nil)
		if entries[0].Program == entries[1].Program || sameProgram(entries[0].Program, entries[1].Program) {
			t.Error("emitters not kept apart")
		}
	})
}

func TestParseProgram(t *testing.T) {
	cases := []struct {
		name  string
		token string
		kind  ARProgram
		index *int
	}{
		{"plain_program", "miniupnpd", ARProgramMiniupnp, nil},
		{"indexed_program", "vpnclient5", ARProgramVpnClient, intPtr(5)},
		{"indexed_server", "vpnserver2", ARProgramVpnServer, intPtr(2)},
		{"unknown_indexed_keeps_no_index", "foo3", ARProgramUnknown, nil},
		{"hyphenated_not_indexed", "dnsmasq-dhcp", ARProgramDnsmasqDhcp, nil},
		{"unknown_plain", "nosuchd", ARProgramUnknown, nil},
		{"alias_semantic", "amas_lib", ARProgramRcService, nil},
		{"alias_case", "HTTPD", ARProgramHttpDaemon, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			kind, index := ParseProgram(tc.token)
			if kind != tc.kind || (index == nil) != (tc.index == nil) || (index != nil && *index != *tc.index) {
				t.Errorf("got (%q, %v)", kind, index)
			}
		})
	}
}

// parserText builds a run of distinct records as raw log text.
func parserText(first, last int) string {
	lines := make([]string, 0, last-first)
	for index := first; index < last; index++ {
		lines = append(lines, fmt.Sprintf("Jul 21 %02d:%02d:%02d kernel: line %d",
			index/3600, index/60%60, index%60, index))
	}
	return strings.Join(lines, "\n")
}

// runeSlice mirrors Python `text[offset:]` for a character offset.
func runeSlice(text string, offset int) string {
	return string([]rune(text)[offset:])
}

func TestAnchorOf(t *testing.T) {
	t.Run("short_text_kept_whole", func(t *testing.T) {
		text := parserText(0, 5)
		if AnchorOf(text) != text {
			t.Error("not kept whole")
		}
	})

	t.Run("long_text_trimmed", func(t *testing.T) {
		text := parserText(0, 2000)
		anchor := AnchorOf(text)
		if utf8.RuneCountInString(anchor) != AnchorLength || !strings.HasSuffix(text, anchor) {
			t.Errorf("anchor of %d characters", utf8.RuneCountInString(anchor))
		}
	})

	// Go-only: the window counts characters, not bytes
	t.Run("counts_characters", func(t *testing.T) {
		text := strings.Repeat("é", AnchorLength+10)
		if anchor := AnchorOf(text); utf8.RuneCountInString(anchor) != AnchorLength {
			t.Errorf("anchor of %d characters", utf8.RuneCountInString(anchor))
		}
	})
}

func TestFindContinuation(t *testing.T) {
	t.Run("whole_anchor_present", func(t *testing.T) {
		previous := parserText(0, 40)
		text := previous + "\n" + parserText(40, 45)
		offset := FindContinuation(text, previous)
		if offset == nil || !strings.HasPrefix(strings.TrimSpace(runeSlice(text, *offset)), "Jul 21 00:00:40") {
			t.Errorf("got %v", offset)
		}
	})

	t.Run("anchor_partly_trimmed", func(t *testing.T) {
		previous := parserText(0, 40)
		text := parserText(20, 60)
		offset := FindContinuation(text, previous)
		if offset == nil || !strings.HasPrefix(strings.TrimSpace(runeSlice(text, *offset)), "Jul 21 00:00:40") {
			t.Errorf("got %v", offset)
		}
	})

	nones := []struct{ name, text, anchor string }{
		{"nothing_in_common", "aaa", "zzz"},
		// both end on the same few characters, sharing no whole record
		{"chance_overlap_rejected", "Jul 21 00:00:09 ntp: x", "zzz x"},
		{"no_overlap", parserText(500, 540), parserText(0, 40)},
		{"empty_anchor", parserText(0, 5), ""},
	}
	for _, tc := range nones {
		t.Run(tc.name, func(t *testing.T) {
			if got := FindContinuation(tc.text, tc.anchor); got != nil {
				t.Errorf("got %d", *got)
			}
		})
	}

	t.Run("unchanged_log", func(t *testing.T) {
		text := parserText(0, 40)
		if got := FindContinuation(text, text); got == nil || *got != len(text) {
			t.Errorf("got %v", got)
		}
	})

	t.Run("repeated_message_does_not_mislead", func(t *testing.T) {
		spam := func(first, last int) string {
			lines := []string{}
			for index := first; index < last; index++ {
				lines = append(lines, fmt.Sprintf("Jul 21 00:%02d:%02d ntp: start NTP update", index/60, index%60))
			}
			return strings.Join(lines, "\n")
		}
		previous := spam(0, 400)
		text := previous + "\n" + spam(400, 405)
		offset := FindContinuation(text, previous)
		if offset == nil || len(ParseText(runeSlice(text, *offset), nil)) != 5 {
			t.Errorf("got %v", offset)
		}
	})

	t.Run("identical_records_are_treated_as_seen", func(t *testing.T) {
		line := "Jul 21 00:00:01 ntp: start NTP update"
		previous := strings.Join(repeat(line, 400), "\n")
		text := previous + "\n" + strings.Join(repeat(line, 5), "\n")
		if got := FindContinuation(text, previous); got == nil || *got != len(text) {
			t.Errorf("got %v, want %d", got, len(text))
		}
	})

	// Go-only: offsets are character offsets, like Python's
	t.Run("character_offsets", func(t *testing.T) {
		text := "Jul 21 00:00:01 ntp: é\nJul 21 00:00:02 ntp: b"
		anchor := "Jul 21 00:00:01 ntp: é"
		if got := FindContinuation(text, anchor); got == nil || *got != utf8.RuneCountInString(anchor) {
			t.Errorf("got %v", got)
		}
	})
}

func repeat(s string, n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = s
	}
	return out
}

func TestParseContinued(t *testing.T) {
	t.Run("after_reads_the_tail_only", func(t *testing.T) {
		previous := parserText(0, 40)
		raw := `{"nvram_dump-syslog.log":` + previous + "\n" + parserText(40, 45) + "}"
		entries := ParseLog(raw, strPtr(previous))
		if len(entries) != 5 {
			t.Fatalf("got %d entries", len(entries))
		}
		for i, e := range entries {
			if want := fmt.Sprintf("line %d", 40+i); e.ContentValue() != want {
				t.Errorf("entry %d: %q", i, e.ContentValue())
			}
		}
	})

	t.Run("unusable_anchor_reads_everything", func(t *testing.T) {
		raw := `{"nvram_dump-syslog.log":` + parserText(0, 10) + "}"
		if got := ParseLog(raw, strPtr(parserText(900, 940))); len(got) != 10 {
			t.Errorf("got %d entries", len(got))
		}
	})
}

func TestUntaggedProgram(t *testing.T) {
	t.Run("empty_tag_recovered", func(t *testing.T) {
		entry := ParseLog("Jul 21 15:08:01 : fwver: 3.0.0.4 (sn:X /ha:Y )", nil)[0]
		if !sameProgram(entry.Program, &ARLogProgram{Name: "init"}) ||
			!strings.HasPrefix(entry.ContentValue(), "fwver: 3.0.0.4") {
			t.Errorf("got %+v", entry)
		}
	})

	t.Run("both_spellings_agree", func(t *testing.T) {
		untagged := ParseLog("Jul 21 15:08:01 : fwver: 3.0.0.4", nil)[0]
		tagged := ParseLog("Jul 21 15:08:01 init: fwver: 3.0.0.4", nil)[0]
		if !sameProgram(untagged.Program, tagged.Program) || untagged.ContentValue() != tagged.ContentValue() {
			t.Errorf("got %+v vs %+v", untagged, tagged)
		}
	})

	t.Run("other_empty_tags_stay_untagged", func(t *testing.T) {
		entry := ParseLog("Jul 21 15:08:01 : something else entirely", nil)[0]
		if entry.Program != nil || entry.ContentValue() != "something else entirely" {
			t.Errorf("got %+v", entry)
		}
	})
}

// Go-only: the RE2 rewrites keep Python's Unicode semantics
func TestParserUnicode(t *testing.T) {
	t.Run("unicode_space_after_stamp", func(t *testing.T) {
		// Python `\s` takes U+00A0 (and `str.rstrip` strips it)
		entries := ParseLog("Jul 21 15:08:01 kernel: boot ", nil)
		if len(entries) != 1 || entries[0].ContentValue() != "boot" {
			t.Errorf("got %+v", entries)
		}
	})

	t.Run("unicode_digit_pid", func(t *testing.T) {
		// Python `\d` and int() take any decimal digit
		entries := ParseLog("Jul 21 15:08:01 httpd[١٢]: x", nil)
		if len(entries) != 1 || !sameProgram(entries[0].Program, &ARLogProgram{Name: "httpd", Pid: intPtr(12)}) {
			t.Errorf("got %+v", entries[0].Program)
		}
	})
}
