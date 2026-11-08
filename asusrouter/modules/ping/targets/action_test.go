package targets

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/eduardpaul/routerctl/asusrouter/modules/action"
	"github.com/eduardpaul/routerctl/asusrouter/modules/common"
	"github.com/eduardpaul/routerctl/asusrouter/modules/endpoint"
	"github.com/eduardpaul/routerctl/asusrouter/modules/service"
	"github.com/eduardpaul/routerctl/asusrouter/registry"
)

type call struct {
	endpoint endpoint.AREndpoint
	request  string
}

func reader(calls *[]call, reply any) registry.ReadFunc {
	return func(_ context.Context, ep endpoint.AREndpoint, request string) (any, error) {
		*calls = append(*calls, call{ep, request})
		return reply, nil
	}
}

func TestEncodeTargets(t *testing.T) {
	if got := encodeTargets(parseTargets(raw)); got != raw {
		t.Errorf("roundtrip: %q", got)
	}
	if got := encodeTargets(nil); got != "" {
		t.Errorf("empty: %q", got)
	}
}

func TestNormalize(t *testing.T) {
	tests := []struct {
		name  string
		input ARPingTargetInput
		want  *ARPingTarget
	}{
		{"keeps_target_name_and_ip", target("X", "1.1.1.1"), &ARPingTarget{Name: "X", IP: ip("1.1.1.1")}},
		{"wraps_bare_ip", ip("9.9.9.9"), &ARPingTarget{Name: "target", IP: ip("9.9.9.9")}},
		{"wraps_ip_string", "9.9.9.9", &ARPingTarget{Name: "target", IP: ip("9.9.9.9")}},
		{"drops_invalid_string", "not-an-ip", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalize(tt.input); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAddRemove(t *testing.T) {
	a, b := target("A", "1.1.1.1"), target("B", "8.8.8.8")
	list := func(items ...ARPingTarget) []ARPingTarget { return items }
	tests := []struct {
		name string
		got  []ARPingTarget
		want []ARPingTarget
	}{
		{"add_appends_new_ip", add(list(a), list(b)), list(a, b)},
		{"add_updates_name_on_ip_match", add(list(a), list(target("renamed", "1.1.1.1"))), list(target("renamed", "1.1.1.1"))},
		{"add_same_target_unchanged", add(list(a), list(a)), list(a)},
		{"remove_matches_ip_only", remove(list(a, b), list(target("whatever", "1.1.1.1"))), list(b)},
		{"add_collapses_duplicate_ips", add(list(a, target("B", "1.1.1.1")), nil), list(target("B", "1.1.1.1"))},
		{"remove_collapses_duplicate_ips", remove(list(a, target("B", "1.1.1.1")), list(target("x", "8.8.8.8"))), list(target("B", "1.1.1.1"))},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !reflect.DeepEqual(tt.got, tt.want) {
				t.Errorf("got %v, want %v", tt.got, tt.want)
			}
		})
	}
}

func TestAction(t *testing.T) {
	a := NewARPingTargetsAction(action.ARActionTypeAdd, []ARPingTargetInput{ip("9.9.9.9"), target("X", "1.1.1.1"), "8.8.8.8", "nope"})
	want := []ARPingTarget{target("target", "9.9.9.9"), target("X", "1.1.1.1"), target("target", "8.8.8.8")}
	if a.Op != action.ARActionTypeAdd || !reflect.DeepEqual(a.Targets, want) {
		t.Errorf("normalizes_targets: %+v", a)
	}
	if got := NewARPingTargetsAction(action.ARActionTypeAdd, "192.168.55.21").Targets; !reflect.DeepEqual(got, []ARPingTarget{target("target", "192.168.55.21")}) {
		t.Errorf("accepts_single_string: %v", got)
	}
	if got := NewARPingTargetsAction(action.ARActionTypeAdd, ip("9.9.9.9")).Targets; !reflect.DeepEqual(got, []ARPingTarget{target("target", "9.9.9.9")}) {
		t.Errorf("accepts_single_ip: %v", got)
	}
	x := target("X", "1.1.1.1")
	if got := NewARPingTargetsAction(action.ARActionTypeAdd, x).Targets; !reflect.DeepEqual(got, []ARPingTarget{x}) {
		t.Errorf("accepts_single_target: %v", got)
	}
	if got := NewARPingTargetsAction(action.ARActionTypeClean, nil).Targets; len(got) != 0 {
		t.Errorf("defaults_to_empty: %v", got)
	}
	if e := registry.Action(NewARPingTargetsAction(action.ARActionTypeClean, nil)); e == nil || e.Run == nil {
		t.Error("not registered")
	}
}

func TestRunAction(t *testing.T) {
	ctx := context.Background()
	modified := map[string]any{common.ModifyKey: "1"}
	run := func(t *testing.T, c *registry.Context, a *ARPingTargetsAction) bool {
		t.Helper()
		got, err := RunAction(ctx, c, a)
		if err != nil {
			t.Fatal(err)
		}
		return got.(service.ARServiceResult).Success
	}

	t.Run("clean_skips_fetch", func(t *testing.T) {
		var calls []call
		var list listCalls
		ok := run(t, &registry.Context{Read: reader(&calls, modified), FetchDataMulti: listFetch(&list, raw)}, NewARPingTargetsAction(action.ARActionTypeClean, nil))
		if !ok || list.n != 0 || calls[0].endpoint != endpoint.AREndpointPushData || !strings.Contains(calls[0].request, "action_mode") {
			t.Errorf("ok %v, list %+v, calls %v", ok, list, calls)
		}
	})

	t.Run("add_reads_and_applies", func(t *testing.T) {
		var calls []call
		var list listCalls
		ok := run(t, &registry.Context{Read: reader(&calls, modified), FetchDataMulti: listFetch(&list, "<A>1.1.1.1")},
			NewARPingTargetsAction(action.ARActionTypeAdd, []ARPingTargetInput{ip("8.8.8.8")}))
		if !ok || list.n != 1 || !list.force[0] || !strings.Contains(calls[0].request, "1.1.1.1") || !strings.Contains(calls[0].request, "8.8.8.8") {
			t.Errorf("ok %v, list %+v, calls %v", ok, list, calls)
		}
	})

	t.Run("remove_reads_and_applies", func(t *testing.T) {
		var calls []call
		var list listCalls
		ok := run(t, &registry.Context{Read: reader(&calls, modified), FetchDataMulti: listFetch(&list, "<A>1.1.1.1<B>8.8.8.8")},
			NewARPingTargetsAction(action.ARActionTypeRemove, []ARPingTargetInput{ip("1.1.1.1")}))
		if !ok || strings.Contains(calls[0].request, "1.1.1.1") || !strings.Contains(calls[0].request, "8.8.8.8") {
			t.Errorf("ok %v, calls %v", ok, calls)
		}
	})

	for name, tc := range map[string]struct {
		op      action.ARActionType
		current string
		input   string
	}{
		"add_unchanged_skips_write": {action.ARActionTypeAdd, "<target>9.9.9.9", "9.9.9.9"},
		"remove_absent_skips_write": {action.ARActionTypeRemove, "<A>1.1.1.1", "8.8.8.8"},
	} {
		t.Run(name, func(t *testing.T) {
			var calls []call
			var list listCalls
			ok := run(t, &registry.Context{Read: reader(&calls, nil), FetchDataMulti: listFetch(&list, tc.current)}, NewARPingTargetsAction(tc.op, tc.input))
			if !ok || list.n != 1 || len(calls) != 0 {
				t.Errorf("ok %v, list %+v, calls %v", ok, list, calls)
			}
		})
	}

	t.Run("add_without_targets", func(t *testing.T) {
		var calls []call
		var list listCalls
		ok := run(t, &registry.Context{Read: reader(&calls, nil), FetchDataMulti: listFetch(&list, raw)}, NewARPingTargetsAction(action.ARActionTypeAdd, []ARPingTargetInput{"not-an-ip"}))
		if ok || len(calls) != 0 || list.n != 0 {
			t.Errorf("ok %v, list %+v, calls %v", ok, list, calls)
		}
	})

	t.Run("add_without_data_callback", func(t *testing.T) {
		var calls []call
		if run(t, &registry.Context{Read: reader(&calls, nil)}, NewARPingTargetsAction(action.ARActionTypeAdd, []ARPingTargetInput{ip("1.1.1.1")})) || len(calls) != 0 {
			t.Error("succeeded")
		}
	})

	t.Run("unknown_op", func(t *testing.T) {
		var calls []call
		var list listCalls
		if run(t, &registry.Context{Read: reader(&calls, nil), FetchDataMulti: listFetch(&list, raw)}, NewARPingTargetsAction(action.ARActionTypeUnknown, nil)) || len(calls) != 0 {
			t.Error("succeeded")
		}
	})

	for name, reply := range map[string]any{"modify_false": map[string]any{common.ModifyKey: "0"}, "non_dict_response": nil} {
		t.Run(name, func(t *testing.T) {
			var calls []call
			if run(t, &registry.Context{Read: reader(&calls, reply)}, NewARPingTargetsAction(action.ARActionTypeClean, nil)) {
				t.Error("succeeded")
			}
		})
	}
}
