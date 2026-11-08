package ping

import (
	"context"
	"testing"

	"github.com/eduardpaul/routerctl/asusrouter/modules/common"
	"github.com/eduardpaul/routerctl/asusrouter/modules/endpoint"
	"github.com/eduardpaul/routerctl/asusrouter/modules/service"
	"github.com/eduardpaul/routerctl/asusrouter/registry"
)

func TestPingActionRegistered(t *testing.T) {
	if e := registry.Action(&ARPingAction{}); e == nil || e.Run == nil {
		t.Error("not registered")
	}
}

func TestRunAction(t *testing.T) {
	tests := []struct {
		name  string
		reply any
		want  bool
	}{
		{"success", map[string]any{common.StatusCodeKey: "success"}, true},
		{"non_success_code", map[string]any{common.StatusCodeKey: "error"}, false},
		{"non_dict_response", nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var calls []call
			got, err := RunAction(context.Background(), &registry.Context{Read: reader(&calls, tt.reply)}, &ARPingAction{})
			if err != nil {
				t.Fatal(err)
			}
			if got.(service.ARServiceResult).Success != tt.want {
				t.Errorf("got %+v", got)
			}
			if len(calls) != 1 || calls[0].endpoint != endpoint.AREndpointRunPing || calls[0].request != "" {
				t.Errorf("calls %v", calls)
			}
		})
	}
}
