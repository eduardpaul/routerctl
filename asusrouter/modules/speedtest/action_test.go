package speedtest

import (
	"context"
	"strings"
	"testing"

	"github.com/eduardpaul/routerctl/asusrouter/modules/action"
	"github.com/eduardpaul/routerctl/asusrouter/modules/endpoint"
	"github.com/eduardpaul/routerctl/asusrouter/modules/service"
	"github.com/eduardpaul/routerctl/asusrouter/registry"
)

type call struct {
	endpoint endpoint.AREndpoint
	request  string
}

func recordRead(calls *[]call) registry.ReadFunc {
	return func(_ context.Context, ep endpoint.AREndpoint, request string) (any, error) {
		*calls = append(*calls, call{ep, request})
		return map[string]any{}, nil
	}
}

func run(t *testing.T, c *registry.Context, a *ARSpeedTestAction) service.ARServiceResult {
	t.Helper()
	got, err := RunAction(context.Background(), c, a)
	if err != nil {
		t.Fatal(err)
	}
	return got.(service.ARServiceResult)
}

func TestRunAction(t *testing.T) {
	t.Run("default", func(t *testing.T) {
		var calls []call
		if !run(t, &registry.Context{Read: recordRead(&calls)}, NewARSpeedTestAction(nil, nil)).Success {
			t.Error("not successful")
		}
		// First the start time, then the run trigger
		if len(calls) != 2 ||
			calls[0].endpoint != endpoint.AREndpointSetSpeedtestStartTime || !strings.HasPrefix(calls[0].request, "ookla_start_time=") ||
			calls[1].endpoint != endpoint.AREndpointRunSpeedtest || calls[1].request != "type=&id=" {
			t.Errorf("calls %v", calls)
		}
	})

	t.Run("with_server_and_iface", func(t *testing.T) {
		var calls []call
		run(t, &registry.Context{Read: recordRead(&calls)}, NewARSpeedTestAction("42", sp("tun11")))
		if calls[1].request != "type=&id=42&iface=tun11" {
			t.Errorf("request %q", calls[1].request)
		}
	})

	t.Run("with_integer_server", func(t *testing.T) {
		var calls []call
		run(t, &registry.Context{Read: recordRead(&calls)}, NewARSpeedTestAction(54112, nil))
		if calls[1].request != "type=&id=54112" {
			t.Errorf("request %q", calls[1].request)
		}
	})

	t.Run("prefers_raw_callback", func(t *testing.T) {
		var calls []call
		raws := 0
		empty := ""
		c := &registry.Context{
			Read: recordRead(&calls),
			FetchRaw: func(context.Context, endpoint.AREndpoint, string) (*string, error) {
				raws++
				return &empty, nil
			},
		}
		if !run(t, c, NewARSpeedTestAction(nil, nil)).Success || len(calls) != 0 || raws != 2 {
			t.Errorf("calls %v, raws %d", calls, raws)
		}
	})

	t.Run("failed_run_returns_false", func(t *testing.T) {
		c := &registry.Context{
			FetchRaw: func(context.Context, endpoint.AREndpoint, string) (*string, error) { return nil, nil },
		}
		if run(t, c, NewARSpeedTestAction(nil, nil)).Success {
			t.Error("success on a failed run")
		}
	})

	if entry := registry.Action(&ARSpeedTestAction{}); entry == nil || entry.Run == nil {
		t.Error("not registered")
	}
}

func TestAction(t *testing.T) {
	if id := NewARSpeedTestAction(54112, nil).ServerID; id == nil || *id != "54112" {
		t.Error("normalizes_integer_id")
	}
	if NewARSpeedTestAction(nil, nil).ServerID != nil {
		t.Error("default_id_is_none")
	}
	for _, value := range []any{0, -5, "0", "abc", ""} {
		if NewARSpeedTestAction(value, nil).ServerID != nil {
			t.Errorf("%v falls back to auto", value)
		}
	}
	if action.Repr(NewARSpeedTestAction(54112, nil)) != action.Repr(NewARSpeedTestAction("54112", nil)) {
		t.Error("equal_by_server_and_iface")
	}
	if action.Repr(NewARSpeedTestAction(1, nil)) == action.Repr(NewARSpeedTestAction(1, sp("tun11"))) {
		t.Error("not_equal_on_iface")
	}
}
