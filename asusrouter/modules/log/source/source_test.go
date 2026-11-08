package source

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/eduardpaul/routerctl/asusrouter/config"
	"github.com/eduardpaul/routerctl/asusrouter/consts"
	"github.com/eduardpaul/routerctl/asusrouter/modules/clock"
	"github.com/eduardpaul/routerctl/asusrouter/modules/device"
	"github.com/eduardpaul/routerctl/asusrouter/modules/endpoint"
	"github.com/eduardpaul/routerctl/asusrouter/modules/log"
	"github.com/eduardpaul/routerctl/asusrouter/modules/nvram"
	"github.com/eduardpaul/routerctl/asusrouter/modules/source"
	"github.com/eduardpaul/routerctl/asusrouter/registry"
	"github.com/eduardpaul/routerctl/asusrouter/tools/identifiers"
	"github.com/eduardpaul/routerctl/asusrouter/tools/security"
)

// Ported from tests/modules/log/test_source.py.

const (
	raw = `{"nvram_dump-syslog.log":Jul 21 15:08:01 kernel: boot}`
	// A cleared log: the wrapper is served, it just holds no records
	rawEmpty = `{"nvram_dump-syslog.log":}`
)

// rawFetcher is a fetch_raw_callback serving one reply.
type rawFetcher struct {
	reply    *string
	calls    int
	endpoint endpoint.AREndpoint
	request  string
}

func (f *rawFetcher) fetch(_ context.Context, ep endpoint.AREndpoint, request string) (*string, error) {
	f.calls++
	f.endpoint, f.request = ep, request
	return f.reply, nil
}

func reply(s string) *rawFetcher { return &rawFetcher{reply: &s} }

// dataFetcher records the fetch_data_callback calls.
type dataFetcher struct{ requested []source.DataSource }

func (f *dataFetcher) fetch(_ context.Context, src source.DataSource, _ registry.Kwargs) (any, error) {
	f.requested = append(f.requested, src)
	return nil, nil
}

func fetch(t *testing.T, c *registry.Context) string {
	t.Helper()
	result, err := fetchState(context.Background(), c, &ARLogSource{})
	if err != nil {
		t.Fatal(err)
	}
	return result.(string)
}

func identityAt(deviceTime time.Time) *device.ARDeviceIdentity {
	identity := device.NewARDeviceIdentity()
	identity.UpdateDeviceTime(&deviceTime)
	return identity
}

func TestFetchState(t *testing.T) {
	t.Run("fetched", func(t *testing.T) {
		f := reply(raw)
		result := fetch(t, &registry.Context{FetchRaw: f.fetch})
		if !strings.Contains(result, "kernel: boot") || f.calls != 1 || f.endpoint != endpoint.AREndpointFetchData {
			t.Errorf("got %q after %d calls to %q", result, f.calls, f.endpoint)
		}
	})

	t.Run("clock_read_when_identity_has_none", func(t *testing.T) {
		data := &dataFetcher{}
		fetch(t, &registry.Context{FetchRaw: reply(raw).fetch, FetchData: data.fetch, Identity: device.NewARDeviceIdentity()})
		if len(data.requested) != 1 || data.requested[0] != clock.ARClockSourceUniversal {
			t.Errorf("requested %v", data.requested)
		}
	})

	t.Run("clock_not_reread_when_current", func(t *testing.T) {
		data := &dataFetcher{}
		identity := identityAt(time.Now().UTC().Add(-2 * time.Hour))
		fetch(t, &registry.Context{FetchRaw: reply(raw).fetch, FetchData: data.fetch, Identity: identity})
		if len(data.requested) != 0 {
			t.Errorf("requested %v", data.requested)
		}
	})

	for name, deviceTime := range map[string]time.Time{
		"never_synced": time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC),
		"far_ahead":    time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC),
		// A clock with no offset cannot be measured against the host
		"states_no_offset": time.Now().In(consts.NaiveLocation),
	} {
		t.Run("clock_reread_"+name, func(t *testing.T) {
			data := &dataFetcher{}
			fetch(t, &registry.Context{FetchRaw: reply(raw).fetch, FetchData: data.fetch, Identity: identityAt(deviceTime)})
			if len(data.requested) != 1 || data.requested[0] != clock.ARClockSourceUniversal {
				t.Errorf("requested %v", data.requested)
			}
		})
	}

	t.Run("no_raw_callback", func(t *testing.T) {
		if result := fetch(t, &registry.Context{}); result != "" {
			t.Errorf("got %q", result)
		}
	})

	t.Run("empty_reply", func(t *testing.T) {
		if result := fetch(t, &registry.Context{FetchRaw: (&rawFetcher{}).fetch}); result != "" {
			t.Errorf("got %q", result)
		}
	})

	t.Run("requested_hook", func(t *testing.T) {
		f := reply(raw)
		fetch(t, &registry.Context{FetchRaw: f.fetch})
		if !strings.Contains(f.request, string(LogHook.Hook)) || !strings.Contains(f.request, "syslog.log") {
			t.Errorf("request %q", f.request)
		}
	})

	t.Run("wrapper_without_records", func(t *testing.T) {
		f := reply(rawEmpty)
		if result := fetch(t, &registry.Context{FetchRaw: f.fetch}); result != rawEmpty || f.calls != 1 {
			t.Errorf("got %q", result)
		}
	})
}

type tcOptions struct {
	identity *device.ARDeviceIdentity
	previous any
	config   any
}

func translate(data any, opts tcOptions) map[log.ARLogField]any {
	tc := &registry.TranslateContext{Identity: opts.identity, Previous: opts.previous}
	if opts.config != nil {
		tc.Kwargs = registry.Kwargs{"config": opts.config}
	}
	return translateState(data, tc).(map[log.ARLogField]any)
}

func entriesOf(state map[log.ARLogField]any) []*log.ARLogEntry {
	return state[log.ARLogFieldEntryList].([]*log.ARLogEntry)
}

func anchorOf(state map[log.ARLogField]any) security.SensitiveText {
	return state[log.ARLogFieldAnchor].(security.SensitiveText)
}

func TestTranslateState(t *testing.T) {
	t.Run("parses_text", func(t *testing.T) {
		result := translate(raw, tcOptions{})
		entries := entriesOf(result)
		if len(entries) != 1 {
			t.Fatalf("got %d entries", len(entries))
		}
		entry := entries[0]
		if entry.TimestampStr != "Jul 21 15:08:01" || entry.Program.Name != "kernel" || entry.Program.Pid != nil ||
			entry.ContentValue() != "boot" {
			t.Errorf("entry %+v", entry)
		}
		// translate resolves the datetime, unlike bare parse_log
		if entry.Timestamp == nil {
			t.Fatal("no timestamp")
		}
		// monitor signals
		if result[log.ARLogFieldLastEntry].(*time.Time) != entry.Timestamp || result[log.ARLogFieldEntryCount] != 1 {
			t.Errorf("signals %v", result)
		}
	})

	t.Run("no_program_list", func(t *testing.T) {
		if _, ok := translate(raw, tcOptions{})[log.ARLogFieldProgramList]; ok {
			t.Error("program list present")
		}
	})

	t.Run("non_string", func(t *testing.T) {
		result := translate(map[string]any{"unexpected": true}, tcOptions{})
		if len(entriesOf(result)) != 0 || result[log.ARLogFieldLastEntry].(*time.Time) != nil || result[log.ARLogFieldEntryCount] != 0 {
			t.Errorf("got %v", result)
		}
	})
}

// logReply wraps raw records the way the device serves them: the device
// closes its buffer with a blank line.
func logReply(records ...string) string {
	return `{"nvram_dump-syslog.log":` + strings.Join(records, "\n") + "\n\n}"
}

// records builds a run of distinct records.
func records(first, last int) []string {
	out := make([]string, 0, last-first)
	for index := first; index < last; index++ {
		out = append(out, fmt.Sprintf("Jul 21 %02d:%02d:%02d kernel: line %d",
			index/3600, index/60%60, index%60, index))
	}
	return out
}

func setHistoryLimit(t *testing.T, limit int) {
	t.Helper()
	if err := config.ARConfig.Set(config.ARConfigKeyLogHistoryLimit, limit); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = config.ARConfig.Set(config.ARConfigKeyLogHistoryLimit, 50000) })
}

func TestContinuedRead(t *testing.T) {
	t.Run("appends_new_records", func(t *testing.T) {
		first := translate(logReply(records(0, 40)...), tcOptions{})
		second := translate(logReply(records(0, 45)...), tcOptions{previous: first})
		entries := entriesOf(second)
		if len(entries) != 45 || entries[0].ContentValue() != "line 0" || entries[44].ContentValue() != "line 44" {
			t.Errorf("got %d entries", len(entries))
		}
	})

	t.Run("unchanged_log_adds_nothing", func(t *testing.T) {
		r := logReply(records(0, 40)...)
		second := translate(r, tcOptions{previous: translate(r, tcOptions{})})
		if second[log.ARLogFieldEntryCount] != 40 {
			t.Errorf("got %v", second[log.ARLogFieldEntryCount])
		}
	})

	t.Run("keeps_entries_the_device_dropped", func(t *testing.T) {
		first := translate(logReply(records(0, 40)...), tcOptions{})
		// the device dropped the oldest records and added new ones
		entries := entriesOf(translate(logReply(records(20, 60)...), tcOptions{previous: first}))
		if len(entries) != 60 || entries[0].ContentValue() != "line 0" || entries[59].ContentValue() != "line 59" {
			t.Errorf("got %d entries", len(entries))
		}
	})

	t.Run("rollover_without_overlap", func(t *testing.T) {
		first := translate(logReply(records(0, 40)...), tcOptions{})
		entries := entriesOf(translate(logReply(records(500, 540)...), tcOptions{previous: first}))
		if len(entries) != 80 || entries[0].ContentValue() != "line 0" || entries[79].ContentValue() != "line 539" {
			t.Errorf("got %d entries", len(entries))
		}
	})

	t.Run("anchor_carried_for_the_next_read", func(t *testing.T) {
		anchor := anchorOf(translate(logReply(records(0, 40)...), tcOptions{}))
		// A slice of the log, so classified like any untranslated text
		if anchor.Level != security.ARSecurityLevelUnsafe || anchor.PyStr() == anchor.Value ||
			!strings.HasSuffix(anchor.Value, "kernel: line 39") {
			t.Errorf("anchor %+v", anchor)
		}
	})

	t.Run("previous_ignored_when_unusable", func(t *testing.T) {
		if got := translate(logReply(records(0, 5)...), tcOptions{previous: "nonsense"}); got[log.ARLogFieldEntryCount] != 5 {
			t.Errorf("got %v", got[log.ARLogFieldEntryCount])
		}
	})

	t.Run("history_limit", func(t *testing.T) {
		setHistoryLimit(t, 50)
		first := translate(logReply(records(0, 40)...), tcOptions{})
		entries := entriesOf(translate(logReply(records(0, 80)...), tcOptions{previous: first}))
		if len(entries) != 50 || entries[0].ContentValue() != "line 30" || entries[49].ContentValue() != "line 79" {
			t.Errorf("got %d entries", len(entries))
		}
	})

	t.Run("history_limit_zero_keeps_everything", func(t *testing.T) {
		setHistoryLimit(t, 0)
		first := translate(logReply(records(0, 40)...), tcOptions{})
		if got := translate(logReply(records(0, 80)...), tcOptions{previous: first}); got[log.ARLogFieldEntryCount] != 80 {
			t.Errorf("got %v", got[log.ARLogFieldEntryCount])
		}
	})
}

func TestReplyWithoutText(t *testing.T) {
	for _, data := range []any{"", `{"nvram_dump-syslog.log":}`, nil} {
		t.Run(fmt.Sprintf("history_and_anchor_survive_%v", data), func(t *testing.T) {
			first := translate(logReply(records(0, 40)...), tcOptions{})
			result := translate(data, tcOptions{previous: first})
			if result[log.ARLogFieldEntryCount] != 40 || anchorOf(result).Value != anchorOf(first).Value {
				t.Errorf("got %v", result[log.ARLogFieldEntryCount])
			}
		})

		t.Run(fmt.Sprintf("next_read_does_not_duplicate_%v", data), func(t *testing.T) {
			first := translate(logReply(records(0, 40)...), tcOptions{})
			empty := translate(data, tcOptions{previous: first})
			third := translate(logReply(records(0, 45)...), tcOptions{previous: empty})
			if third[log.ARLogFieldEntryCount] != 45 {
				t.Errorf("got %v", third[log.ARLogFieldEntryCount])
			}
		})
	}

	t.Run("without_previous", func(t *testing.T) {
		result := translate("", tcOptions{})
		if result[log.ARLogFieldEntryCount] != 0 || anchorOf(result).Value != "" {
			t.Errorf("got %v", result)
		}
	})
}

func secretIdentity() *device.ARDeviceIdentity {
	identity := device.NewARDeviceIdentity()
	username := identifiers.NewUsername("fakeadmin")
	identity.UpdateUsername(&username)
	return identity
}

func secretLog() string {
	return logReply(
		"Jul 21 15:08:01 fakeadmin: Server certificate updated.",
		"Jul 21 15:08:02 pppd[21306]: pppd 2.4.7 started by fakeadmin",
	)
}

func levelConfig(level security.ARSecurityLevel) *config.ARInstanceConfig {
	return config.NewARInstanceConfig(map[config.ARConfigKey]any{config.ARConfigKeySecurityLevelData: level})
}

func TestSecretRemoval(t *testing.T) {
	t.Run("removed_by_default", func(t *testing.T) {
		entries := entriesOf(translate(secretLog(), tcOptions{identity: secretIdentity()}))
		if len(entries) != 2 {
			t.Fatalf("got %d entries", len(entries))
		}
		for _, entry := range entries {
			name := ""
			if n := entry.ProgramName(); n != nil {
				name = *n
			}
			// It is a program tag on one of them, not only message text
			if strings.Contains(entry.ContentValue(), "fakeadmin") || strings.Contains(name, "fakeadmin") {
				t.Errorf("entry %+v", entry)
			}
		}
	})

	t.Run("kept_when_unsafe", func(t *testing.T) {
		entries := entriesOf(translate(secretLog(), tcOptions{
			identity: secretIdentity(), config: levelConfig(security.ARSecurityLevelUnsafe),
		}))
		if name := entries[0].ProgramName(); name == nil || *name != "fakeadmin" ||
			!strings.Contains(entries[1].ContentValue(), "fakeadmin") {
			t.Errorf("entries %+v %+v", entries[0], entries[1])
		}
	})

	t.Run("without_an_identity", func(t *testing.T) {
		if got := entriesOf(translate(secretLog(), tcOptions{})); len(got) != 2 {
			t.Errorf("got %d entries", len(got))
		}
	})

	t.Run("without_a_username", func(t *testing.T) {
		if got := entriesOf(translate(secretLog(), tcOptions{identity: device.NewARDeviceIdentity()})); len(got) != 2 {
			t.Errorf("got %d entries", len(got))
		}
	})

	t.Run("anchor_matches_the_cleaned_text", func(t *testing.T) {
		identity := secretIdentity()
		first := translate(secretLog(), tcOptions{identity: identity})
		grown := logReply(
			"Jul 21 15:08:01 fakeadmin: Server certificate updated.",
			"Jul 21 15:08:02 pppd[21306]: pppd 2.4.7 started by fakeadmin",
			"Jul 21 15:08:03 ntp: start NTP update",
		)
		second := translate(grown, tcOptions{identity: identity, previous: first})
		// Continued, not read whole again: three entries, not five
		if second[log.ARLogFieldEntryCount] != 3 {
			t.Errorf("got %v", second[log.ARLogFieldEntryCount])
		}
	})

	t.Run("device_mac_removed", func(t *testing.T) {
		identity := device.ARDeviceIdentityBuild(device.IdentityData{
			source.IDOf(nvram.ARNvramTypeMac): "C8:7F:54:00:00:01",
		})
		// The device spells it upper case, the identity holds it lower
		r := logReply("Jul 21 15:08:01 init: fwver: 3.0.0.6 (/ha:C8:7F:54:00:00:01 )")
		content := entriesOf(translate(r, tcOptions{identity: identity, config: levelConfig(security.ARSecurityLevelStrict)}))[0].ContentValue()
		if strings.Contains(content, "C8:7F:54:00:00:01") || strings.Contains(content, "c8:7f:54:00:00:01") {
			t.Errorf("content %q", content)
		}
	})

	t.Run("serial_removed", func(t *testing.T) {
		identity := device.ARDeviceIdentityBuild(device.IdentityData{
			source.IDOf(nvram.ARNvramTypeSerial): "R2FAKE000000ABC",
		})
		r := logReply("Jul 21 15:08:01 init: fwver: 3.0.0.6 (sn:R2FAKE000000ABC )")
		content := entriesOf(translate(r, tcOptions{identity: identity, config: levelConfig(security.ARSecurityLevelStrict)}))[0].ContentValue()
		if strings.Contains(content, "R2FAKE000000ABC") {
			t.Errorf("content %q", content)
		}
	})
}

func TestYearAnchor(t *testing.T) {
	t.Run("device_year_is_used", func(t *testing.T) {
		identity := identityAt(time.Date(2019, 7, 31, 10, 24, 4, 0, time.UTC))
		entry := entriesOf(translate(logReply("Jul 21 15:08:01 kernel: boot"), tcOptions{identity: identity}))[0]
		if entry.Timestamp.Year() != 2019 {
			t.Errorf("got %v", entry.Timestamp)
		}
	})

	t.Run("device_offset_is_used", func(t *testing.T) {
		cest := time.FixedZone("", 2*3600)
		identity := identityAt(time.Date(2026, 7, 31, 10, 24, 4, 0, cest))
		entry := entriesOf(translate(logReply("Jul 21 15:08:01 kernel: boot"), tcOptions{identity: identity}))[0]
		want := time.Date(2026, 7, 21, 15, 8, 1, 0, cest)
		if _, offset := entry.Timestamp.Zone(); !entry.Timestamp.Equal(want) || offset != 2*3600 {
			t.Errorf("got %v", entry.Timestamp)
		}
	})

	t.Run("host_offset_without_a_device_clock", func(t *testing.T) {
		entry := entriesOf(translate(logReply("Jul 21 15:08:01 kernel: boot"), tcOptions{}))[0]
		_, offset := entry.Timestamp.Zone()
		_, host := time.Now().Zone()
		if entry.Timestamp.Location() == consts.NaiveLocation || offset != host {
			t.Errorf("got %v", entry.Timestamp)
		}
	})

	t.Run("host_year_without_a_device_clock", func(t *testing.T) {
		entry := entriesOf(translate(logReply("Jul 21 15:08:01 kernel: boot"), tcOptions{}))[0]
		if entry.Timestamp.Year() != time.Now().UTC().Year() {
			t.Errorf("got %v", entry.Timestamp)
		}
	})
}
