package clock

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/eduardpaul/routerctl/asusrouter/consts"
	"github.com/eduardpaul/routerctl/asusrouter/modules/device"
	"github.com/eduardpaul/routerctl/asusrouter/modules/endpoint"
	"github.com/eduardpaul/routerctl/asusrouter/modules/nvram"
	"github.com/eduardpaul/routerctl/asusrouter/modules/source"
	"github.com/eduardpaul/routerctl/asusrouter/registry"
)

var (
	when = time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	boot = when.Add(-100 * time.Second)

	// A real reply: the device states its clock, its offset and its uptime
	cest = time.FixedZone("UTC+02:00", 2*3600)
)

const uptimeReply = "Fri, 31 Jul 2026 10:24:04 +0200(2131043 secs since boot)"

func naive(year int, month time.Month, day, hour, minute, second int) time.Time {
	return time.Date(year, month, day, hour, minute, second, 0, consts.NaiveLocation)
}

func tp(t time.Time) *time.Time { return &t }

// sameInstant mirrors Python aware-datetime equality.
func sameInstant(got any, want time.Time) bool {
	switch v := got.(type) {
	case ARBoottime:
		return v.Time.Equal(want)
	case *ARBoottime:
		return v != nil && v.Time.Equal(want)
	case time.Time:
		return v.Equal(want)
	}
	return false
}

func TestReadUptimeReadsAllThreeValues(t *testing.T) {
	got := ReadUptime(uptimeReply)
	if len(got) != 3 ||
		!sameInstant(got[ARClockFieldBoottime], time.Date(2026, 7, 6, 18, 26, 41, 0, cest)) ||
		!sameInstant(got[ARClockFieldDeviceTime], time.Date(2026, 7, 31, 10, 24, 4, 0, cest)) ||
		got[ARClockFieldUptime] != 2131043 {
		t.Fatalf("got %v", got)
	}
}

func TestReadUptimeDeviceTimeKeepsTheOffset(t *testing.T) {
	deviceTime := ReadUptime(uptimeReply)[ARClockFieldDeviceTime].(time.Time)
	if !isAware(deviceTime) {
		t.Fatal("naive device time")
	}
	if _, offset := deviceTime.Zone(); offset != 2*3600 {
		t.Fatalf("offset %d", offset)
	}
}

func TestReadUptimeBootTimeIsTagged(t *testing.T) {
	if _, ok := ReadUptime(uptimeReply)[ARClockFieldBoottime].(ARBoottime); !ok {
		t.Fatal("boot time is not an ARBoottime")
	}
}

func TestReadUptimeUnparseable(t *testing.T) {
	for _, uptime := range []string{"no-paren", "when(no digits)"} {
		if got := ReadUptime(uptime); len(got) != 0 {
			t.Errorf("%q: got %v", uptime, got)
		}
	}
}

func TestReadUptimeClockWithoutAnOffset(t *testing.T) {
	for _, uptime := range []string{
		// Parses as nothing at all
		"Fri, 31 Jul 2026 10:24:04(100 secs)",
		// Parses, but naive: ISO with no offset
		"2026-07-31T10:24:04(100 secs)",
	} {
		got := ReadUptime(uptime)
		if !reflect.DeepEqual(got, map[ARClockField]any{ARClockFieldUptime: 100}) {
			t.Errorf("%q: got %v", uptime, got)
		}
	}
}

func TestReadUptimeBeyondAnyDevice(t *testing.T) {
	// Past a century, and past what a timedelta can even hold
	for _, seconds := range []string{"99999999999999", strings.Repeat("9", 400)} {
		content := fmt.Sprintf("Fri, 31 Jul 2026 10:24:04 +0200(%s secs)", seconds)
		if got := ReadUptime(content); len(got) != 0 {
			t.Errorf("%s: got %v", seconds[:10], got)
		}
	}
}

func TestReadUptimeClockTooEarlyForItsOwnUptime(t *testing.T) {
	for _, w := range []string{"Mon, 01 Jan 0001 00:00:00 +0000", "Sat, 01 Jan 0050 00:00:00 +0000"} {
		got := ReadUptime(w + "(3153600000 secs)")
		if !reflect.DeepEqual(got, map[ARClockField]any{ARClockFieldUptime: 3153600000}) {
			t.Errorf("%q: got %v", w, got)
		}
	}
}

func TestReadUptimeALongButPossibleUptime(t *testing.T) {
	fiveYears := 5 * 365 * 24 * 3600
	content := fmt.Sprintf("Fri, 31 Jul 2026 10:24:04 +0200(%d secs)", fiveYears)
	if got := ReadUptime(content)[ARClockFieldUptime]; got != fiveYears {
		t.Fatalf("got %v", got)
	}
}

func TestSourceIsDataSourceAllEqual(t *testing.T) {
	var _ source.DataSource = &ARClockSource{}
	if source.IDOf(&ARClockSource{}) != source.IDOf(&ARClockSource{}) {
		t.Fatal("not equal")
	}
}

func TestSourceNotEqualOtherType(t *testing.T) {
	// Python compares to a str; Go compares source IDs of other sources
	if source.IDOf(&ARClockSource{}) == source.IDOf(nvram.ARNvramTypeMac) {
		t.Fatal("equal to another source")
	}
}

func TestSourceRepr(t *testing.T) {
	if got := source.Repr(&ARClockSource{}); got != "<ARClockSource>" {
		t.Fatalf("got %q", got)
	}
}

func TestStabilizeFirstSample(t *testing.T) {
	if got := Stabilize(&boot, nil); got == nil || !got.Equal(boot) {
		t.Fatalf("got %v", got)
	}
}

func TestStabilizeKeepsWithinJitter(t *testing.T) {
	if got := Stabilize(tp(boot.Add(time.Second)), &boot); got == nil || !got.Equal(boot) {
		t.Fatalf("got %v", got)
	}
}

func TestStabilizeReboot(t *testing.T) {
	rebooted := boot.Add(5 * time.Second)
	if got := Stabilize(&rebooted, &boot); got == nil || !got.Equal(rebooted) {
		t.Fatalf("got %v", got)
	}
}

func TestStabilizeNoneCandidateKeepsPrevious(t *testing.T) {
	if got := Stabilize(nil, &boot); got == nil || !got.Equal(boot) {
		t.Fatalf("got %v", got)
	}
}

func TestStabilizeNaiveNeverPassesThrough(t *testing.T) {
	nv := naive(2026, 1, 1, 11, 58, 20)
	cases := []struct {
		name            string
		candidate, prev *time.Time
		expected        *time.Time
	}{
		{"naive_prev", &boot, &nv, &boot},
		{"naive_candidate", &nv, &boot, &boot},
		{"naive_prev_only", nil, &nv, nil},
		{"naive_only", &nv, nil, nil},
	}
	for _, c := range cases {
		got := Stabilize(c.candidate, c.prev)
		if c.expected == nil {
			if got != nil {
				t.Errorf("%s: got %v", c.name, got)
			}
			continue
		}
		if got == nil || !got.Equal(*c.expected) || !isAware(got.Time) {
			t.Errorf("%s: got %v", c.name, got)
		}
	}
}

func TestStabilizeAlwaysReturnsBoottimeType(t *testing.T) {
	// prev is a plain datetime (e.g. a seed); jitter keeps it. In Go the
	// return type is *ARBoottime by construction.
	result := Stabilize(tp(boot.Add(time.Second)), &boot)
	if result == nil || !result.Equal(boot) {
		t.Fatalf("got %v", result)
	}
}

func TestARBoottimeFromDatetime(t *testing.T) {
	tagged, err := ARBoottimeFromDatetime(boot)
	if err != nil || !tagged.Equal(boot) {
		t.Fatalf("got %v, %v", tagged, err)
	}
	// A datetime subclass in Python; Go embeds time.Time
	var _ time.Time = tagged.Time
}

func TestARBoottimeNaiveIsRejected(t *testing.T) {
	// Python `ARBoottime(2026, 1, 1)`: Go has no datetime constructor on
	// the tag, so it goes through ARBoottimeFromDatetime
	if _, err := ARBoottimeFromDatetime(naive(2026, 1, 1, 0, 0, 0)); err == nil || !strings.Contains(err.Error(), "must state its offset") {
		t.Fatalf("got %v", err)
	}
}

func TestARBoottimeNaiveFromDatetimeIsRejected(t *testing.T) {
	if _, err := ARBoottimeFromDatetime(naive(2026, 1, 1, 0, 0, 0)); err == nil || !strings.Contains(err.Error(), "must state its offset") {
		t.Fatalf("got %v", err)
	}
}

func TestARBoottimeSurvivesARoundTrip(t *testing.T) {
	// Python pickles and `replace`s; Go copies the value and moves the
	// year, which must keep both the tag and the offset
	tagged, _ := ARBoottimeFromDatetime(boot)
	restored := tagged
	if !restored.Equal(tagged.Time) {
		t.Fatal("copy differs")
	}
	replaced := ARBoottime{Time: tagged.AddDate(1, 0, 0)}
	if !isAware(replaced.Time) {
		t.Fatal("offset lost")
	}
	if got := replaced.CanonicalValue(); !reflect.DeepEqual(got, replaced.Time) {
		t.Fatalf("canonical %v", got)
	}
}

func strp(s string) *string { return &s }

func TestExtractUptimeFromInvalidJSON(t *testing.T) {
	content := "{\n\"uptime\":Sat, 01 Aug 2015 03:09:22 +0200(4162 secs since boot)\n}"
	got := extractUptime(content)
	if got == nil || *got != "Sat, 01 Aug 2015 03:09:22 +0200(4162 secs since boot)" {
		t.Fatalf("got %v", got)
	}
}

func TestExtractUptimeQuotedValue(t *testing.T) {
	if got := extractUptime(`{"uptime":"when(1 secs)"}`); got == nil || *got != "when(1 secs)" {
		t.Fatalf("got %v", got)
	}
	// The raw callback hands a *string
	if got := extractUptime(strp(`{"uptime":"when(1 secs)"}`)); got == nil || *got != "when(1 secs)" {
		t.Fatalf("got %v", got)
	}
}

func TestExtractUptimeNoUptime(t *testing.T) {
	for _, content := range []any{nil, 42, "{}", `{"other":1}`, (*string)(nil)} {
		if got := extractUptime(content); got != nil {
			t.Errorf("%#v: got %q", content, *got)
		}
	}
}

type readCall struct {
	ep      endpoint.AREndpoint
	request string
}

func reader(value any, calls *[]readCall) registry.ReadFunc {
	return func(ctx context.Context, ep endpoint.AREndpoint, request string) (any, error) {
		*calls = append(*calls, readCall{ep, request})
		return value, nil
	}
}

func TestFetchStateRawFallback(t *testing.T) {
	var reads []readCall
	rawCalls := 0
	c := &registry.Context{
		Read: reader(map[string]any{}, &reads),
		FetchRaw: func(ctx context.Context, ep endpoint.AREndpoint, request string) (*string, error) {
			rawCalls++
			body := "{\n\"uptime\":" + uptimeReply + "\n}"
			return &body, nil
		},
	}

	raw, err := fetchState(context.Background(), c, &ARClockSource{})
	if err != nil {
		t.Fatal(err)
	}
	result := raw.(map[ARClockField]any)
	if result[ARClockFieldUptime] != 2131043 {
		t.Errorf("uptime %v", result[ARClockFieldUptime])
	}
	if rawCalls != 1 {
		t.Errorf("raw called %d times", rawCalls)
	}
	// The raw read answered, so the same hook is not asked twice
	if len(reads) != 0 {
		t.Errorf("read called %d times", len(reads))
	}
}

func TestFetchStateFetchesAndStabilizes(t *testing.T) {
	var reads []readCall
	identity := device.NewARDeviceIdentity()
	// A 1s jitter is absorbed: the identity's value is kept
	anchor := time.Date(2026, 7, 6, 18, 26, 40, 0, cest)
	identity.UpdateBoottime(&anchor)
	c := &registry.Context{Read: reader(map[string]any{"uptime": uptimeReply}, &reads), Identity: identity}

	raw, err := fetchState(context.Background(), c, &ARClockSource{})
	if err != nil {
		t.Fatal(err)
	}
	if got := raw.(map[ARClockField]any)[ARClockFieldBoottime]; !sameInstant(got, anchor) {
		t.Errorf("boottime %v", got)
	}
	if len(reads) != 1 || reads[0].ep != endpoint.AREndpointFetchData || reads[0].request != "hook=uptime()" {
		t.Errorf("reads %v", reads)
	}
}

func TestFetchStateWithoutIdentity(t *testing.T) {
	var reads []readCall
	c := &registry.Context{Read: reader(map[string]any{"uptime": uptimeReply}, &reads)}

	raw, err := fetchState(context.Background(), c, &ARClockSource{})
	if err != nil {
		t.Fatal(err)
	}
	if got := raw.(map[ARClockField]any)[ARClockFieldBoottime]; !sameInstant(got, time.Date(2026, 7, 6, 18, 26, 41, 0, cest)) {
		t.Errorf("boottime %v", got)
	}
}

func TestFetchStateNaiveAnchorIsNotCompared(t *testing.T) {
	var reads []readCall
	identity := device.NewARDeviceIdentity()
	// A naive seed cannot be subtracted from an aware candidate
	seed := naive(2026, 7, 6, 18, 26, 40)
	identity.UpdateBoottime(&seed)
	c := &registry.Context{Read: reader(map[string]any{"uptime": uptimeReply}, &reads), Identity: identity}

	raw, err := fetchState(context.Background(), c, &ARClockSource{})
	if err != nil {
		t.Fatal(err)
	}
	if got := raw.(map[ARClockField]any)[ARClockFieldBoottime]; !sameInstant(got, time.Date(2026, 7, 6, 18, 26, 41, 0, cest)) {
		t.Errorf("boottime %v", got)
	}
}

func TestFetchStateMissingUptime(t *testing.T) {
	for _, raw := range []any{nil, map[string]any{}, map[string]any{"other": 1}} {
		var reads []readCall
		c := &registry.Context{Read: reader(raw, &reads)}
		got, err := fetchState(context.Background(), c, &ARClockSource{})
		if err != nil {
			t.Fatal(err)
		}
		if m := got.(map[ARClockField]any); len(m) != 0 {
			t.Errorf("%#v: got %v", raw, m)
		}
	}
}

func TestRegistersCallable(t *testing.T) {
	// Python reloads the module under a mocked register_source; Go checks
	// the entry init() registered
	e := registry.SourceByName("ARClockSource")
	if e == nil {
		t.Fatal("not registered")
	}
	if reflect.ValueOf(e.Fetch).Pointer() != reflect.ValueOf(fetchState).Pointer() {
		t.Error("fetch is not fetchState")
	}
	if e.Translate != nil || e.Multi() {
		t.Error("only fetch_state is registered")
	}
	if e.Default == nil {
		t.Error("no default instance")
	}
}
