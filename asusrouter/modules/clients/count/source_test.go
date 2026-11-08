package count

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/eduardpaul/routerctl/asusrouter/modules/aimesh"
	"github.com/eduardpaul/routerctl/asusrouter/modules/device"
	"github.com/eduardpaul/routerctl/asusrouter/modules/endpoint"
	"github.com/eduardpaul/routerctl/asusrouter/modules/nvram"
	"github.com/eduardpaul/routerctl/asusrouter/modules/source"
	"github.com/eduardpaul/routerctl/asusrouter/modules/support"
	"github.com/eduardpaul/routerctl/asusrouter/registry"
	"github.com/eduardpaul/routerctl/asusrouter/tools/identifiers"
)

const (
	router  = "AA:BB:CC:00:00:01"
	node    = "AA:BB:CC:00:00:02"
	c1      = "AA:BB:CC:00:00:11"
	c2      = "AA:BB:CC:00:00:22"
	aiboard = "AA:BB:CC:00:00:33"
)

func mac(s string) identifiers.MacAddress {
	m, err := identifiers.MacAddressFromValue(s)
	if err != nil {
		panic(err)
	}
	return m
}

func testIdentity(ai bool, nodes ...string) *device.ARDeviceIdentity {
	data := device.IdentityData{source.IDOf(nvram.ARNvramTypeMac): router}
	if ai {
		data[source.IDOf(support.ARSupportSourceUniversal)] = map[support.ARSupportType]any{support.ARSupportTypeAi: true}
	}
	identity := device.ARDeviceIdentityBuild(data)
	if len(nodes) > 0 {
		topology := aimesh.NewARAiMeshTopology()
		for _, m := range nodes {
			topology.Add(aimesh.NewARAiMeshNode(mac(m)))
		}
		identity.UpdateAimesh(topology)
	}
	return identity
}

func onlineWired(m string, node string) map[string]any {
	entry := map[string]any{"mac": m, "isOnline": "1", "isWL": "0"}
	if node != "" {
		entry["amesh_papMac"] = node
	}
	return entry
}

// reader records calls and answers with fn.
type reader struct {
	calls    int
	requests []string
	fn       func(ep endpoint.AREndpoint) any
}

func (r *reader) read(_ context.Context, ep endpoint.AREndpoint, request string) (any, error) {
	r.calls++
	r.requests = append(r.requests, request)
	return r.fn(ep), nil
}

func run(t *testing.T, src *ARClientsCountSource, identity *device.ARDeviceIdentity, r *reader) map[identifiers.MacAddress]int {
	t.Helper()
	got, err := fetchState(context.Background(), &registry.Context{Read: r.read, Identity: identity}, src)
	if err != nil {
		t.Fatal(err)
	}
	return got.(map[identifiers.MacAddress]int)
}

func TestSource(t *testing.T) {
	if source.IDOf(&ARClientsCountSource{}) != source.IDOf(&ARClientsCountSource{}) ||
		source.IDOf(NewARClientsCountSource(node)) != source.IDOf(NewARClientsCountSource(node)) ||
		source.IDOf(NewARClientsCountSource(node)) == source.IDOf(&ARClientsCountSource{}) {
		t.Error("sources should be equal by target")
	}

	s := &ARClientsCountSource{}
	if s.Target() != nil {
		t.Error("default target should be nil")
	}
	s.SetTarget(node)
	if s.Target() == nil || *s.Target() != mac(node) {
		t.Error("target setter failed")
	}

	if got := source.Repr(&ARClientsCountSource{}); got != "<ARClientsCountSource (None)>" {
		t.Errorf("repr = %q", got)
	}
	if ARClientsCountSourceUniversal.Target() != nil {
		t.Error("universal instance should target all nodes")
	}
}

func TestTargetMacs(t *testing.T) {
	if got := targetMacs(NewARClientsCountSource(node), testIdentity(false, router)); !reflect.DeepEqual(got, []identifiers.MacAddress{mac(node)}) {
		t.Errorf("explicit target = %v", got)
	}
	if got := targetMacs(&ARClientsCountSource{}, testIdentity(false, router, node)); !reflect.DeepEqual(got, []identifiers.MacAddress{mac(router), mac(node)}) {
		t.Errorf("all nodes = %v", got)
	}
	if got := targetMacs(&ARClientsCountSource{}, testIdentity(false)); !reflect.DeepEqual(got, []identifiers.MacAddress{mac(router)}) {
		t.Errorf("router fallback = %v", got)
	}
	if got := targetMacs(&ARClientsCountSource{}, device.NewARDeviceIdentity()); len(got) != 0 {
		t.Errorf("no router mac = %v", got)
	}
}

func counts(values ...any) *reader {
	return &reader{fn: func(endpoint.AREndpoint) any { return map[string]any{"count": values} }}
}

func TestModern(t *testing.T) {
	r := counts(int64(20), int64(21), int64(22))
	if got := run(t, NewARClientsCountSource(node), testIdentity(false), r); !reflect.DeepEqual(got, map[identifiers.MacAddress]int{mac(node): 22}) {
		t.Errorf("newest wins = %v", got)
	}
	if !strings.Contains(r.requests[0], "node_mac=AA%3ABB%3ACC%3A00%3A00%3A02") {
		t.Errorf("request = %s", r.requests[0])
	}

	for _, c := range []struct {
		name   string
		values []any
		want   int
	}{
		{"trailing zero falls back one point", []any{int64(20), int64(0)}, 20},
		{"two trailing zeros are zero", []any{int64(20), int64(0), int64(0)}, 0},
		{"single zero point is zero", []any{int64(0)}, 0},
	} {
		if got := run(t, NewARClientsCountSource(node), testIdentity(false), counts(c.values...)); !reflect.DeepEqual(got, map[identifiers.MacAddress]int{mac(node): c.want}) {
			t.Errorf("%s: %v", c.name, got)
		}
	}

	r = counts(int64(7))
	if got := run(t, &ARClientsCountSource{}, testIdentity(false, router, node), r); !reflect.DeepEqual(got, map[identifiers.MacAddress]int{mac(router): 7, mac(node): 7}) || r.calls != 2 {
		t.Errorf("all nodes = %v (%d calls)", got, r.calls)
	}

	r = counts()
	if got := run(t, &ARClientsCountSource{}, device.NewARDeviceIdentity(), r); len(got) != 0 || r.calls != 0 {
		t.Errorf("no macs = %v (%d calls)", got, r.calls)
	}
}

func legacy(clientlist map[string]any) *reader {
	return &reader{fn: func(ep endpoint.AREndpoint) any {
		if ep == endpoint.AREndpointFetchDiagnosticsActiveClient {
			return map[string]any{"count": []any{}}
		}
		return map[string]any{"get_clientlist": clientlist}
	}}
}

func TestLegacyFallback(t *testing.T) {
	withName := func(entry map[string]any, name string) map[string]any {
		entry["name"] = name
		return entry
	}
	cases := []struct {
		name     string
		identity *device.ARDeviceIdentity
		list     map[string]any
		want     map[identifiers.MacAddress]int
	}{
		{"counts per node", testIdentity(false, router, node),
			map[string]any{c1: onlineWired(c1, ""), c2: onlineWired(c2, node)},
			map[identifiers.MacAddress]int{mac(router): 1, mac(node): 1}},
		{"requested node without clients is zero", testIdentity(false, router, node),
			map[string]any{c1: onlineWired(c1, "")},
			map[identifiers.MacAddress]int{mac(router): 1, mac(node): 0}},
		{"ai board excluded", testIdentity(true),
			map[string]any{c1: onlineWired(c1, ""), aiboard: withName(onlineWired(aiboard, ""), "AiBoard-1")},
			map[identifiers.MacAddress]int{mac(router): 1}},
		{"ai board kept without ai support", testIdentity(false),
			map[string]any{c1: onlineWired(c1, ""), aiboard: withName(onlineWired(aiboard, ""), "AiBoard-1")},
			map[identifiers.MacAddress]int{mac(router): 2}},
		{"offline and nodeless ignored", testIdentity(false),
			map[string]any{c1: onlineWired(c1, ""), c2: map[string]any{"mac": c2, "isOnline": "0", "isWL": "0"}},
			map[identifiers.MacAddress]int{mac(router): 1}},
	}
	for _, c := range cases {
		if got := run(t, &ARClientsCountSource{}, c.identity, legacy(c.list)); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: %v", c.name, got)
		}
	}
}

func TestRegistersCallable(t *testing.T) {
	e := registry.SourceByName("ARClientsCountSource")
	if e == nil || e.Fetch == nil || e.Translate != nil {
		t.Error("ARClientsCountSource should register fetch only")
	}
}
