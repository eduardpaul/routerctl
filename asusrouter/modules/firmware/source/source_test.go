package source

import (
	"context"
	"reflect"
	"testing"

	"github.com/eduardpaul/routerctl/asusrouter/modules/device"
	"github.com/eduardpaul/routerctl/asusrouter/modules/endpoint"
	"github.com/eduardpaul/routerctl/asusrouter/modules/firmware"
	"github.com/eduardpaul/routerctl/asusrouter/modules/nvram"
	"github.com/eduardpaul/routerctl/asusrouter/modules/source"
	"github.com/eduardpaul/routerctl/asusrouter/registry"
)

// Firmware version strings reused across cases
const (
	older = "3004_388_2_0"
	newer = "3004_388_4_0"
)

// A fully populated firmware detection payload
var fullUpdate = map[string]any{
	"webs_state_update":    "1",
	"webs_state_upgrade":   "",
	"webs_state_error":     "0",
	"webs_state_info":      newer,
	"webs_state_info_beta": "",
	"webs_state_REQinfo":   "",
	"webs_state_flag":      "1",
	"webs_state_level":     "0",
	"sig_ver":              "2.380",
	"sig_state_update":     "0",
	"sig_state_upgrade":    "1",
	"sig_state_error":      "0",
	"sig_state_flag":       "1",
	"cfg_check":            "",
	"cfg_upgrade":          "",
}

// identityWith builds a device identity whose firmware equals
// `ARFirmware.from_string(fw)` (Python uses a SimpleNamespace fake). The
// versions used here are `3004_388_<build>_<rev>`, i.e. nvram 3.0.0.4 /
// 388 / <build>_<rev>.
func identityWith(build string) *device.ARDeviceIdentity {
	if build == "" {
		return device.NewARDeviceIdentity()
	}
	return device.ARDeviceIdentityBuild(device.IdentityData{
		source.IDOf(nvram.ARNvramTypeFwMajor): "3.0.0.4",
		source.IDOf(nvram.ARNvramTypeFwMinor): "388",
		source.IDOf(nvram.ARNvramTypeFwBuild): build,
	})
}

var (
	identityOlder = func() *device.ARDeviceIdentity { return identityWith("2_0") }
	identityNewer = func() *device.ARDeviceIdentity { return identityWith("4_0") }
)

func TestIdentityHelperMatchesFromString(t *testing.T) {
	if !identityOlder().Firmware().Equal(firmware.ARFirmwareFromString(older)) {
		t.Fatal("older identity firmware mismatch")
	}
	if !identityNewer().Firmware().Equal(firmware.ARFirmwareFromString(newer)) {
		t.Fatal("newer identity firmware mismatch")
	}
}

func TestDataclassDefaultsWeb(t *testing.T) {
	web := NewARFirmwareWeb()
	if web.Fetch != firmware.ARFirmwareWebFetchUnknown ||
		web.Upgrade != firmware.ARFirmwareWebUpgradeUnknown ||
		web.Error != firmware.ARFirmwareWebErrorUnknown ||
		web.Notify != firmware.ARFirmwareWebNotifyUnknown ||
		web.Available != nil || web.State || web.ReleaseNote != nil {
		t.Fatalf("got %+v", web)
	}
}

func TestDataclassDefaultsState(t *testing.T) {
	state := NewARFirmwareState()
	if state.Current != nil {
		t.Fatal("current not nil")
	}
	if !reflect.DeepEqual(state.Web, NewARFirmwareWeb()) ||
		state.Signature != (ARFirmwareSignature{}) || state.Sync != (ARFirmwareSync{}) {
		t.Fatalf("got %+v", state)
	}
}

func TestARFirmwareSourceEqualityAndHash(t *testing.T) {
	if source.IDOf(&ARFirmwareSource{}) != source.IDOf(&ARFirmwareSource{}) {
		t.Fatal("not equal")
	}
	if source.IDOf(&ARFirmwareSource{}) != source.IDOf(ARFirmwareSourceUniversal) {
		t.Fatal("not equal to universal")
	}
}

func TestARFirmwareSourceEqualityOtherType(t *testing.T) {
	// Python compares to a str; Go compares source IDs of other sources
	if source.IDOf(&ARFirmwareSource{}) == source.IDOf(nvram.ARNvramTypeMac) {
		t.Fatal("equal to another source")
	}
}

func TestARFirmwareSourceRepr(t *testing.T) {
	if got := source.Repr(&ARFirmwareSource{}); got != "<ARFirmwareSource>" {
		t.Fatalf("got %q", got)
	}
}

func TestAvailableValid(t *testing.T) {
	fw := available(newer)
	if fw == nil || *fw.Major() != [4]int{3, 0, 0, 4} {
		t.Fatalf("got %v", fw)
	}
}

func TestAvailableNone(t *testing.T) {
	for _, raw := range []any{nil, "", "garbage"} {
		if got := available(raw); got != nil {
			t.Errorf("%#v: got %v", raw, got)
		}
	}
}

func TestIsStableUpdate(t *testing.T) {
	fs := firmware.ARFirmwareFromString
	cases := []struct {
		current, available *firmware.ARFirmware
		expected           bool
	}{
		{nil, nil, false},
		{fs(newer), nil, false},
		{nil, fs(newer), true},
		{firmware.NewARFirmware(nil, nil, nil, nil, false), fs(newer), true},
		{fs(older), fs(newer), true},
		{fs(newer), fs(older), false},
	}
	for i, c := range cases {
		if got := isStableUpdate(c.current, c.available); got != c.expected {
			t.Errorf("case %d: got %v", i, got)
		}
	}
}

// rawSequence mirrors AsyncMock(side_effect=[...]) / return_value: it
// answers the given bodies in order (the last one repeats) and counts
// the calls.
func rawSequence(calls *int, bodies ...string) registry.FetchRawFunc {
	return func(ctx context.Context, ep endpoint.AREndpoint, request string) (*string, error) {
		i := *calls
		*calls++
		if i >= len(bodies) {
			i = len(bodies) - 1
		}
		body := bodies[i]
		return &body, nil
	}
}

func TestFetchNote(t *testing.T) {
	cases := []struct {
		name     string
		bodies   []string
		expected *string
		calls    int
	}{
		{"first_endpoint", []string{"Release Note\n- Fix A\n"}, strp("- Fix A"), 1},
		{"fallback_to_second", []string{"", "Release Note\n- Fix B\n"}, strp("- Fix B"), 2},
		{"none_when_all_empty", []string{""}, nil, 2},
		{"none_when_only_headers", []string{"Firmware version 1\nRelease Note\n"}, nil, 2},
	}
	for _, c := range cases {
		calls := 0
		got, err := fetchNote(context.Background(), rawSequence(&calls, c.bodies...))
		if err != nil {
			t.Fatal(err)
		}
		if !eqStrP(got, c.expected) || calls != c.calls {
			t.Errorf("%s: got %v after %d calls", c.name, got, calls)
		}
	}
}

func strp(s string) *string { return &s }

func eqStrP(a, b *string) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

func readReturning(value any) registry.ReadFunc {
	return func(ctx context.Context, ep endpoint.AREndpoint, request string) (any, error) {
		return value, nil
	}
}

func TestFetchStateStableUpdateFetchesNote(t *testing.T) {
	calls := 0
	c := &registry.Context{
		Read:     readReturning(map[string]any{"webs_state_info": newer}),
		FetchRaw: rawSequence(&calls, "Release Note\n- Fix\n"),
		Identity: identityOlder(),
	}
	raw, err := fetchState(context.Background(), c, ARFirmwareSourceUniversal)
	if err != nil {
		t.Fatal(err)
	}
	result := raw.(map[string]any)
	if !reflect.DeepEqual(result["update"], map[string]any{"webs_state_info": newer}) {
		t.Errorf("update %v", result["update"])
	}
	if note, _ := result["note"].(*string); !eqStrP(note, strp("- Fix")) {
		t.Errorf("note %v", result["note"])
	}
	if fw, _ := result["available"].(*firmware.ARFirmware); fw == nil {
		t.Errorf("available %v", result["available"])
	}
	if calls == 0 {
		t.Error("raw callback not awaited")
	}
}

func TestFetchStateNoStableUpdateSkipsNote(t *testing.T) {
	calls := 0
	c := &registry.Context{
		Read:     readReturning(map[string]any{"webs_state_info": older}),
		FetchRaw: rawSequence(&calls, ""),
		Identity: identityNewer(),
	}
	raw, err := fetchState(context.Background(), c, ARFirmwareSourceUniversal)
	if err != nil {
		t.Fatal(err)
	}
	if note, _ := raw.(map[string]any)["note"].(*string); note != nil {
		t.Errorf("note %v", *note)
	}
	if calls != 0 {
		t.Error("raw callback awaited")
	}
}

func TestFetchStateNoRawCallback(t *testing.T) {
	c := &registry.Context{
		Read:     readReturning(map[string]any{"webs_state_info": newer}),
		Identity: identityOlder(),
	}
	raw, err := fetchState(context.Background(), c, ARFirmwareSourceUniversal)
	if err != nil {
		t.Fatal(err)
	}
	if note, _ := raw.(map[string]any)["note"].(*string); note != nil {
		t.Errorf("note %v", *note)
	}
}

func TestFetchStateNonDictUpdate(t *testing.T) {
	calls := 0
	c := &registry.Context{
		Read:     readReturning(nil),
		FetchRaw: rawSequence(&calls, ""),
		Identity: identityWith(""),
	}
	raw, err := fetchState(context.Background(), c, ARFirmwareSourceUniversal)
	if err != nil {
		t.Fatal(err)
	}
	result := raw.(map[string]any)
	note, _ := result["note"].(*string)
	avail, _ := result["available"].(*firmware.ARFirmware)
	if !reflect.DeepEqual(result["update"], map[string]any{}) || note != nil || avail != nil || len(result) != 3 {
		t.Fatalf("got %v", result)
	}
}

func tcWith(identity *device.ARDeviceIdentity) *registry.TranslateContext {
	return &registry.TranslateContext{Identity: identity}
}

func TestTranslateStateNonDict(t *testing.T) {
	got := translateState(nil, tcWith(identityWith("")))
	if !reflect.DeepEqual(got, NewARFirmwareState()) {
		t.Fatalf("got %+v", got)
	}
}

func TestTranslateStateFull(t *testing.T) {
	data := map[string]any{"update": fullUpdate, "note": "- Fix"}
	state := translateState(data, tcWith(identityOlder())).(*ARFirmwareState)

	if state.Current == nil {
		t.Error("current nil")
	}
	w := state.Web
	if w.Fetch != firmware.ARFirmwareWebFetchInactive ||
		w.Upgrade != firmware.ARFirmwareWebUpgradeUnknown ||
		w.Error != firmware.ARFirmwareWebErrorNone ||
		w.Notify != firmware.ARFirmwareWebNotifyAvailable {
		t.Errorf("web enums %+v", w)
	}
	if w.Level == nil || *w.Level != 0 || w.Available == nil || w.Required != nil ||
		!w.State || w.StateBeta || !eqStrP(w.ReleaseNote, strp("- Fix")) {
		t.Errorf("web %+v", w)
	}

	s := state.Signature
	if !eqStrP(s.Version, strp("2.380")) || *s.Update != 0 || *s.Upgrade != 1 || *s.Error != 0 || *s.Flag != 1 {
		t.Errorf("signature %+v", s)
	}

	if state.Sync.Check != nil || state.Sync.Upgrade != nil {
		t.Errorf("sync %+v", state.Sync)
	}
}

func TestTranslateStateAvailableGatedOutWhenNotNewer(t *testing.T) {
	data := map[string]any{"update": map[string]any{"webs_state_info": older}}
	state := translateState(data, tcWith(identityNewer())).(*ARFirmwareState)
	if state.Web.Available != nil || state.Web.State {
		t.Fatalf("web %+v", state.Web)
	}
}

func TestTranslateStateBetaPresence(t *testing.T) {
	data := map[string]any{"update": map[string]any{"webs_state_info_beta": "3004_388_5_0"}}
	state := translateState(data, tcWith(identityWith(""))).(*ARFirmwareState)
	if state.Web.AvailableBeta == nil || !state.Web.StateBeta {
		t.Fatalf("web %+v", state.Web)
	}
}

func TestTranslateStateEmptyUpdate(t *testing.T) {
	state := translateState(map[string]any{}, tcWith(identityWith(""))).(*ARFirmwareState)
	if state.Web.Fetch != firmware.ARFirmwareWebFetchUnknown || state.Web.Available != nil || state.Signature.Version != nil {
		t.Fatalf("state %+v", state)
	}
}

func TestModuleRegistersSource(t *testing.T) {
	e := registry.SourceByName("ARFirmwareSource")
	if e == nil {
		t.Fatal("not registered")
	}
	if reflect.ValueOf(e.Fetch).Pointer() != reflect.ValueOf(fetchState).Pointer() {
		t.Error("fetch is not fetchState")
	}
	if reflect.ValueOf(e.Translate).Pointer() != reflect.ValueOf(translateState).Pointer() {
		t.Error("translate is not translateState")
	}
	if e.Multi() {
		t.Error("fetch must not be multi")
	}
}
