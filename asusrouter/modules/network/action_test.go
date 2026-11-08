package network

import (
	"context"
	"strings"
	"testing"

	"github.com/eduardpaul/routerctl/asusrouter/modules/action"
	"github.com/eduardpaul/routerctl/asusrouter/modules/endpoint"
	"github.com/eduardpaul/routerctl/asusrouter/modules/nvram"
	"github.com/eduardpaul/routerctl/asusrouter/modules/service"
	"github.com/eduardpaul/routerctl/asusrouter/modules/source"
	"github.com/eduardpaul/routerctl/asusrouter/registry"
	"github.com/eduardpaul/routerctl/asusrouter/tools/identifiers"
)

var actionSdnRl = enc("<1>MAINFH>1>0>0>2<3>Customized>1>0>0>4")

var (
	sdnHandle    = ARNetworkHandle{Backend: ARNetworkBackendSdn, SdnIdx: intp(3), ApPrefix: strp("apg"), ApIdx: intp(4)}
	legacyHandle = ARNetworkHandle{Backend: ARNetworkBackendLegacy, Units: []ARNetworkHandleUnit{{Unit: 0, Slot: intp(1)}}}
)

func testNetworks() map[ARNetworkType][]map[ARNetworkField]any {
	return map[ARNetworkType][]map[ARNetworkField]any{
		ARNetworkTypeMainfh: {
			{ARNetworkFieldSsid: identifiers.SsidFromValueSafe("Home"), ARNetworkFieldHandle: sdnHandle},
		},
		ARNetworkTypeGuest: {
			{ARNetworkFieldSsid: identifiers.SsidFromValueSafe("Guest"), ARNetworkFieldHandle: legacyHandle},
			{ARNetworkFieldSsid: identifiers.SsidFromValueSafe("NoHandle")},
		},
	}
}

func TestFindHandleBySsid(t *testing.T) {
	networks := testNetworks()
	cases := []struct {
		name string
		ssid any
		want *ARNetworkHandle
	}{
		// A matching Ssid object returns the profile's handle
		{"ssid object", identifiers.SsidFromValueSafe("Guest"), &legacyHandle},
		{"ssid value", identifiers.NewSsid("Guest"), &legacyHandle},
		// A matching plain string returns the profile's handle
		{"string", "Home", &sdnHandle},
		// An unknown SSID returns nil
		{"not found", "Missing", nil},
		// A matching profile that carries no handle returns nil
		{"no handle", "NoHandle", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := FindHandleBySsid(networks, tc.ssid)
			switch {
			case tc.want == nil && got != nil:
				t.Errorf("got %s, want nil", got)
			case tc.want != nil && (got == nil || !got.Equal(*tc.want)):
				t.Errorf("got %v, want %s", got, tc.want)
			}
		})
	}
}

func TestARNetworkAction(t *testing.T) {
	// The handle and desired state are stored as given
	a := NewARNetworkAction(legacyHandle, true)
	if !a.Handle.Equal(legacyHandle) || !a.State {
		t.Errorf("action %+v", a)
	}

	// Actions are equal by handle and state
	id := func(x *ARNetworkAction) source.ID { return source.IDOf(actionSource{x}) }
	same := &ARNetworkAction{
		Handle: ARNetworkHandle{Backend: ARNetworkBackendLegacy, Units: []ARNetworkHandleUnit{{Unit: 0, Slot: intp(1)}}},
		State:  true,
	}
	assertEqual(t, id(a), id(same))
	if id(a) == id(NewARNetworkAction(legacyHandle, false)) {
		t.Error("different state must differ")
	}
	if id(a) == id(NewARNetworkAction(sdnHandle, true)) {
		t.Error("different handle must differ")
	}

	// The action resolves a run_action callable via the registry
	if entry := registry.Action(a); entry == nil || entry.Run == nil {
		t.Error("action not registered")
	}
}

// actionSource exposes an action key as a data source key for ID checks.
type actionSource struct{ a action.ARAction }

func (s actionSource) SourceName() string { return s.a.ActionName() }
func (s actionSource) SourceKey() []any   { return s.a.ActionKey() }

// pusher records PUSH_DATA calls.
type pusher struct {
	calls []string
	reply any
}

func (p *pusher) raw(_ context.Context, ep endpoint.AREndpoint, request string) (*string, error) {
	if ep != endpoint.AREndpointPushData {
		panic("unexpected endpoint " + string(ep))
	}
	p.calls = append(p.calls, request)
	if s, ok := p.reply.(string); ok {
		return &s, nil
	}
	return nil, nil
}

func (p *pusher) read(_ context.Context, ep endpoint.AREndpoint, request string) (any, error) {
	if ep != endpoint.AREndpointPushData {
		panic("unexpected endpoint " + string(ep))
	}
	p.calls = append(p.calls, request)
	return p.reply, nil
}

// nvramData is a FetchDataMulti answering nvram items from fixed values.
type nvramData struct {
	values map[string]any
	calls  [][]source.DataSource
}

func (n *nvramData) fetch(_ context.Context, srcs []source.DataSource, _ registry.Kwargs) (map[source.ID]any, error) {
	n.calls = append(n.calls, srcs)
	if n.values == nil {
		return nil, nil
	}
	out := map[source.ID]any{}
	for _, src := range srcs {
		if item, ok := src.(nvram.ARNvramType); ok {
			if v, ok := n.values[string(item)]; ok {
				out[source.IDOf(src)] = v
			}
		}
	}
	return out, nil
}

func TestRunActionSdnReadsThenPushes(t *testing.T) {
	// SDN fetches sdn_rl, then pushes the toggled list and apg enable
	data := &nvramData{values: map[string]any{"sdn_rl": actionSdnRl}}
	push := &pusher{reply: "NOT MODIFIED"}
	c := &registry.Context{Read: (&pusher{}).read, FetchDataMulti: data.fetch, FetchRaw: push.raw}

	result, err := runAction(context.Background(), c, NewARNetworkAction(sdnHandle, true))
	if err != nil {
		t.Fatal(err)
	}
	assertEqual(t, result.(service.ARServiceResult).Success, true)
	// sdn_rl fetched via the data pipeline
	if len(data.calls) != 1 || len(data.calls[0]) != 1 || data.calls[0][0] != nvram.ARNvramTypeSdnRl {
		t.Errorf("data calls %v", data.calls)
	}
	// request pushed via the raw callback
	if len(push.calls) != 1 {
		t.Fatalf("push calls %v", push.calls)
	}
	for _, want := range []string{`"rc_service":"restart_wireless;restart_sdn 3;"`, `"apg4_enable":1`} {
		if !strings.Contains(push.calls[0], want) {
			t.Errorf("request %s misses %s", push.calls[0], want)
		}
	}
}

func TestRunActionSdnFails(t *testing.T) {
	cases := []struct {
		name  string
		fetch registry.FetchDataMultiFunc
	}{
		// A missing sdn_rl fails without a push
		{"missing list", (&nvramData{values: map[string]any{}}).fetch},
		// SDN without a data callback fails without a push
		{"no data callback", nil},
		// A non-dict nvram reply fails without a push
		{"non-dict read", (&nvramData{}).fetch},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			push := &pusher{}
			c := &registry.Context{Read: (&pusher{}).read, FetchDataMulti: tc.fetch, FetchRaw: push.raw}
			result, err := runAction(context.Background(), c, NewARNetworkAction(sdnHandle, true))
			if err != nil {
				t.Fatal(err)
			}
			assertCanon(t, result, service.ARServiceResult{Success: false})
			assertEqual(t, len(push.calls), 0)
		})
	}
}

func TestRunActionLegacyPushesViaCallback(t *testing.T) {
	// Legacy builds its payload and pushes via the plain callback
	push := &pusher{reply: map[string]any{"run_service": "restart_wireless;restart_firewall"}}
	c := &registry.Context{Read: push.read}

	result, err := runAction(context.Background(), c, NewARNetworkAction(legacyHandle, true))
	if err != nil {
		t.Fatal(err)
	}
	assertEqual(t, result.(service.ARServiceResult).Success, true)
	if len(push.calls) != 1 {
		t.Fatalf("push calls %v", push.calls)
	}
	for _, want := range []string{`"wl0.1_bss_enabled":1`, `"wl0.1_expire":0`} {
		if !strings.Contains(push.calls[0], want) {
			t.Errorf("request %s misses %s", push.calls[0], want)
		}
	}
}
