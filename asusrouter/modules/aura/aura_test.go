package aura

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/eduardpaul/routerctl/asusrouter/modules/device"
	"github.com/eduardpaul/routerctl/asusrouter/modules/endpoint"
	"github.com/eduardpaul/routerctl/asusrouter/modules/nvram"
	"github.com/eduardpaul/routerctl/asusrouter/modules/service"
	"github.com/eduardpaul/routerctl/asusrouter/modules/source"
	"github.com/eduardpaul/routerctl/asusrouter/modules/support"
	"github.com/eduardpaul/routerctl/asusrouter/parity/replay"
	"github.com/eduardpaul/routerctl/asusrouter/registry"
	"github.com/eduardpaul/routerctl/asusrouter/tools/color"
)

func intp(v int) *int    { return &v }
func boolp(v bool) *bool { return &v }
func schemep(s ARAuraScheme) *ARAuraScheme {
	return &s
}

// rgb mirrors `_rgb`: the indexed color source for a scheme code.
func rgb(code int) *nvram.ARNvramIndexSource {
	return nvram.NewARNvramIndexSource(nvram.ARNvramIndexTypeAuraRgb, code)
}

// identityWith builds an identity with the given support map.
func identityWith(sup map[support.ARSupportType]any) *device.ARDeviceIdentity {
	return device.ARDeviceIdentityBuild(device.IdentityData{
		source.IDOf(support.ARSupportSourceUniversal): sup,
	})
}

// fakeIdentity mirrors `_identity`: a fake identity exposing an Aura
// support map.
func fakeIdentity(aura, night bool) *device.ARDeviceIdentity {
	capabilities := map[ARAuraCapability]any{}
	if night {
		capabilities[ARAuraCapabilityNightMode] = true
	}
	return identityWith(map[support.ARSupportType]any{
		support.ARSupportTypeAura:             aura,
		support.ARSupportTypeAuraCapabilities: capabilities,
	})
}

// nv builds nvram data keyed like the nvram fetch result (item ID).
func nv(pairs ...any) map[source.ID]any {
	out := map[source.ID]any{}
	for i := 0; i < len(pairs); i += 2 {
		out[source.IDOf(pairs[i].(nvram.ARNvramItem))] = pairs[i+1]
	}
	return out
}

// col mirrors `Color(r, g, b[, brightness])`.
func col(r, g, b int, brightness ...int) color.Color {
	br := color.ColorScale
	if len(brightness) > 0 {
		br = brightness[0]
	}
	return color.NewColor(r, g, b, br)
}

// ---- test_support.py ----

func TestAuraSupported(t *testing.T) {
	if !AuraSupported(fakeIdentity(true, false)) {
		t.Fatal("supported")
	}
	if AuraSupported(fakeIdentity(false, false)) {
		t.Fatal("unsupported")
	}
	if AuraSupported(nil) {
		t.Fatal("no identity")
	}
}

func TestNightModeSupported(t *testing.T) {
	if !NightModeSupported(fakeIdentity(true, true)) {
		t.Fatal("supported")
	}
	if NightModeSupported(fakeIdentity(true, false)) {
		t.Fatal("unsupported")
	}
	if NightModeSupported(nil) {
		t.Fatal("no identity")
	}
}

// ---- test_enums.py ----

func TestEnums(t *testing.T) {
	for _, m := range ARAuraSchemeMembers {
		if ARAuraSchemeFromValue(int(m)) != m {
			t.Fatalf("scheme %s", m.Name())
		}
	}
	for _, m := range ARAuraFieldMembers {
		if ARAuraFieldFromValue(string(m)) != m {
			t.Fatalf("field %s", m.Name())
		}
	}
	const bogus = "zzz-not-a-real-enum-value"
	if ARAuraSchemeFromValue(bogus) != ARAuraSchemeUnknown || ARAuraFieldFromValue(bogus) != ARAuraFieldUnknown {
		t.Fatal("unknown fallback")
	}
}

// ---- test_effect.py ----

func TestDefaultColors(t *testing.T) {
	if len(DefaultColors(0)) != 0 || len(DefaultColors(-1)) != 0 {
		t.Fatal("zero")
	}
	colors := DefaultColors(5)
	if len(colors) != 5 || colors[0] != colors[4] {
		t.Fatalf("repeats pattern: %v", colors)
	}
}

func TestFitZones(t *testing.T) {
	colors := []color.Color{col(255, 0, 0), col(0, 255, 0), col(0, 0, 255)}
	if got := FitZones(colors, 2); !reflect.DeepEqual(got, colors[:2]) {
		t.Fatalf("truncate: %v", got)
	}
	fitted := FitZones([]color.Color{col(255, 0, 0)}, 3)
	if len(fitted) != 3 || fitted[0] != col(255, 0, 0) {
		t.Fatalf("pad: %v", fitted)
	}
	one := []color.Color{col(255, 0, 0)}
	if got := FitZones(one, 0); !reflect.DeepEqual(got, one) {
		t.Fatalf("zero zones: %v", got)
	}
}

func TestApplyColor(t *testing.T) {
	colors := []color.Color{col(1, 2, 3, 100)}
	if got := ApplyColor(colors, nil, nil); !reflect.DeepEqual(got, colors) {
		t.Fatal("none")
	}

	two := []color.Color{col(0, 0, 0, 100), col(0, 0, 0, 200)}
	got := ApplyColor(two, col(255, 0, 0), nil)
	if got[0] != col(255, 0, 0, 100) || got[1] != col(255, 0, 0, 200) {
		t.Fatalf("single all zones: %v", got)
	}

	got = ApplyColor(two, col(255, 0, 0), intp(1))
	if got[0] != col(0, 0, 0, 100) || got[1] != col(255, 0, 0, 200) {
		t.Fatalf("single one zone: %v", got)
	}

	got = ApplyColor([]color.Color{col(0, 0, 0, 100)}, col(255, 0, 0), intp(5))
	if got[0] != col(255, 0, 0, 100) {
		t.Fatalf("zone out of range: %v", got)
	}

	three := []color.Color{col(0, 0, 0, 10), col(0, 0, 0, 10), col(0, 0, 0, 10)}
	got = ApplyColor(three, []color.Color{col(255, 0, 0), col(0, 255, 0)}, nil)
	if got[0].Red != 255 || got[1].Green != 255 || got[2].Red != 255 {
		t.Fatalf("list cycles: %v", got)
	}
	for _, c := range got {
		if c.Brightness != 10 {
			t.Fatalf("brightness kept: %v", got)
		}
	}

	if got := ApplyColor(colors, []color.Color{}, nil); !reflect.DeepEqual(got, colors) {
		t.Fatal("empty list")
	}
}

func TestApplyBrightness(t *testing.T) {
	colors := []color.Color{col(1, 2, 3, 100)}
	if got := ApplyBrightness(colors, nil, nil); !reflect.DeepEqual(got, colors) {
		t.Fatal("none")
	}
	two := []color.Color{col(1, 0, 0, 10), col(0, 1, 0, 20)}
	for _, c := range ApplyBrightness(two, intp(200), nil) {
		if c.Brightness != 200 {
			t.Fatal("all zones")
		}
	}
	got := ApplyBrightness(two, intp(200), intp(0))
	if got[0].Brightness != 200 || got[1].Brightness != 20 {
		t.Fatalf("one zone: %v", got)
	}
	got = ApplyBrightness([]color.Color{col(1, 0, 0, 10)}, intp(200), intp(9))
	if got[0].Brightness != 200 {
		t.Fatalf("zone out of range: %v", got)
	}
}

// ---- test_source.py ----

func TestFetchState(t *testing.T) {
	ctx := context.Background()

	t.Run("no_identity", func(t *testing.T) {
		got, err := fetchState(ctx, &registry.Context{}, ARAuraSourceUniversal)
		if err != nil || len(got.(map[source.ID]any)) != 0 {
			t.Fatalf("fetchState = %v, %v", got, err)
		}
	})

	t.Run("unsupported", func(t *testing.T) {
		called := false
		c := &registry.Context{
			Identity: fakeIdentity(false, false),
			FetchDataMulti: func(context.Context, []source.DataSource, registry.Kwargs) (map[source.ID]any, error) {
				called = true
				return nil, nil
			},
		}
		got, err := fetchState(ctx, c, ARAuraSourceUniversal)
		if err != nil || len(got.(map[source.ID]any)) != 0 || called {
			t.Fatalf("fetchState = %v, %v, called=%v", got, err, called)
		}
	})

	t.Run("supported_fetches_via_nvram", func(t *testing.T) {
		values := nv(nvram.ARNvramTypeAura, "1")
		var request []source.DataSource
		c := &registry.Context{
			Identity: fakeIdentity(true, false),
			FetchDataMulti: func(_ context.Context, srcs []source.DataSource, _ registry.Kwargs) (map[source.ID]any, error) {
				request = srcs
				return values, nil
			},
		}
		got, err := fetchState(ctx, c, ARAuraSourceUniversal)
		if err != nil || !reflect.DeepEqual(got, values) {
			t.Fatalf("fetchState = %v, %v", got, err)
		}
		ids := map[source.ID]bool{}
		for _, s := range request {
			ids[source.IDOf(s)] = true
		}
		for _, want := range []nvram.ARNvramItem{nvram.ARNvramTypeAura, nvram.ARNvramTypeAuraScheme, rgb(0), rgb(7), nvram.ARNvramTypeAuraNightMode} {
			if !ids[source.IDOf(want)] {
				t.Fatalf("%v not requested", want)
			}
		}
		// Night colors are firmware-dead, not fetched
		if ids[source.IDOf(nvram.ARNvramTypeAuraNightRgb)] {
			t.Fatal("night colors requested")
		}
	})

	t.Run("no_get_data_callback", func(t *testing.T) {
		got, err := fetchState(ctx, &registry.Context{Identity: fakeIdentity(true, false)}, ARAuraSourceUniversal)
		if err != nil || len(got.(map[source.ID]any)) != 0 {
			t.Fatalf("fetchState = %v, %v", got, err)
		}
	})

	t.Run("non_dict_response", func(t *testing.T) {
		c := &registry.Context{
			Identity: fakeIdentity(true, false),
			FetchDataMulti: func(context.Context, []source.DataSource, registry.Kwargs) (map[source.ID]any, error) {
				return nil, nil
			},
		}
		got, err := fetchState(ctx, c, ARAuraSourceUniversal)
		if err != nil || len(got.(map[source.ID]any)) != 0 {
			t.Fatalf("fetchState = %v, %v", got, err)
		}
	})
}

func translate(data any, identity *device.ARDeviceIdentity) map[ARAuraField]any {
	var tc *registry.TranslateContext
	if identity != nil {
		tc = &registry.TranslateContext{Identity: identity}
	}
	return translateState(data, tc).(map[ARAuraField]any)
}

func TestTranslateState(t *testing.T) {
	id := fakeIdentity(true, false)

	for _, data := range []any{nil, map[source.ID]any{}, "text", []any{}} {
		if got := translate(data, nil); len(got) != 0 {
			t.Fatalf("empty: %v", got)
		}
	}

	t.Run("basic_fields", func(t *testing.T) {
		got := translate(nv(
			nvram.ARNvramTypeAura, "1",
			nvram.ARNvramTypeAuraScheme, "2",
			nvram.ARNvramTypeAuraSchemePrev, "3",
			nvram.ARNvramTypeAuraCount, "4",
		), id)
		if got[ARAuraFieldState] != true || got[ARAuraFieldScheme] != ARAuraSchemeStatic ||
			got[ARAuraFieldSchemePrev] != ARAuraSchemeBreathing || got[ARAuraFieldZones] != 4 {
			t.Fatalf("result = %v", got)
		}
	})

	t.Run("off_forces_state_off", func(t *testing.T) {
		got := translate(nv(nvram.ARNvramTypeAura, "1", nvram.ARNvramTypeAuraScheme, "0"), id)
		if got[ARAuraFieldState] != false {
			t.Fatalf("state = %v", got[ARAuraFieldState])
		}
	})

	t.Run("zones_fallback_to_static", func(t *testing.T) {
		got := translate(nv(nvram.ARNvramTypeAuraScheme, "2", rgb(2), "10,0,64,64,0,32"), id)
		if got[ARAuraFieldZones] != 2 {
			t.Fatalf("zones = %v", got[ARAuraFieldZones])
		}
	})

	t.Run("day_colors_parsed", func(t *testing.T) {
		got := translate(nv(nvram.ARNvramTypeAuraScheme, "2", rgb(2), "10,0,64"), id)
		colors := got[ARAuraFieldColors].(map[ARAuraScheme][]color.Color)
		if len(colors[ARAuraSchemeStatic]) != 1 {
			t.Fatalf("colors = %v", colors)
		}
	})

	night := nv(nvram.ARNvramTypeAuraScheme, "2", nvram.ARNvramTypeAuraNightMode, "1")
	t.Run("night_absent_without_identity", func(t *testing.T) {
		if _, ok := translate(night, nil)[ARAuraFieldNightMode]; ok {
			t.Fatal("night_mode present")
		}
	})
	t.Run("night_absent_without_support", func(t *testing.T) {
		if _, ok := translate(night, fakeIdentity(true, false))[ARAuraFieldNightMode]; ok {
			t.Fatal("night_mode present")
		}
	})
	t.Run("night_present_with_support", func(t *testing.T) {
		got := translate(night, fakeIdentity(true, true))
		if got[ARAuraFieldNightMode] != true {
			t.Fatalf("night_mode = %v", got[ARAuraFieldNightMode])
		}
		// Night colors are firmware-dead and never emitted
		if _, ok := got[ARAuraFieldNightColors]; ok {
			t.Fatal("night_colors present")
		}
	})

	t.Run("sdn_not_emitted", func(t *testing.T) {
		got := translate(nv(nvram.ARNvramTypeAuraScheme, "2", nvram.ARNvramTypeAuraSdn, "1<0<0"), id)
		if _, ok := got[ARAuraFieldSdn]; ok {
			t.Fatal("sdn present")
		}
	})

	t.Run("active_summary", func(t *testing.T) {
		got := translate(nv(nvram.ARNvramTypeAuraScheme, "2", rgb(2), "10,0,64,64,0,32"), id)
		if _, ok := got[ARAuraFieldColor]; !ok {
			t.Fatal("color missing")
		}
		if _, ok := got[ARAuraFieldBrightness]; !ok {
			t.Fatal("brightness missing")
		}
	})

	t.Run("no_active_summary_for_non_color_scheme", func(t *testing.T) {
		got := translate(nv(nvram.ARNvramTypeAuraScheme, "5", rgb(5), "10,0,64"), id)
		if _, ok := got[ARAuraFieldColor]; ok {
			t.Fatal("color present")
		}
	})
}

// ---- test_action.py ----

func TestResolveScheme(t *testing.T) {
	cases := []struct {
		name    string
		action  *ARAuraAction
		current map[ARAuraField]any
		want    ARAuraScheme
	}{
		{"explicit", &ARAuraAction{Scheme: schemep(ARAuraSchemeBreathing)}, nil, ARAuraSchemeBreathing},
		{"none_uses_current", &ARAuraAction{}, map[ARAuraField]any{ARAuraFieldScheme: ARAuraSchemeMarquee}, ARAuraSchemeMarquee},
		{"on_uses_previous", &ARAuraAction{Scheme: schemep(ARAuraSchemeOn)}, map[ARAuraField]any{ARAuraFieldSchemePrev: ARAuraSchemeGradient}, ARAuraSchemeGradient},
		{"off", &ARAuraAction{Scheme: schemep(ARAuraSchemeOff)}, nil, ARAuraSchemeOff},
		{"unknown_falls_back_to_static", &ARAuraAction{}, map[ARAuraField]any{ARAuraFieldScheme: ARAuraSchemeUnknown}, ARAuraSchemeStatic},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := resolveScheme(tc.action, tc.current); got != tc.want {
				t.Fatalf("resolveScheme = %v, want %v", got, tc.want)
			}
		})
	}
}

type poster struct {
	calls []struct {
		ep      endpoint.AREndpoint
		request string
	}
}

func (p *poster) read(reply any) registry.ReadFunc {
	return func(_ context.Context, ep endpoint.AREndpoint, request string) (any, error) {
		p.calls = append(p.calls, struct {
			ep      endpoint.AREndpoint
			request string
		}{ep, request})
		return reply, nil
	}
}

func (p *poster) raw(reply *string) registry.FetchRawFunc {
	return func(_ context.Context, ep endpoint.AREndpoint, request string) (*string, error) {
		p.calls = append(p.calls, struct {
			ep      endpoint.AREndpoint
			request string
		}{ep, request})
		return reply, nil
	}
}

// fetch mirrors `_fetch`: FetchData returning the Aura content.
func fetch(current map[ARAuraField]any, fetched *int) registry.FetchDataFunc {
	return func(context.Context, source.DataSource, registry.Kwargs) (any, error) {
		if fetched != nil {
			*fetched++
		}
		return current, nil
	}
}

func TestRunAction(t *testing.T) {
	ctx := context.Background()
	id := identityWith(map[support.ARSupportType]any{support.ARSupportTypeAura: true})

	// Exact queries computed by the Python library for the same inputs
	cases := []struct {
		name    string
		action  *ARAuraAction
		current map[ARAuraField]any
		noData  bool
		want    string
	}{
		{"scheme_only", &ARAuraAction{Scheme: schemep(ARAuraSchemeStatic)}, map[ARAuraField]any{}, false,
			"ledg_scheme=2"},
		{"night_mode_on", &ARAuraAction{Scheme: schemep(ARAuraSchemeStatic), NightMode: boolp(true)},
			map[ARAuraField]any{ARAuraFieldNightMode: false}, false, "ledg_scheme=2&ledg_night_mode=1"},
		{"night_mode_off", &ARAuraAction{NightMode: boolp(false)},
			map[ARAuraField]any{ARAuraFieldNightMode: true}, false, "ledg_scheme=2&ledg_night_mode=0"},
		{"night_mode_left_unchanged", &ARAuraAction{Scheme: schemep(ARAuraSchemeStatic)},
			map[ARAuraField]any{ARAuraFieldNightMode: true}, false, "ledg_scheme=2"},
		{"night_mode_ignored_when_unsupported", &ARAuraAction{Scheme: schemep(ARAuraSchemeStatic), NightMode: boolp(true)},
			map[ARAuraField]any{}, false, "ledg_scheme=2"},
		{"no_get_data_callback", &ARAuraAction{}, nil, true, "ledg_scheme=2"},
		{"color_on_color_scheme", &ARAuraAction{Color: col(255, 0, 0)}, map[ARAuraField]any{
			ARAuraFieldScheme: ARAuraSchemeStatic,
			ARAuraFieldZones:  1,
			ARAuraFieldColors: map[ARAuraScheme][]color.Color{ARAuraSchemeStatic: {col(0, 0, 0, 100)}},
		}, false, "ledg_scheme=2&ledg_rgb=50%2C0%2C0"},
		{"color_skipped_without_zones", &ARAuraAction{Color: col(255, 0, 0)}, map[ARAuraField]any{
			ARAuraFieldScheme: ARAuraSchemeStatic,
			ARAuraFieldZones:  0,
		}, false, "ledg_scheme=2"},
		{"color_ignored_on_non_color_scheme", &ARAuraAction{Color: col(255, 0, 0)},
			map[ARAuraField]any{ARAuraFieldScheme: ARAuraSchemeRainbow}, false, "ledg_scheme=5"},
		{"night_colors_never_written", &ARAuraAction{Color: col(0, 0, 255)}, map[ARAuraField]any{
			ARAuraFieldScheme:    ARAuraSchemeStatic,
			ARAuraFieldZones:     1,
			ARAuraFieldNightMode: true,
			ARAuraFieldColors:    map[ARAuraScheme][]color.Color{ARAuraSchemeStatic: {col(0, 0, 0, 100)}},
		}, false, "ledg_scheme=2&ledg_rgb=0%2C0%2C50"},
		{"brightness_one_zone_defaults", &ARAuraAction{Brightness: intp(50), Zone: intp(1)}, map[ARAuraField]any{
			ARAuraFieldScheme: ARAuraSchemeGradient,
			ARAuraFieldZones:  3,
		}, false, "ledg_scheme=1&ledg_rgb=20%2C0%2C128%2C25%2C0%2C23%2C128%2C0%2C80"},
		{"on_restores_previous_with_palette", &ARAuraAction{
			Scheme: schemep(ARAuraSchemeOn),
			Color:  []color.Color{col(255, 0, 0), col(0, 255, 0)},
		}, map[ARAuraField]any{
			ARAuraFieldScheme:     ARAuraSchemeOff,
			ARAuraFieldSchemePrev: ARAuraSchemeMarquee,
			ARAuraFieldZones:      3,
		}, false, "ledg_scheme=7&ledg_rgb=128%2C0%2C0%2C0%2C110%2C0%2C128%2C0%2C0"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := &poster{}
			c := &registry.Context{Read: p.read(map[string]any{}), Identity: id}
			if !tc.noData {
				c.FetchData = fetch(tc.current, nil)
			}
			got, err := runAction(ctx, c, tc.action)
			if err != nil {
				t.Fatal(err)
			}
			if !got.(service.ARServiceResult).Success {
				t.Fatalf("result = %+v", got)
			}
			if len(p.calls) != 1 || p.calls[0].ep != endpoint.AREndpointSetAura {
				t.Fatalf("calls = %+v", p.calls)
			}
			if p.calls[0].request != tc.want {
				t.Fatalf("request = %s, want %s", p.calls[0].request, tc.want)
			}
		})
	}

	t.Run("failure_when_raw_none", func(t *testing.T) {
		p := &poster{}
		c := &registry.Context{Read: p.read(map[string]any{}), FetchRaw: p.raw(nil), Identity: id}
		got, _ := runAction(ctx, c, &ARAuraAction{Scheme: schemep(ARAuraSchemeStatic)})
		if got.(service.ARServiceResult).Success {
			t.Fatal("success on nil raw")
		}
	})

	t.Run("success_on_empty_body", func(t *testing.T) {
		p := &poster{}
		empty := ""
		c := &registry.Context{Read: p.read(map[string]any{}), FetchRaw: p.raw(&empty), Identity: id}
		got, _ := runAction(ctx, c, &ARAuraAction{Scheme: schemep(ARAuraSchemeStatic)})
		if !got.(service.ARServiceResult).Success || p.calls[0].request != "ledg_scheme=2" {
			t.Fatalf("result = %+v, calls = %+v", got, p.calls)
		}
	})

	for _, tc := range []struct {
		name     string
		identity *device.ARDeviceIdentity
	}{
		{"none", nil},
		{"no-aura", identityWith(map[support.ARSupportType]any{support.ARSupportTypeAura: false})},
	} {
		t.Run("action_ignored_"+tc.name, func(t *testing.T) {
			p := &poster{}
			fetched := 0
			c := &registry.Context{
				Read: p.read(map[string]any{}), FetchRaw: p.raw(nil),
				FetchData: fetch(nil, &fetched), Identity: tc.identity,
			}
			got, _ := runAction(ctx, c, &ARAuraAction{Scheme: schemep(ARAuraSchemeStatic)})
			if got.(service.ARServiceResult).Success || len(p.calls) != 0 || fetched != 0 {
				t.Fatalf("result = %+v, calls = %d, fetched = %d", got, len(p.calls), fetched)
			}
		})
	}
}

func TestActionFields(t *testing.T) {
	a := &ARAuraAction{Scheme: schemep(ARAuraSchemeWave), Brightness: intp(50), Zone: intp(2)}
	if *a.Scheme != ARAuraSchemeWave || *a.Brightness != 50 || *a.Zone != 2 {
		t.Fatalf("action = %+v", a)
	}
	if e := registry.Action(a); e == nil || e.Run == nil {
		t.Fatal("ARAuraAction is not registered")
	}
}

// TestTranslateCrossCheck compares a full translation with the canonical
// output of the Python library for the same nvram values.
func TestTranslateCrossCheck(t *testing.T) {
	const golden = `{"brightness": 253, "color": {"blue": 183, "brightness": 253, "green": 158, "red": 230}, "colors": {"1": [{"blue": 0, "brightness": 255, "green": 0, "red": 255}, {"blue": 0, "brightness": 255, "green": 255, "red": 0}, {"blue": 255, "brightness": 255, "green": 0, "red": 0}], "2": [{"blue": 255, "brightness": 128, "green": 0, "red": 40}, {"blue": 128, "brightness": 128, "green": 0, "red": 255}, {"blue": 3, "brightness": 153, "green": 255, "red": 109}], "3": [{"blue": 40, "brightness": 253, "green": 2, "red": 255}, {"blue": 255, "brightness": 14, "green": 219, "red": 182}, {"blue": 255, "brightness": 199, "green": 255, "red": 255}], "5": [{"blue": 0, "brightness": 0, "green": 0, "red": 0}]}, "night_mode": false, "scheme": 3, "scheme_prev": 2, "state": true, "zones": 3}`
	var want any
	if err := json.Unmarshal([]byte(golden), &want); err != nil {
		t.Fatal(err)
	}
	got := translate(nv(
		nvram.ARNvramTypeAura, "1",
		nvram.ARNvramTypeAuraScheme, "3",
		nvram.ARNvramTypeAuraSchemePrev, "2",
		nvram.ARNvramTypeAuraCount, "3",
		nvram.ARNvramTypeAuraNightMode, "0",
		rgb(1), "128,0,0,0,128,0,0,0,128",
		rgb(2), "10,0,64,64,0,32,33,77,1",
		rgb(3), "127,1,20,5,6,7,100,100,100",
		rgb(5), "0,0,0",
	), fakeIdentity(true, true))
	if diffs := replay.Diff(want, got); len(diffs) > 0 {
		t.Fatalf("differs from Python:\n%v", diffs)
	}
}
