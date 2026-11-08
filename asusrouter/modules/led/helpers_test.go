package led

import (
	"context"

	"github.com/eduardpaul/routerctl/asusrouter/modules/aimesh"
	aimeshsource "github.com/eduardpaul/routerctl/asusrouter/modules/aimesh/source"
	"github.com/eduardpaul/routerctl/asusrouter/modules/device"
	"github.com/eduardpaul/routerctl/asusrouter/modules/firmware"
	"github.com/eduardpaul/routerctl/asusrouter/modules/nvram"
	"github.com/eduardpaul/routerctl/asusrouter/modules/source"
	"github.com/eduardpaul/routerctl/asusrouter/registry"
	"github.com/eduardpaul/routerctl/asusrouter/tools/identifiers"
)

const testMac = "c8:7f:54:00:00:01"

func mustMac(s string) identifiers.MacAddress {
	m := identifiers.MacAddressFromValueSafe(s)
	if m == nil {
		panic(s)
	}
	return *m
}

// testIdentity builds an identity of the test device (Python mocks one
// with `mac`), on the given firmware type.
func testIdentity(fw firmware.ARFirmwareType) *device.ARDeviceIdentity {
	data := device.IdentityData{source.IDOf(nvram.ARNvramTypeMac): testMac}
	switch fw {
	case firmware.ARFirmwareTypeMerlin:
		data[source.IDOf(nvram.ARNvramTypeFwMajor)] = "3.0.0.6"
		data[source.IDOf(nvram.ARNvramTypeFwMinor)] = "388"
		data[source.IDOf(nvram.ARNvramTypeFwBuild)] = "8_beta2"
	case firmware.ARFirmwareTypeStock:
		data[source.IDOf(nvram.ARNvramTypeFwMajor)] = "3.0.0.4"
		data[source.IDOf(nvram.ARNvramTypeFwMinor)] = "388"
		data[source.IDOf(nvram.ARNvramTypeFwBuild)] = "24911-g2055148"
	}
	identity := device.ARDeviceIdentityBuild(data)
	if fw != "" && identity.Firmware().FirmwareType() != fw {
		panic("firmware type " + string(identity.Firmware().FirmwareType()))
	}
	return identity
}

func topologyWith(led *bool) *aimesh.ARAiMeshTopology {
	topology := aimesh.NewARAiMeshTopology()
	topology.Add(&aimesh.ARAiMeshNode{Mac: mustMac(testMac), Led: led})
	return topology
}

// nvramReply is the LED nvram the data callback serves
func nvramReply() map[source.ID]any {
	return map[source.ID]any{source.IDOf(nvram.ARNvramTypeLed): "0"}
}

// dataCalls counts the data-callback calls of a test context
type dataCalls struct {
	multi    int
	single   int
	requests [][]source.DataSource
}

// contextWith builds a context whose data callbacks serve nvram and the
// given AiMesh content (Python `_get_data`).
func contextWith(nvramValues map[source.ID]any, aimeshContent any, identity *device.ARDeviceIdentity) (*registry.Context, *dataCalls) {
	calls := &dataCalls{}
	c := &registry.Context{
		Identity: identity,
		FetchDataMulti: func(_ context.Context, srcs []source.DataSource, _ registry.Kwargs) (map[source.ID]any, error) {
			calls.multi++
			calls.requests = append(calls.requests, srcs)
			return nvramValues, nil
		},
		FetchData: func(_ context.Context, src source.DataSource, _ registry.Kwargs) (any, error) {
			calls.single++
			if src == aimeshsource.ARAiMeshSourceUniversal {
				return aimeshContent, nil
			}
			return nil, nil
		},
	}
	return c, calls
}
