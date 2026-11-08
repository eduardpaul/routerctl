package clients

import (
	"context"
	"reflect"
	"testing"

	"github.com/eduardpaul/routerctl/asusrouter/modules/endpoint"
	"github.com/eduardpaul/routerctl/asusrouter/modules/source"
	"github.com/eduardpaul/routerctl/asusrouter/registry"
	"github.com/eduardpaul/routerctl/asusrouter/tools/identifiers"
)

func TestARClientsSource(t *testing.T) {
	if source.IDOf(&ARClientsSource{}) != source.IDOf(&ARClientsSource{}) {
		t.Error("all clients sources should be equal")
	}
	if got := source.Repr(&ARClientsSource{}); got != "<ARClientsSource>" {
		t.Errorf("repr = %q", got)
	}
}

func TestMergeHistoryStashes(t *testing.T) {
	src := &ARClientsSource{}
	current := map[identifiers.MacAddress]ARClient{mac(c1): NewARClient(mac(c1))}
	result := src.MergeHistory(current)
	if !reflect.DeepEqual(result, current) || !reflect.DeepEqual(src.prev, current) {
		t.Errorf("result = %v, prev = %v", result, src.prev)
	}
}

func TestMergeHistoryMarksVanishedOffline(t *testing.T) {
	src := &ARClientsSource{}
	gone := NewARClient(mac(c2))
	gone.Online = true
	conn := NewARClientConnection()
	gone.Connection = &conn
	src.MergeHistory(map[identifiers.MacAddress]ARClient{mac(c2): gone})

	result := src.MergeHistory(map[identifiers.MacAddress]ARClient{mac(c1): NewARClient(mac(c1))})
	kept, ok := result[mac(c2)]
	if !ok || kept.Online || kept.Connection != nil {
		t.Errorf("kept = %+v", kept)
	}
}

func TestFetchState(t *testing.T) {
	raw := map[string]any{"get_clientlist": map[string]any{
		c1: map[string]any{"mac": c1, "isOnline": "1", "isWL": "0"},
	}}
	for name, c := range map[string]*registry.Context{
		"with identity":    {Identity: testIdentity()},
		"without identity": {},
	} {
		t.Run(name, func(t *testing.T) {
			var gotEp endpoint.AREndpoint
			var gotReq string
			c.Read = func(_ context.Context, ep endpoint.AREndpoint, request string) (any, error) {
				gotEp, gotReq = ep, request
				return raw, nil
			}
			result, err := fetchState(context.Background(), c, &ARClientsSource{})
			if err != nil {
				t.Fatal(err)
			}
			if _, ok := result.(map[identifiers.MacAddress]ARClient)[mac(c1)]; !ok {
				t.Errorf("result = %v", result)
			}
			if gotEp != endpoint.AREndpointFetchData ||
				gotReq != "hook=get_clientlist();get_clientlist_from_json_database()" {
				t.Errorf("request = %s %s", gotEp, gotReq)
			}
		})
	}
}

func TestRegistersCallable(t *testing.T) {
	e := registry.SourceByName("ARClientsSource")
	if e == nil || e.Fetch == nil || e.Translate != nil {
		t.Error("ARClientsSource should register fetch only")
	}
}
