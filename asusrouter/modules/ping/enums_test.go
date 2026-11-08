package ping

import (
	"testing"

	"github.com/eduardpaul/routerctl/asusrouter/consts"
	"github.com/eduardpaul/routerctl/asusrouter/modules/endpoint"
)

func TestEnums(t *testing.T) {
	for _, m := range ARPingStatusMembers {
		if ARPingStatusFromValue(string(m)) != m {
			t.Errorf("%s", m.Name())
		}
	}
	if ARPingStatusFromValue("3") != ARPingStatusFinished || ARPingStatusFromValue("1") != ARPingStatusUnknown {
		t.Error("status codes")
	}
}

// test_init.py: the ping trigger is a GET
func TestRunPingIsGet(t *testing.T) {
	if endpoint.GetEndpointRequestType(endpoint.AREndpointRunPing) != consts.RequestTypeGet {
		t.Error("RUN_PING is not GET")
	}
}
