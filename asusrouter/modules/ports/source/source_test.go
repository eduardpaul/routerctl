package source

import (
	"context"
	"reflect"
	"testing"

	"github.com/eduardpaul/routerctl/asusrouter/modules/device"
	"github.com/eduardpaul/routerctl/asusrouter/modules/endpoint"
	"github.com/eduardpaul/routerctl/asusrouter/modules/nvram"
	"github.com/eduardpaul/routerctl/asusrouter/modules/ports"
	"github.com/eduardpaul/routerctl/asusrouter/modules/source"
	"github.com/eduardpaul/routerctl/asusrouter/registry"
	"github.com/eduardpaul/routerctl/asusrouter/tools/identifiers"
)

const testMac = "12:34:56:78:9A:BC"

var portStatus = map[string]any{
	"node_info": map[string]any{},
	"port_info": map[string]any{
		testMac: map[string]any{
			"W0": map[string]any{"is_on": "1", "cap": "1", "max_rate": "1000", "link_rate": "1000"},
		},
	},
}

var ethernet = map[string]any{"portSpeed": map[string]any{"LAN 1": "G"}}

// identity builds an identity with the given MAC.
func identity(mac string) *device.ARDeviceIdentity {
	if mac == "" {
		return device.NewARDeviceIdentity()
	}
	return device.ARDeviceIdentityBuild(device.IdentityData{
		source.IDOf(nvram.ARNvramTypeMac): mac,
	})
}

type call struct {
	ep      endpoint.AREndpoint
	request string
}

func reader(mapping map[endpoint.AREndpoint]any, calls *[]call) registry.ReadFunc {
	return func(ctx context.Context, ep endpoint.AREndpoint, request string) (any, error) {
		*calls = append(*calls, call{ep, request})
		if v, ok := mapping[ep]; ok {
			return v, nil
		}
		return map[string]any{}, nil
	}
}

func mustMac(t *testing.T, s string) identifiers.MacAddress {
	t.Helper()
	m := identifiers.MacAddressFromValueSafe(s)
	if m == nil {
		t.Fatalf("bad MAC %q", s)
	}
	return *m
}

func TestARPortsSource(t *testing.T) {
	t.Run("IsDataSource", func(t *testing.T) {
		var _ source.DataSource = &ARPortsSource{}
		if registry.SourceByName("ARPortsSource") == nil {
			t.Error("ARPortsSource is not registered")
		}
	})
	t.Run("UniversalInstance", func(t *testing.T) {
		if ARPortsSourceUniversal == nil || ARPortsSourceUniversal.SourceName() != "ARPortsSource" {
			t.Error("ARPortsSourceUniversal must be an ARPortsSource")
		}
	})
}

func TestFetchState(t *testing.T) {
	ctx := context.Background()
	t.Run("PrefersPortStatus", func(t *testing.T) {
		var calls []call
		c := &registry.Context{
			Read:     reader(map[endpoint.AREndpoint]any{endpoint.AREndpointFetchPortStatus: portStatus}, &calls),
			Identity: identity(testMac),
		}
		result, err := fetchState(ctx, c, ARPortsSourceUniversal)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(result, portStatus) {
			t.Errorf("result = %v", result)
		}
		if want := []call{{endpoint.AREndpointFetchPortStatus, "node_mac=all"}}; !reflect.DeepEqual(calls, want) {
			t.Errorf("calls = %v, want %v", calls, want)
		}
	})
	t.Run("FallsBackToEthernet", func(t *testing.T) {
		var calls []call
		c := &registry.Context{
			Read:     reader(map[endpoint.AREndpoint]any{endpoint.AREndpointFetchPortsEthernet: ethernet}, &calls),
			Identity: identity(testMac),
		}
		result, err := fetchState(ctx, c, ARPortsSourceUniversal)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(result, ethernet) {
			t.Errorf("result = %v", result)
		}
		if len(calls) != 2 {
			t.Errorf("calls = %v, want 2", calls)
		}
	})
	t.Run("ReturnsEmptyWhenNothingAvailable", func(t *testing.T) {
		var calls []call
		c := &registry.Context{Read: reader(nil, &calls), Identity: identity(testMac)}
		result, err := fetchState(ctx, c, ARPortsSourceUniversal)
		if err != nil {
			t.Fatal(err)
		}
		if m, ok := result.(map[string]any); !ok || len(m) != 0 {
			t.Errorf("result = %v, want {}", result)
		}
	})
}

func TestTranslateState(t *testing.T) {
	tc := &registry.TranslateContext{Identity: identity(testMac)}
	t.Run("DispatchesPortStatus", func(t *testing.T) {
		result := translateState(portStatus, tc).(map[identifiers.MacAddress]ports.ARPortsData)
		var role any
		for _, p := range result[mustMac(t, testMac)].Ports {
			if p[ports.ARPortPropertyNativeName] == "W0" {
				role = p[ports.ARPortPropertyRole]
			}
		}
		if role != ports.ARPortTypeWan {
			t.Errorf("W0 role = %v, want WAN", role)
		}
	})
	t.Run("DispatchesEthernet", func(t *testing.T) {
		result := translateState(ethernet, tc).(map[identifiers.MacAddress]ports.ARPortsData)
		node := result[mustMac(t, testMac)]
		if len(node.Ports) != 1 || node.Ports[0][ports.ARPortPropertyNativeName] != "L1" {
			t.Errorf("ports = %v, want only L1", node.Ports)
		}
	})
	for _, tt := range []struct {
		name string
		data any
	}{
		{"ReturnsEmptyForUnusable/Empty", map[string]any{}},
		{"ReturnsEmptyForUnusable/UnknownShape", map[string]any{"unknown": 1}},
		{"ReturnsEmptyForUnusable/Str", "not-a-dict"},
		{"ReturnsEmptyForUnusable/None", nil},
	} {
		t.Run(tt.name, func(t *testing.T) {
			result := translateState(tt.data, tc).(map[identifiers.MacAddress]ports.ARPortsData)
			if len(result) != 0 {
				t.Errorf("result = %v, want {}", result)
			}
		})
	}
}
