package source

import (
	"context"
	"reflect"
	"testing"

	"github.com/eduardpaul/routerctl/asusrouter/modules/common"
	"github.com/eduardpaul/routerctl/asusrouter/modules/device"
	"github.com/eduardpaul/routerctl/asusrouter/modules/nvram"
	arsource "github.com/eduardpaul/routerctl/asusrouter/modules/source"
	"github.com/eduardpaul/routerctl/asusrouter/modules/traffic"
	trafficaimesh "github.com/eduardpaul/routerctl/asusrouter/modules/traffic/aimesh"
	trafficinterface "github.com/eduardpaul/routerctl/asusrouter/modules/traffic/interface"
	"github.com/eduardpaul/routerctl/asusrouter/modules/wifi"
	"github.com/eduardpaul/routerctl/asusrouter/registry"
)

const (
	routerMAC = "AA:BB:CC:00:00:01"
	remoteMAC = "AA:BB:CC:00:00:02"
)

type m = map[common.ARMetricType]any

func routerIdentity() *device.ARDeviceIdentity {
	return device.ARDeviceIdentityBuild(device.IdentityData{arsource.IDOf(nvram.ARNvramTypeMac): routerMAC})
}

// fakeMulti returns per-source-type content and records the requests.
func fakeMulti(aimesh, iface Metrics, requested *[]arsource.DataSource) registry.FetchDataMultiFunc {
	return func(_ context.Context, srcs []arsource.DataSource, _ registry.Kwargs) (map[arsource.ID]any, error) {
		*requested = append(*requested, srcs...)
		out := map[arsource.ID]any{}
		for _, src := range srcs {
			switch src.(type) {
			case *trafficinterface.ARTrafficInterfaceSource:
				if iface == nil {
					iface = Metrics{}
				}
				out[arsource.IDOf(src)] = iface
			case *trafficaimesh.ARTrafficAiMeshSource:
				if aimesh == nil {
					aimesh = Metrics{}
				}
				out[arsource.IDOf(src)] = aimesh
			}
		}
		return out, nil
	}
}

func hasType[T any](srcs []arsource.DataSource) bool {
	for _, s := range srcs {
		if _, ok := s.(T); ok {
			return true
		}
	}
	return false
}

func TestFetchState(t *testing.T) {
	ctx := context.Background()
	wired, wan, backhaul := traffic.ARTrafficTypeWired, traffic.ARTrafficTypeWan, traffic.ARTrafficTypeBackhaul
	band2, band5, band6 := wifi.ARWiFiBandBand2g1, wifi.ARWiFiBandBand5g1, wifi.ARWiFiBandBand6g1

	if got, _ := FetchState(ctx, &registry.Context{}, traffic.NewARTrafficSource(nil, routerMAC)); len(got.(Metrics)) != 0 {
		t.Errorf("without_callback: %v", got)
	}

	tests := []struct {
		name          string
		src           *traffic.ARTrafficSource
		aimesh, iface Metrics
		want          Metrics
		wantAimesh    bool
		wantInterface bool
	}{
		{
			"self_merges_both", traffic.NewARTrafficSource(nil, nil),
			Metrics{wired: {common.ARMetricTypeRxSpeed: 100.0}, band2: {common.ARMetricTypeRxSpeed: 50.0}},
			Metrics{wired: {common.ARMetricTypeRx: 10, common.ARMetricTypeRxSpeed: 999.0}, wan: {common.ARMetricTypeRx: 5, common.ARMetricTypeRxSpeed: 7.0}},
			Metrics{
				// AiMesh speed wins, interface counter kept
				wired: {common.ARMetricTypeRx: 10, common.ARMetricTypeRxSpeed: 100.0},
				// interface-only link
				wan: {common.ARMetricTypeRx: 5, common.ARMetricTypeRxSpeed: 7.0},
				// AiMesh-only link
				band2: {common.ARMetricTypeRxSpeed: 50.0},
			},
			true, true,
		},
		{
			"self_by_mac", traffic.NewARTrafficSource(wan, routerMAC),
			nil, Metrics{wan: {common.ARMetricTypeRx: 1}},
			Metrics{wan: {common.ARMetricTypeRx: 1}},
			false, true,
		},
		{
			"remote_aimesh_only", traffic.NewARTrafficSource(nil, remoteMAC),
			Metrics{band5: {common.ARMetricTypeRxSpeed: 12.0}}, Metrics{wan: {common.ARMetricTypeRx: 1}},
			Metrics{band5: {common.ARMetricTypeRxSpeed: 12.0}},
			true, false,
		},
		{
			"interface_only_link_skips_aimesh", traffic.NewARTrafficSource(wan, routerMAC),
			nil, Metrics{wan: {common.ARMetricTypeRx: 5}, wired: {common.ARMetricTypeRx: 9}},
			Metrics{wan: {common.ARMetricTypeRx: 5}},
			false, true,
		},
		{
			"interface_only_link_remote_empty", traffic.NewARTrafficSource(wan, remoteMAC),
			nil, Metrics{wan: {common.ARMetricTypeRx: 5}},
			Metrics{},
			false, false,
		},
		{
			"backhaul_aimesh_only", traffic.NewARTrafficSource(backhaul, routerMAC),
			Metrics{backhaul: {common.ARMetricTypeRxSpeed: 80.0}}, Metrics{wan: {common.ARMetricTypeRx: 1}},
			Metrics{backhaul: {common.ARMetricTypeRxSpeed: 80.0}},
			true, false,
		},
		{
			"overlap_link_filters_interface", traffic.NewARTrafficSource(wired, routerMAC),
			Metrics{wired: {common.ARMetricTypeRxSpeed: 100.0}},
			Metrics{wired: {common.ARMetricTypeRx: 10, common.ARMetricTypeRxSpeed: 999.0}, wan: {common.ARMetricTypeRx: 5}},
			Metrics{wired: {common.ARMetricTypeRx: 10, common.ARMetricTypeRxSpeed: 100.0}},
			true, true,
		},
		{
			"overlap_link_absent_in_interface", traffic.NewARTrafficSource(band6, routerMAC),
			Metrics{band6: {common.ARMetricTypeRxSpeed: 5.0}}, Metrics{wan: {common.ARMetricTypeRx: 1}},
			Metrics{band6: {common.ARMetricTypeRxSpeed: 5.0}},
			true, true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var requested []arsource.DataSource
			c := &registry.Context{Identity: routerIdentity(), FetchDataMulti: fakeMulti(tt.aimesh, tt.iface, &requested)}
			got, err := FetchState(ctx, c, tt.src)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %v, want %v", got, tt.want)
			}
			if a := hasType[*trafficaimesh.ARTrafficAiMeshSource](requested); a != tt.wantAimesh {
				t.Errorf("aimesh requested %v", a)
			}
			if i := hasType[*trafficinterface.ARTrafficInterfaceSource](requested); i != tt.wantInterface {
				t.Errorf("interface requested %v", i)
			}
		})
	}

	// Non-dict fetch results collapse to an empty state
	for name, value := range map[string]map[arsource.ID]any{
		"not_dict":         nil,
		"non_dict_content": {"": nil},
	} {
		t.Run("bad_fetch_results_"+name, func(t *testing.T) {
			c := &registry.Context{
				Identity: routerIdentity(),
				FetchDataMulti: func(context.Context, []arsource.DataSource, registry.Kwargs) (map[arsource.ID]any, error) {
					return value, nil
				},
			}
			got, _ := FetchState(ctx, c, traffic.NewARTrafficSource(nil, remoteMAC))
			if len(got.(Metrics)) != 0 {
				t.Errorf("got %v", got)
			}
		})
	}
}

func TestRegistered(t *testing.T) {
	entry := registry.SourceByName("ARTrafficSource")
	if entry == nil || entry.Fetch == nil || entry.Translate != nil {
		t.Fatal("ARTrafficSource registers only a fetcher")
	}
}

var _ = m{}
