package ddns

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/eduardpaul/routerctl/asusrouter/modules/action"
	"github.com/eduardpaul/routerctl/asusrouter/modules/endpoint"
	"github.com/eduardpaul/routerctl/asusrouter/modules/service"
	"github.com/eduardpaul/routerctl/asusrouter/modules/source"
	"github.com/eduardpaul/routerctl/asusrouter/registry"
	"github.com/eduardpaul/routerctl/asusrouter/tools/identifiers"
)

func init() { pollInterval = time.Millisecond }

const regResultKey = "asusddns_reg_result"

type call struct {
	endpoint endpoint.AREndpoint
	request  string
}

// poster answers every read with a service run echo.
func poster(calls *[]call, service string) registry.ReadFunc {
	return func(_ context.Context, ep endpoint.AREndpoint, request string) (any, error) {
		*calls = append(*calls, call{ep, request})
		return map[string]any{"run_service": service}, nil
	}
}

func data(current any) registry.FetchDataFunc {
	return func(context.Context, source.DataSource, registry.Kwargs) (any, error) { return current, nil }
}

func payload(t *testing.T, calls []call) map[string]any {
	t.Helper()
	last := calls[len(calls)-1]
	if last.endpoint != endpoint.AREndpointPushData {
		t.Fatalf("endpoint %s", last.endpoint)
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(last.request), &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func deregisterRead(calls *[]call, polls []any, dispatch, clean any) registry.ReadFunc {
	return func(_ context.Context, ep endpoint.AREndpoint, request string) (any, error) {
		*calls = append(*calls, call{ep, request})
		switch ep {
		case endpoint.AREndpointDdnsUnregister:
			return dispatch, nil
		case endpoint.AREndpointDdnsClean:
			return clean, nil
		}
		entry := polls[0]
		polls = polls[1:]
		if s, ok := entry.(string); ok {
			return map[string]any{regResultKey: s}, nil
		}
		return entry, nil
	}
}

func run(t *testing.T, c *registry.Context, a *ARDdnsAction) bool {
	t.Helper()
	got, err := RunAction(context.Background(), c, a)
	if err != nil {
		t.Fatal(err)
	}
	return got.(service.ARServiceResult).Success
}

func bp(b bool) *bool { return &b }

func TestState(t *testing.T) {
	for state, want := range map[bool]float64{true: 1, false: 0} {
		var calls []call
		if !run(t, &registry.Context{Read: poster(&calls, "restart_ddns")}, &ARDdnsAction{Command: ARDdnsCommandState, State: bp(state)}) {
			t.Error("toggle failed")
		}
		p := payload(t, calls)
		if p["action_mode"] != "apply" || p["rc_service"] != "restart_ddns" || p["ddns_enable_x"] != want {
			t.Errorf("payload %v", p)
		}
	}

	var calls []call
	if run(t, &registry.Context{Read: poster(&calls, "restart_ddns")}, &ARDdnsAction{Command: ARDdnsCommandState}) || len(calls) != 0 {
		t.Error("no_state")
	}
}

func TestSet(t *testing.T) {
	t.Run("full_config", func(t *testing.T) {
		var calls []call
		a := &ARDdnsAction{Command: ARDdnsCommandSet, State: bp(true), Config: ARDdnsConfig{
			ARDdnsFieldServer:             ARDdnsServerDyndns,
			ARDdnsFieldHostname:           identifiers.NewHostname("myhost.dyndns.org"),
			ARDdnsFieldUsername:           "user",
			ARDdnsFieldPassword:           identifiers.NewPassword("secret"),
			ARDdnsFieldIpv6Update:         true,
			ARDdnsFieldWildcard:           false,
			ARDdnsFieldVerification:       true,
			ARDdnsFieldVerificationPeriod: 60,
			ARDdnsFieldRefreshInterval:    21,
			ARDdnsFieldWanUnit:            -1,
		}}
		if !run(t, &registry.Context{Read: poster(&calls, "restart_ddns")}, a) {
			t.Fatal("failed")
		}
		p := payload(t, calls)
		want := map[string]any{
			"rc_service": "restart_ddns", "ddns_enable_x": 1.0, "ddns_server_x": "WWW.DYNDNS.ORG",
			"ddns_hostname_x": "myhost.dyndns.org", "ddns_username_x": "user", "ddns_passwd_x": "secret",
			"ddns_ipv6_update": 1.0, "ddns_wildcard_x": 0.0, "ddns_regular_check": 1.0,
			"ddns_regular_period": 60.0, "ddns_refresh_x": 21.0, "ddns_wan_unit": -1.0,
		}
		for k, v := range want {
			if p[k] != v {
				t.Errorf("%s: %v, want %v", k, p[k], v)
			}
		}
	})

	t.Run("empty", func(t *testing.T) {
		var calls []call
		if run(t, &registry.Context{Read: poster(&calls, "restart_ddns")}, &ARDdnsAction{Command: ARDdnsCommandSet}) || len(calls) != 0 {
			t.Error("succeeded")
		}
	})

	t.Run("none_clears", func(t *testing.T) {
		var calls []call
		if !run(t, &registry.Context{Read: poster(&calls, "restart_ddns")}, &ARDdnsAction{Command: ARDdnsCommandSet, Config: ARDdnsConfig{ARDdnsFieldUsername: nil}}) {
			t.Fatal("failed")
		}
		if p := payload(t, calls); p["ddns_username_x"] != "" {
			t.Errorf("payload %v", p)
		}
	})

	for _, config := range []ARDdnsConfig{
		{ARDdnsFieldServer: "NOT.A.SERVER"},
		{ARDdnsFieldIpv6Update: "maybe"},
		{ARDdnsFieldVerificationPeriod: "soon"},
	} {
		var calls []call
		if run(t, &registry.Context{Read: poster(&calls, "restart_ddns")}, &ARDdnsAction{Command: ARDdnsCommandSet, Config: config}) || len(calls) != 0 {
			t.Errorf("bad_value %v succeeded", config)
		}
	}

	t.Run("hostname_change_unbinds", func(t *testing.T) {
		var calls []call
		c := &registry.Context{Read: poster(&calls, "restart_ddns"), FetchData: data(ARDdnsConfig{ARDdnsFieldHostname: identifiers.NewHostname("old.example.com")})}
		run(t, c, &ARDdnsAction{Command: ARDdnsCommandSet, Config: ARDdnsConfig{ARDdnsFieldHostname: "new.example.com"}})
		if p := payload(t, calls); p["ddns_replace_status"] != 0.0 {
			t.Errorf("payload %v", p)
		}
	})

	t.Run("hostname_unchanged", func(t *testing.T) {
		var calls []call
		c := &registry.Context{Read: poster(&calls, "restart_ddns"), FetchData: data(ARDdnsConfig{ARDdnsFieldHostname: identifiers.NewHostname("same.example.com")})}
		run(t, c, &ARDdnsAction{Command: ARDdnsCommandSet, Config: ARDdnsConfig{ARDdnsFieldHostname: "same.example.com"}})
		if _, ok := payload(t, calls)["ddns_replace_status"]; ok {
			t.Error("unbinds an unchanged hostname")
		}
	})

	for name, fetch := range map[string]registry.FetchDataFunc{"no_callback": nil, "none": data(nil), "none_content": data(ARDdnsConfig(nil))} {
		t.Run("hostname_current_unknown_"+name, func(t *testing.T) {
			var calls []call
			run(t, &registry.Context{Read: poster(&calls, "restart_ddns"), FetchData: fetch}, &ARDdnsAction{Command: ARDdnsCommandSet, Config: ARDdnsConfig{ARDdnsFieldHostname: "new.example.com"}})
			if _, ok := payload(t, calls)["ddns_replace_status"]; ok {
				t.Error("unbinds without a known hostname")
			}
		})
	}
}

func TestUpdate(t *testing.T) {
	var calls []call
	if !run(t, &registry.Context{Read: poster(&calls, "ddnsclient")}, &ARDdnsAction{Command: ARDdnsCommandUpdate}) {
		t.Fatal("failed")
	}
	if p := payload(t, calls); p["action_mode"] != "Update" || p["rc_service"] != "ddnsclient" {
		t.Errorf("payload %v", p)
	}
}

func endpoints(calls []call) []endpoint.AREndpoint {
	out := make([]endpoint.AREndpoint, len(calls))
	for i, c := range calls {
		out[i] = c.endpoint
	}
	return out
}

func TestDeregister(t *testing.T) {
	deregister := &ARDdnsAction{Command: ARDdnsCommandDeregister}

	t.Run("success", func(t *testing.T) {
		var calls []call
		if !run(t, &registry.Context{Read: deregisterRead(&calls, []any{"unregister,200"}, "", "")}, deregister) {
			t.Fatal("failed")
		}
		want := []endpoint.AREndpoint{endpoint.AREndpointDdnsUnregister, endpoint.AREndpointFetchData, endpoint.AREndpointDdnsClean}
		if got := endpoints(calls); len(got) != 3 || got[0] != want[0] || got[1] != want[1] || got[2] != want[2] {
			t.Errorf("endpoints %v", got)
		}
	})

	for _, first := range []any{"register,200", nil} {
		var calls []call
		if !run(t, &registry.Context{Read: deregisterRead(&calls, []any{first, "unregister,200"}, "", "")}, deregister) {
			t.Errorf("polls_until_ready %v: failed", first)
		}
	}

	t.Run("token_bound", func(t *testing.T) {
		var calls []call
		c := &registry.Context{Read: deregisterRead(&calls, []any{"unregister,200"}, "", ""), FetchData: data(ARDdnsConfig{ARDdnsFieldTokenState: true})}
		if run(t, c, deregister) || len(calls) != 0 {
			t.Error("deregistered an app-bound hostname")
		}
	})

	polls := make([]any, 6)
	for i := range polls {
		polls[i] = "register,200"
	}
	failures := map[string]registry.ReadFunc{}
	var calls []call
	failures["dispatch_failed"] = deregisterRead(&calls, []any{"unregister,200"}, nil, "")
	failures["poll_timeout"] = deregisterRead(&calls, polls, "", "")
	failures["cleanup_failed"] = deregisterRead(&calls, []any{"unregister,200"}, "", nil)
	failures["dispatch_empty_dict_fails"] = deregisterRead(&calls, []any{"unregister,200"}, map[string]any{}, "")
	failures["cleanup_empty_dict_fails"] = deregisterRead(&calls, []any{"unregister,200"}, "", map[string]any{})
	for name, read := range failures {
		if run(t, &registry.Context{Read: read}, deregister) {
			t.Errorf("%s: succeeded", name)
		}
	}

	t.Run("rejected_by_server", func(t *testing.T) {
		var calls []call
		if run(t, &registry.Context{Read: deregisterRead(&calls, []any{"unregister,402"}, "", "")}, deregister) {
			t.Error("succeeded")
		}
		for _, ep := range endpoints(calls) {
			if ep == endpoint.AREndpointDdnsClean {
				t.Error("cleaned after a rejection")
			}
		}
	})
}

func captureDebug(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	orig := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(orig) })
	return &buf
}

func TestCommon(t *testing.T) {
	t.Run("unknown_command", func(t *testing.T) {
		var calls []call
		if run(t, &registry.Context{Read: poster(&calls, "restart_ddns")}, &ARDdnsAction{Command: ARDdnsCommandUnknown}) || len(calls) != 0 {
			t.Error("succeeded")
		}
	})

	t.Run("expire_on_success", func(t *testing.T) {
		var calls []call
		var expired []source.DataSource
		c := &registry.Context{Read: poster(&calls, "restart_ddns"), Expire: func(_ context.Context, src source.DataSource) { expired = append(expired, src) }}
		run(t, c, &ARDdnsAction{Command: ARDdnsCommandState, State: bp(true)})
		if len(expired) != 1+len(DdnsRequest) || expired[0] != source.DataSource(ARDdnsSourceUniversal) {
			t.Errorf("expired %v", expired)
		}
		for i, item := range DdnsRequest {
			if expired[i+1] != source.DataSource(item) {
				t.Errorf("expired %v", expired[i+1])
			}
		}
	})

	t.Run("no_expire_on_failure", func(t *testing.T) {
		var expired []source.DataSource
		c := &registry.Context{
			Read:   func(context.Context, endpoint.AREndpoint, string) (any, error) { return nil, nil },
			Expire: func(_ context.Context, src source.DataSource) { expired = append(expired, src) },
		}
		if run(t, c, &ARDdnsAction{Command: ARDdnsCommandState, State: bp(true)}) || len(expired) != 0 {
			t.Errorf("expired %v", expired)
		}
	})

	t.Run("raw_callback_preferred", func(t *testing.T) {
		var calls, raws []call
		c := &registry.Context{
			Read: poster(&calls, "restart_ddns"),
			FetchRaw: func(_ context.Context, ep endpoint.AREndpoint, request string) (*string, error) {
				raws = append(raws, call{ep, request})
				body := `{"run_service":"restart_ddns"}`
				return &body, nil
			},
		}
		if !run(t, c, &ARDdnsAction{Command: ARDdnsCommandState, State: bp(true)}) || len(calls) != 0 {
			t.Error("raw callback not preferred")
		}
		payload(t, raws)
	})

	t.Run("update_ignores_config", func(t *testing.T) {
		logs := captureDebug(t)
		var calls []call
		a := &ARDdnsAction{Command: ARDdnsCommandUpdate, State: bp(true), Config: ARDdnsConfig{ARDdnsFieldHostname: identifiers.NewHostname("host")}}
		if !run(t, &registry.Context{Read: poster(&calls, "ddnsclient")}, a) || !strings.Contains(logs.String(), "ignores state and config") {
			t.Errorf("logs %s", logs)
		}
	})

	t.Run("set_logs_non_writable_field", func(t *testing.T) {
		logs := captureDebug(t)
		var calls []call
		a := &ARDdnsAction{Command: ARDdnsCommandSet, Config: ARDdnsConfig{ARDdnsFieldHostname: identifiers.NewHostname("host"), ARDdnsFieldStatus: "200"}}
		if !run(t, &registry.Context{Read: poster(&calls, "restart_ddns")}, a) || !strings.Contains(logs.String(), "non-writable config fields") {
			t.Errorf("logs %s", logs)
		}
	})
}

func TestEquality(t *testing.T) {
	base := &ARDdnsAction{Command: ARDdnsCommandSet, State: bp(true)}
	same := &ARDdnsAction{Command: ARDdnsCommandSet, State: bp(true)}
	if action.Repr(base) != action.Repr(same) {
		t.Error("equal")
	}
	other := &ARDdnsAction{Command: ARDdnsCommandSet, State: bp(true), Config: ARDdnsConfig{ARDdnsFieldHostname: identifiers.NewHostname("host")}}
	if action.Repr(base) == action.Repr(other) {
		t.Error("key distinguishes config")
	}
}
