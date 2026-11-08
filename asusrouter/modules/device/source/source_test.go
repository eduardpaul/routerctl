package source

import (
	"context"
	"reflect"
	"testing"

	"github.com/eduardpaul/routerctl/asusrouter/modules/device"
	"github.com/eduardpaul/routerctl/asusrouter/modules/nvram"
	"github.com/eduardpaul/routerctl/asusrouter/modules/source"
	"github.com/eduardpaul/routerctl/asusrouter/registry"
)

// TestCallsGetDataCallbackWithDeviceRequest: fetchState delegates to
// fetch_data_callback with DEVICE_REQUEST.
func TestCallsGetDataCallbackWithDeviceRequest(t *testing.T) {
	expected := map[source.ID]any{"key": "value"}
	var calls [][]source.DataSource
	c := &registry.Context{
		FetchDataMulti: func(ctx context.Context, srcs []source.DataSource, kw registry.Kwargs) (map[source.ID]any, error) {
			calls = append(calls, srcs)
			return expected, nil
		},
	}
	result, err := fetchState(context.Background(), c, ARDeviceSourceUniversal)
	if err != nil {
		t.Fatal(err)
	}
	if len(calls) != 1 || !reflect.DeepEqual(calls[0], DeviceRequest) {
		t.Errorf("calls = %v", calls)
	}
	if !reflect.DeepEqual(result, device.IdentityData(expected)) {
		t.Errorf("result = %v", result)
	}
}

// TestIgnoresCallbackAndSource: fetchState ignores the callback and source
// arguments.
func TestIgnoresCallbackAndSource(t *testing.T) {
	n := 0
	c := &registry.Context{
		FetchDataMulti: func(ctx context.Context, srcs []source.DataSource, kw registry.Kwargs) (map[source.ID]any, error) {
			n++
			if !reflect.DeepEqual(srcs, DeviceRequest) {
				t.Errorf("srcs = %v", srcs)
			}
			return nil, nil
		},
	}
	if _, err := fetchState(context.Background(), c, nil); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("called %d times", n)
	}
}

// TestDeviceRequest mirrors the DEVICE_REQUEST content and order.
func TestDeviceRequest(t *testing.T) {
	if len(DeviceRequest) != 20 {
		t.Fatalf("len = %d", len(DeviceRequest))
	}
	if DeviceRequest[0] != nvram.ARNvramTypeMac || DeviceRequest[19].SourceName() != "ARSupportSource" {
		t.Errorf("unexpected order: %v", DeviceRequest)
	}
	if got := source.IDOf(DeviceRequest[8]); got != source.IDOf(nvram.NewARNvramIndexSource(nvram.ARNvramIndexTypeWlNband, 0)) {
		t.Errorf("index source = %v", got)
	}
}

// TestReturnsDeviceIdentity: translateState returns an ARDeviceIdentity.
func TestReturnsDeviceIdentity(t *testing.T) {
	for _, raw := range []any{device.IdentityData{}, map[source.ID]any{}, nil} {
		if _, ok := translateState(raw, nil).(*device.ARDeviceIdentity); !ok {
			t.Errorf("%T: not an identity", raw)
		}
	}
}

// TestPassesDataToBuild: translateState builds the identity from the data.
func TestPassesDataToBuild(t *testing.T) {
	data := device.IdentityData{source.IDOf(nvram.ARNvramTypeModel): "RT-AX88U"}
	identity := translateState(data, &registry.TranslateContext{Kwargs: registry.Kwargs{"extra_arg": "ignored"}}).(*device.ARDeviceIdentity)
	if m := identity.Model(); m == nil || *m != "RT-AX88U" {
		t.Errorf("model = %v", m)
	}
}

// TestRegistered: the device source is registered with its callables.
func TestRegistered(t *testing.T) {
	e := registry.SourceByName("ARDeviceSource")
	if e == nil || e.Fetch == nil || e.Translate == nil || e.Multi() {
		t.Fatalf("entry = %+v", e)
	}
	if _, ok := e.Default().(*ARDeviceSource); !ok {
		t.Error("default is not an ARDeviceSource")
	}
}
