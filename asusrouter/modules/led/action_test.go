package led

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/eduardpaul/routerctl/asusrouter/modules/action"
	"github.com/eduardpaul/routerctl/asusrouter/modules/endpoint"
	"github.com/eduardpaul/routerctl/asusrouter/modules/firmware"
	"github.com/eduardpaul/routerctl/asusrouter/modules/nvram"
	"github.com/eduardpaul/routerctl/asusrouter/modules/service"
	"github.com/eduardpaul/routerctl/asusrouter/modules/source"
	"github.com/eduardpaul/routerctl/asusrouter/registry"
)

// Ported from tests/modules/led/test_action.py.

// Poll without sleeping (the Python autouse fixture)
func init() { pollInterval = 0 }

// poster is a push callback reporting the given service ran; it keeps
// the last request.
type poster struct {
	service string
	request string
	calls   int
}

func (p *poster) read(_ context.Context, ep endpoint.AREndpoint, request string) (any, error) {
	p.calls++
	if ep != endpoint.AREndpointPushData {
		return nil, fmt.Errorf("endpoint %q", ep)
	}
	p.request = request
	return map[string]any{"run_service": p.service}, nil
}

func (p *poster) payload(t *testing.T) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal([]byte(p.request), &out); err != nil {
		t.Fatalf("payload %q: %v", p.request, err)
	}
	return out
}

// dataStates is a get-data callback serving the given LED states in turn
// (Python `_data`); a nil state serves a non-dict.
type dataStates struct {
	states  []any
	calls   int
	sources []source.DataSource
}

func (d *dataStates) fetch(_ context.Context, src source.DataSource, _ registry.Kwargs) (any, error) {
	d.sources = append(d.sources, src)
	state := d.states[min(d.calls, len(d.states)-1)]
	d.calls++
	if state == "non-dict" {
		return nil, nil
	}
	return map[ARLedField]any{ARLedFieldState: state}, nil
}

type expirer struct{ expired []source.DataSource }

func (e *expirer) expire(_ context.Context, src source.DataSource) {
	e.expired = append(e.expired, src)
}

func run(t *testing.T, c *registry.Context, state bool) service.ARServiceResult {
	t.Helper()
	result, err := runAction(context.Background(), c, &ARLedAction{State: state})
	if err != nil {
		t.Fatal(err)
	}
	return result.(service.ARServiceResult)
}

func TestRunAction(t *testing.T) {
	for _, tc := range []struct {
		state    bool
		expected float64
	}{{true, 1}, {false, 0}} {
		t.Run(fmt.Sprintf("sets_state_%v", tc.state), func(t *testing.T) {
			p := &poster{service: "start_ctrl_led"}
			result := run(t, &registry.Context{Read: p.read}, tc.state)
			payload := p.payload(t)
			if !result.Success || payload["action_mode"] != "apply" || payload["rc_service"] != "start_ctrl_led" ||
				payload[string(nvram.ARNvramTypeLed)] != tc.expected {
				t.Errorf("result %+v, payload %v", result, payload)
			}
		})
	}

	t.Run("waits_for_settle", func(t *testing.T) {
		p := &poster{service: "start_ctrl_led"}
		// Blank first (daemon restarting), then a real value
		data := &dataStates{states: []any{nil, true}}
		exp := &expirer{}
		result := run(t, &registry.Context{Read: p.read, FetchData: data.fetch, Expire: exp.expire}, true)
		if !result.Success || data.calls != 2 || data.sources[1] != ARLedSourceUniversal || len(exp.expired) != 0 {
			t.Errorf("result %+v, %d polls, expired %v", result, data.calls, exp.expired)
		}
	})

	t.Run("settle_waits_for_new_state", func(t *testing.T) {
		p := &poster{service: "start_ctrl_led"}
		data := &dataStates{states: []any{false, true}}
		if result := run(t, &registry.Context{Read: p.read, FetchData: data.fetch}, true); !result.Success || data.calls != 2 {
			t.Errorf("result %+v, %d polls", result, data.calls)
		}
	})

	t.Run("settle_ignores_non_dict_poll", func(t *testing.T) {
		p := &poster{service: "start_ctrl_led"}
		data := &dataStates{states: []any{"non-dict", true}}
		if result := run(t, &registry.Context{Read: p.read, FetchData: data.fetch}, true); !result.Success || data.calls != 2 {
			t.Errorf("result %+v, %d polls", result, data.calls)
		}
	})

	t.Run("settle_timeout_still_succeeds", func(t *testing.T) {
		p := &poster{service: "start_ctrl_led"}
		data := &dataStates{states: []any{nil}}
		if result := run(t, &registry.Context{Read: p.read, FetchData: data.fetch}, true); !result.Success || data.calls != pollAttempts {
			t.Errorf("result %+v, %d polls", result, data.calls)
		}
	})

	t.Run("expires_without_data_callback", func(t *testing.T) {
		p := &poster{service: "start_ctrl_led"}
		exp := &expirer{}
		result := run(t, &registry.Context{Read: p.read, Expire: exp.expire}, true)
		seen := map[source.ID]bool{}
		for _, src := range exp.expired {
			seen[source.IDOf(src)] = true
		}
		if !result.Success || !seen[source.IDOf(ARLedSourceUniversal)] || !seen[source.IDOf(nvram.ARNvramTypeLed)] {
			t.Errorf("result %+v, expired %v", result, exp.expired)
		}
	})

	t.Run("no_effect_on_failure", func(t *testing.T) {
		p := &poster{service: "other"}
		data := &dataStates{states: []any{true}}
		exp := &expirer{}
		result := run(t, &registry.Context{Read: p.read, FetchData: data.fetch, Expire: exp.expire}, true)
		if result.Success || data.calls != 0 || len(exp.expired) != 0 {
			t.Errorf("result %+v, %d polls, expired %v", result, data.calls, exp.expired)
		}
	})
}

// recorder records the actions a recovery runs.
type recorder struct{ states []bool }

func (r *recorder) run(_ context.Context, a action.ARAction) (any, error) {
	r.states = append(r.states, a.(*ARLedAction).State)
	return nil, nil
}

func boolPtr(v bool) *bool { return &v }

func TestRecoverState(t *testing.T) {
	t.Run("merlin_off_toggles", func(t *testing.T) {
		r := &recorder{}
		recovered, err := AsyncRecoverState(context.Background(), r.run, boolPtr(false), testIdentity(firmware.ARFirmwareTypeMerlin))
		if err != nil || !recovered || fmt.Sprint(r.states) != "[true false]" {
			t.Errorf("recovered %v, states %v", recovered, r.states)
		}
	})

	for _, tc := range []struct {
		name    string
		desired *bool
		fw      firmware.ARFirmwareType
	}{
		{"stock_off", boolPtr(false), firmware.ARFirmwareTypeStock}, // not Merlin: boot keeps the state
		{"merlin_on", boolPtr(true), firmware.ARFirmwareTypeMerlin}, // on-state survives a reboot
		{"merlin_none", nil, firmware.ARFirmwareTypeMerlin},         // never commanded
	} {
		t.Run("no_recovery_"+tc.name, func(t *testing.T) {
			r := &recorder{}
			recovered, _ := AsyncRecoverState(context.Background(), r.run, tc.desired, testIdentity(tc.fw))
			if recovered || len(r.states) != 0 {
				t.Errorf("recovered %v, states %v", recovered, r.states)
			}
		})
	}

	t.Run("no_identity", func(t *testing.T) {
		r := &recorder{}
		recovered, _ := AsyncRecoverState(context.Background(), r.run, boolPtr(false), nil)
		if recovered || len(r.states) != 0 {
			t.Errorf("recovered %v, states %v", recovered, r.states)
		}
	})
}

func TestEquality(t *testing.T) {
	key := func(a *ARLedAction) string { return action.Repr(a) }
	if key(&ARLedAction{State: true}) != key(&ARLedAction{State: true}) ||
		key(&ARLedAction{State: true}) == key(&ARLedAction{State: false}) {
		t.Error("actions are not keyed by state")
	}
	if registry.Action(&ARLedAction{}) == nil {
		t.Error("not registered")
	}
}
