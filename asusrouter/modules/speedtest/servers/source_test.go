package servers

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/eduardpaul/routerctl/asusrouter/modules/device"
	"github.com/eduardpaul/routerctl/asusrouter/modules/endpoint"
	"github.com/eduardpaul/routerctl/asusrouter/modules/source"
	"github.com/eduardpaul/routerctl/asusrouter/modules/speedtest"
	"github.com/eduardpaul/routerctl/asusrouter/modules/support"
	"github.com/eduardpaul/routerctl/asusrouter/registry"
)

func init() { pollInterval = time.Millisecond }

func identity(speedtestSupport bool) *device.ARDeviceIdentity {
	return device.ARDeviceIdentityBuild(device.IdentityData{
		source.IDOf(support.ARSupportSourceUniversal): map[support.ARSupportType]any{support.ARSupportTypeSpeedtest: speedtestSupport},
	})
}

var hook = string(endpoint.ARHookOoklaSpeedtestServers)

func servers() []any {
	return []any{map[string]any{"id": int64(1), "name": "A", "location": "L"}, map[string]any{}}
}

type call struct {
	endpoint endpoint.AREndpoint
	request  string
}

func sequence(calls *[]call, replies ...any) registry.ReadFunc {
	return func(_ context.Context, ep endpoint.AREndpoint, request string) (any, error) {
		i := len(*calls)
		*calls = append(*calls, call{ep, request})
		if i >= len(replies) {
			i = len(replies) - 1
		}
		return replies[i], nil
	}
}

func TestSource(t *testing.T) {
	if e := registry.SourceByName("ARSpeedTestServersSource"); e == nil || e.Fetch == nil {
		t.Error("not registered")
	}
	if source.IDOf(&ARSpeedTestServersSource{}) != source.IDOf(ARSpeedTestServersSourceUniversal) {
		t.Error("equal by type")
	}
}

func TestFetchState(t *testing.T) {
	ctx := context.Background()

	var calls []call
	got, _ := fetchState(ctx, &registry.Context{Read: sequence(&calls, map[string]any{hook: servers()}), Identity: identity(true)}, ARSpeedTestServersSourceUniversal)
	if !reflect.DeepEqual(got, servers()) || calls[0].endpoint != endpoint.AREndpointFetchData {
		t.Errorf("default_reads_list: %v", got)
	}

	calls = nil
	got, _ = fetchState(ctx, &registry.Context{Read: sequence(&calls, nil), Identity: identity(true)}, ARSpeedTestServersSourceUniversal)
	if m, ok := got.(map[string]any); !ok || len(m) != 0 {
		t.Errorf("extract_non_dict: %v", got)
	}

	calls = nil
	got, _ = fetchState(ctx, &registry.Context{Read: sequence(&calls, nil), Identity: identity(false)}, ARSpeedTestServersSourceUniversal)
	if m, ok := got.(map[string]any); !ok || len(m) != 0 || len(calls) != 0 {
		t.Errorf("unsupported_skips_fetch: %v", got)
	}

	calls = nil
	full := []any{map[string]any{"id": int64(0)}, map[string]any{"id": int64(1)}, map[string]any{"id": int64(2)}}
	c := &registry.Context{Read: sequence(&calls, nil, map[string]any{hook: full}), Identity: identity(true), Kwargs: registry.Kwargs{"refresh": true}}
	got, _ = fetchState(ctx, c, ARSpeedTestServersSourceUniversal)
	if !reflect.DeepEqual(got, full) || calls[0].endpoint != endpoint.AREndpointRunSpeedtest || calls[0].request != "type=list&id=" {
		t.Errorf("refresh_dispatches_and_polls: %v, calls %v", got, calls)
	}
}

func TestTranslateState(t *testing.T) {
	got := translateState(servers(), nil).([]speedtest.ARSpeedTestServer)
	if len(got) != 1 || *got[0].Name != "A" {
		t.Errorf("got %+v", got)
	}
}
