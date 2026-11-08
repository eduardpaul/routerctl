package source

import (
	"context"
	"reflect"
	"testing"

	"github.com/eduardpaul/routerctl/asusrouter/modules/aimesh"
	"github.com/eduardpaul/routerctl/asusrouter/modules/device"
	"github.com/eduardpaul/routerctl/asusrouter/modules/endpoint"
	"github.com/eduardpaul/routerctl/asusrouter/modules/source"
	"github.com/eduardpaul/routerctl/asusrouter/modules/support"
	"github.com/eduardpaul/routerctl/asusrouter/registry"
	"github.com/eduardpaul/routerctl/asusrouter/tools/readers"
)

const rawOnboarding = `get_cfg_clientlist = [[{"mac":"AA:00:00:00:00:00","online":"1",` +
	`"level":"0","re_path":"0","ap2g":"AA:00:00:00:00:10"}]][0];` + "\n"

// identity builds an identity with the AiMesh support flag set.
func identity(aimeshSupport bool) *device.ARDeviceIdentity {
	return device.ARDeviceIdentityBuild(device.IdentityData{
		source.IDOf(support.ARSupportSourceUniversal): map[support.ARSupportType]any{
			support.ARSupportTypeAimesh: aimeshSupport,
		},
	})
}

type readCall struct {
	ep      endpoint.AREndpoint
	request string
}

func fakeRead(result any, calls *[]readCall) registry.ReadFunc {
	return func(_ context.Context, ep endpoint.AREndpoint, request string) (any, error) {
		*calls = append(*calls, readCall{ep, request})
		return result, nil
	}
}

// === TestARAiMeshSource ===

func TestARAiMeshSourceIsDataSource(t *testing.T) {
	var _ source.DataSource = &ARAiMeshSource{}
}

func TestARAiMeshSourceDedupByType(t *testing.T) {
	a, b := &ARAiMeshSource{}, &ARAiMeshSource{}
	if source.IDOf(a) != source.IDOf(b) {
		t.Fatal("IDs differ")
	}
	set := map[source.ID]struct{}{source.IDOf(a): {}, source.IDOf(b): {}}
	if len(set) != 1 {
		t.Fatal("dedup")
	}
}

func TestARAiMeshSourceNotEqualOtherType(t *testing.T) {
	// Go adaptation: compare against another source type
	if source.IDOf(&ARAiMeshSource{}) == source.IDOf(support.ARSupportSourceUniversal) {
		t.Fatal("equal to another type")
	}
}

func TestARAiMeshSourceRepr(t *testing.T) {
	if got := source.Repr(&ARAiMeshSource{}); got != "<ARAiMeshSource>" {
		t.Fatalf("repr %q", got)
	}
}

// === TestGetState ===

func TestGetStateFetchesOnboarding(t *testing.T) {
	var calls []readCall
	resp := map[string]any{"get_cfg_clientlist": []any{[]any{}}}
	c := &registry.Context{Read: fakeRead(resp, &calls), Identity: identity(true)}

	result, err := fetchState(context.Background(), c, ARAiMeshSourceUniversal)
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(calls, []readCall{{endpoint.AREndpointFetchOnboarding, ""}}) {
		t.Fatalf("calls %v", calls)
	}
	if !reflect.DeepEqual(result, resp) {
		t.Fatalf("result %v", result)
	}
}

func TestGetStateNonDictResponse(t *testing.T) {
	var calls []readCall
	c := &registry.Context{Read: fakeRead("not-a-dict", &calls), Identity: identity(true)}

	result, _ := fetchState(context.Background(), c, ARAiMeshSourceUniversal)
	if m, ok := result.(map[string]any); !ok || len(m) != 0 {
		t.Fatalf("result %v", result)
	}
}

func TestGetStateUnsupportedSkipsFetch(t *testing.T) {
	var calls []readCall
	c := &registry.Context{Read: fakeRead(nil, &calls), Identity: identity(false)}

	result, _ := fetchState(context.Background(), c, ARAiMeshSourceUniversal)
	if m, ok := result.(map[string]any); !ok || len(m) != 0 {
		t.Fatalf("result %v", result)
	}
	if len(calls) != 0 {
		t.Fatalf("calls %v", calls)
	}
}

func TestGetStateNoIdentitySkipsFetch(t *testing.T) {
	var calls []readCall
	c := &registry.Context{Read: fakeRead(nil, &calls)}

	result, _ := fetchState(context.Background(), c, ARAiMeshSourceUniversal)
	if m, ok := result.(map[string]any); !ok || len(m) != 0 {
		t.Fatalf("result %v", result)
	}
	if len(calls) != 0 {
		t.Fatalf("calls %v", calls)
	}
}

// === TestTranslateState ===

func TestTranslateStateBuildsTopology(t *testing.T) {
	data := readers.ReadJSVariables(rawOnboarding)

	tp, ok := translateState(data, nil).(*aimesh.ARAiMeshTopology)
	if !ok {
		t.Fatal("not a topology")
	}
	if tp.Root() == nil {
		t.Fatal("no root")
	}
}

func TestTranslateStateStatusAndOnboarding(t *testing.T) {
	raw := rawOnboarding +
		`get_onboardingstatus = [{"cfg_ready":"1",` +
		`"cfg_re_maxnum":"16","cfg_obrssi":"-20"}][0];` + "\n" +
		"get_onboardinglist = [{}][0];\n"

	tp := translateState(readers.ReadJSVariables(raw), nil).(*aimesh.ARAiMeshTopology)

	if !tp.Status.Ready {
		t.Fatal("ready")
	}
	if tp.Status.ReMaxnum == nil || *tp.Status.ReMaxnum != 16 {
		t.Fatalf("re_maxnum %v", tp.Status.ReMaxnum)
	}
	if tp.Status.Rssi == nil || *tp.Status.Rssi != -20 {
		t.Fatalf("rssi %v", tp.Status.Rssi)
	}
	if tp.Onboarding == nil || len(tp.Onboarding) != 0 {
		t.Fatalf("onboarding %v", tp.Onboarding)
	}
}

func TestTranslateStateEmptyOrBad(t *testing.T) {
	cases := []struct {
		name string
		data any
	}{
		{"str", "not-a-dict"},
		{"empty", map[string]any{}},
		{"empty_list", map[string]any{"get_cfg_clientlist": []any{}}},
		{"not_list", map[string]any{"get_cfg_clientlist": "x"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			tp := translateState(c.data, nil).(*aimesh.ARAiMeshTopology)
			if len(tp.Nodes) != 0 {
				t.Fatalf("nodes %d", len(tp.Nodes))
			}
		})
	}
}

// TestReaderRegistered mirrors test_reader_registered: Go funcs are not
// comparable, so the reader is checked by its output.
func TestReaderRegistered(t *testing.T) {
	got := endpoint.GetEndpointReader(endpoint.AREndpointFetchOnboarding)(rawOnboarding)
	if !reflect.DeepEqual(got, any(readers.ReadJSVariables(rawOnboarding))) {
		t.Fatalf("reader output %v", got)
	}
}

// TestRegistersCallables mirrors test_registers_callables.
func TestRegistersCallables(t *testing.T) {
	entry := registry.SourceByName("ARAiMeshSource")
	if entry == nil {
		t.Fatal("not registered")
	}
	if entry.Fetch == nil || entry.Translate == nil || entry.Multi() {
		t.Fatalf("entry %+v", entry)
	}
	if _, ok := entry.Default().(*ARAiMeshSource); !ok {
		t.Fatal("default instance")
	}
}
