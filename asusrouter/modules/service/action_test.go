package service

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/eduardpaul/routerctl/asusrouter/modules/common"
	"github.com/eduardpaul/routerctl/asusrouter/modules/device"
	"github.com/eduardpaul/routerctl/asusrouter/modules/endpoint"
	"github.com/eduardpaul/routerctl/asusrouter/registry"
)

func intp(v int) *int { return &v }

// TestAsList mirrors TestAsList.
func TestAsList(t *testing.T) {
	cases := []struct {
		name     string
		services any
		want     []ARServiceInput
	}{
		{"enum", common.ARServiceWirelessRestart, []ARServiceInput{common.ARServiceWirelessRestart}},
		{"string", "restart_sdn 3", []ARServiceInput{"restart_sdn 3"}},
		{"list", []any{common.ARServiceWirelessRestart, "restart_sdn 3"}, []ARServiceInput{common.ARServiceWirelessRestart, "restart_sdn 3"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := asList(tc.services); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("asList = %#v, want %#v", got, tc.want)
			}
		})
	}
}

// TestBuildServiceRequest mirrors TestBuildServiceRequest (exact payloads).
func TestBuildServiceRequest(t *testing.T) {
	cases := []struct {
		name      string
		services  any
		arguments [][2]any
		want      string
	}{
		{"single_service", common.ARServiceWirelessRestart, nil,
			`{"action_mode":"apply","rc_service":"restart_wireless"}`},
		{"multiple_services", []any{common.ARServiceWirelessRestart, "restart_sdn 3"}, nil,
			`{"action_mode":"apply","rc_service":"restart_wireless;restart_sdn 3"}`},
		{"with_arguments", common.ARServiceWirelessRestart, [][2]any{{"wl0_radio", "0"}},
			`{"action_mode":"apply","rc_service":"restart_wireless","wl0_radio":"0"}`},
		// Python json.dumps: `<>;` kept as is (not validated, see PARITY.md),
		// non-ASCII escaped
		{"python_escaping", []any{"restart_wireless", "restart_sdn 3"}, [][2]any{{"wl0_radio", "0"}, {"x", "<a>;\u00e9"}},
			`{"action_mode":"apply","rc_service":"restart_wireless;restart_sdn 3","wl0_radio":"0","x":"<a>;\u00e9"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := BuildServiceRequest(tc.services, tc.arguments, common.ARActionModeApply); got != tc.want {
				t.Fatalf("BuildServiceRequest = %s, want %s", got, tc.want)
			}
		})
	}
}

// TestReadServiceResult mirrors TestReadServiceResult.
func TestReadServiceResult(t *testing.T) {
	cases := []struct {
		name     string
		result   any
		services any
		want     ARServiceResult
	}{
		{"echo_matches_requested", map[string]any{"run_service": "restart_wireless"}, common.ARServiceWirelessRestart,
			ARServiceResult{Success: true}},
		{"echo_matches_ignoring_trailing_semicolon", map[string]any{"run_service": "restart_wireless;restart_sdn 3;"},
			[]any{common.ARServiceWirelessRestart, "restart_sdn 3"}, ARServiceResult{Success: true}},
		{"echo_mismatch", map[string]any{"run_service": "restart_wireless"},
			[]any{common.ARServiceWirelessRestart, "restart_sdn 3"}, ARServiceResult{Success: false}},
		{"echo_without_services", map[string]any{"run_service": "restart_wireless"}, nil, ARServiceResult{Success: true}},
		{"empty_echo_without_services", map[string]any{"run_service": ""}, nil, ARServiceResult{Success: false}},
		{"modify_true", map[string]any{common.ModifyKey: "1"}, nil, ARServiceResult{Success: true}},
		{"modify_false", map[string]any{common.ModifyKey: "0"}, nil, ARServiceResult{Success: false}},
		{"needed_time_and_id", map[string]any{"run_service": "restart_wireless", "restart_needed_time": "8", "id": "2"},
			common.ARServiceWirelessRestart, ARServiceResult{Success: true, NeededTime: intp(8), LastID: intp(2)}},
		{"needed_time_defaults_when_id_present", map[string]any{"run_service": "restart_wireless", "id": "7"},
			common.ARServiceWirelessRestart, ARServiceResult{Success: true, NeededTime: intp(5), LastID: intp(7)}},
		{"no_id_no_time", map[string]any{"run_service": "restart_wireless"}, common.ARServiceWirelessRestart,
			ARServiceResult{Success: true}},
		{"none_response", nil, nil, ARServiceResult{Success: false}},
		{"nil_raw_response", (*string)(nil), nil, ARServiceResult{Success: false}},
		{"plain_text_body_is_dispatched", "NOT MODIFIED", common.ARServiceDnsRestart, ARServiceResult{Success: true}},
		{"empty_text_body", "", nil, ARServiceResult{Success: false}},
		{"blank_text_body", "   ", nil, ARServiceResult{Success: false}},
		{"json_string_body", `{"run_service":"restart_wireless"}`, common.ARServiceWirelessRestart, ARServiceResult{Success: true}},
		{"empty_json_dict_is_dispatched", map[string]any{}, common.ARServiceDnsRestart, ARServiceResult{Success: true}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ReadServiceResult(tc.result, tc.services); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("ReadServiceResult = %+v, want %+v", got, tc.want)
			}
		})
	}
}

// TestARServiceResultBool mirrors TestARServiceResult.
func TestARServiceResultBool(t *testing.T) {
	for _, success := range []bool{true, false} {
		if (ARServiceResult{Success: success}).Bool() != success {
			t.Fatalf("Bool() != %v", success)
		}
	}
}

// TestARServiceAction mirrors TestARServiceAction.
func TestARServiceAction(t *testing.T) {
	a := NewARServiceAction(common.ARServiceWirelessRestart, nil)
	if !reflect.DeepEqual(a.Services, []ARServiceInput{common.ARServiceWirelessRestart}) || len(a.Arguments) != 0 {
		t.Fatalf("single service: %+v", a)
	}

	a = NewARServiceAction([]any{common.ARServiceWirelessRestart, "restart_sdn 3"}, [][2]any{{"wl0_radio", "0"}})
	if !reflect.DeepEqual(a.Services, []ARServiceInput{common.ARServiceWirelessRestart, "restart_sdn 3"}) {
		t.Fatalf("services: %+v", a.Services)
	}
	if !reflect.DeepEqual(a.Arguments, [][2]any{{"wl0_radio", "0"}}) {
		t.Fatalf("arguments: %+v", a.Arguments)
	}

	if e := registry.Action(a); e == nil || e.Run == nil {
		t.Fatal("ARServiceAction is not registered")
	}
}

type call struct {
	endpoint endpoint.AREndpoint
	request  string
}

func rawPoster(body *string, calls *[]call) registry.FetchRawFunc {
	return func(_ context.Context, ep endpoint.AREndpoint, request string) (*string, error) {
		*calls = append(*calls, call{ep, request})
		return body, nil
	}
}

func readPoster(body any, calls *[]call) registry.ReadFunc {
	return func(_ context.Context, ep endpoint.AREndpoint, request string) (any, error) {
		*calls = append(*calls, call{ep, request})
		return body, nil
	}
}

func strp(s string) *string { return &s }

// TestRunAction mirrors TestRunAction.
func TestRunAction(t *testing.T) {
	ctx := context.Background()

	t.Run("success_via_raw_callback", func(t *testing.T) {
		var raw, read []call
		c := &registry.Context{
			Read:     readPoster(nil, &read),
			FetchRaw: rawPoster(strp("NOT MODIFIED"), &raw),
		}
		got, err := runAction(ctx, c, NewARServiceAction(common.ARServiceWirelessRestart, [][2]any{{"wl0_radio", "0"}}))
		if err != nil || !got.(ARServiceResult).Success {
			t.Fatalf("result = %+v, %v", got, err)
		}
		if len(read) != 0 || len(raw) != 1 {
			t.Fatalf("calls: read=%d raw=%d", len(read), len(raw))
		}
		if raw[0].endpoint != endpoint.AREndpointPushData {
			t.Fatalf("endpoint = %s", raw[0].endpoint)
		}
		want := `{"action_mode":"apply","rc_service":"restart_wireless","wl0_radio":"0"}`
		if raw[0].request != want {
			t.Fatalf("request = %s, want %s", raw[0].request, want)
		}
		for _, frag := range []string{`"rc_service":"restart_wireless"`, `"wl0_radio":"0"`} {
			if !strings.Contains(raw[0].request, frag) {
				t.Fatalf("request missing %s", frag)
			}
		}
	})

	t.Run("falls_back_to_callback", func(t *testing.T) {
		var read []call
		c := &registry.Context{Read: readPoster(map[string]any{"run_service": "restart_wireless"}, &read)}
		got, err := runAction(ctx, c, NewARServiceAction(common.ARServiceWirelessRestart, nil))
		if err != nil || !got.(ARServiceResult).Success {
			t.Fatalf("result = %+v, %v", got, err)
		}
		if len(read) != 1 || read[0].endpoint != endpoint.AREndpointPushData {
			t.Fatalf("calls = %+v", read)
		}
	})

	t.Run("no_services", func(t *testing.T) {
		var raw, read []call
		c := &registry.Context{Read: readPoster(nil, &read), FetchRaw: rawPoster(nil, &raw)}
		got, err := runAction(ctx, c, NewARServiceAction([]any{}, nil))
		if err != nil || got.(ARServiceResult).Success {
			t.Fatalf("result = %+v, %v", got, err)
		}
		if len(read)+len(raw) != 0 {
			t.Fatal("unexpected calls")
		}
	})

	rebootCases := []struct {
		name     string
		body     string
		service  common.ARService
		rebooted bool
	}{
		{"reboot_marks_identity", "NOT MODIFIED", common.ARServiceReboot, true},
		{"reboot_failure_does_not_mark", "", common.ARServiceReboot, false},
		{"non_reboot_does_not_mark", "NOT MODIFIED", common.ARServiceWirelessRestart, false},
	}
	for _, tc := range rebootCases {
		t.Run(tc.name, func(t *testing.T) {
			var raw []call
			identity := device.NewARDeviceIdentity()
			c := &registry.Context{FetchRaw: rawPoster(strp(tc.body), &raw), Identity: identity}
			if _, err := runAction(ctx, c, NewARServiceAction(tc.service, nil)); err != nil {
				t.Fatal(err)
			}
			if identity.Rebooted() != tc.rebooted {
				t.Fatalf("Rebooted() = %v, want %v", identity.Rebooted(), tc.rebooted)
			}
		})
	}

	t.Run("reboot_without_identity", func(t *testing.T) {
		var raw []call
		c := &registry.Context{FetchRaw: rawPoster(strp("NOT MODIFIED"), &raw)}
		got, err := runAction(ctx, c, NewARServiceAction(common.ARServiceReboot, nil))
		if err != nil || !got.(ARServiceResult).Success {
			t.Fatalf("result = %+v, %v", got, err)
		}
	})
}

// TestTriggersReboot mirrors TestTriggersReboot.
func TestTriggersReboot(t *testing.T) {
	cases := []struct {
		name     string
		services []ARServiceInput
		want     bool
	}{
		{"reboot", []ARServiceInput{common.ARServiceReboot}, true},
		{"reboot_string", []ARServiceInput{"reboot"}, true},
		{"wireless", []ARServiceInput{common.ARServiceWirelessRestart}, false},
		{"mixed", []ARServiceInput{common.ARServiceWirelessRestart, common.ARServiceReboot}, true},
		{"empty", []ARServiceInput{}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := triggersReboot(tc.services); got != tc.want {
				t.Fatalf("triggersReboot = %v, want %v", got, tc.want)
			}
		})
	}
}
