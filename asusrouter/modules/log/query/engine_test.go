package query

import (
	"testing"

	"github.com/eduardpaul/routerctl/asusrouter/modules/common"
	"github.com/eduardpaul/routerctl/asusrouter/modules/log"
	"github.com/eduardpaul/routerctl/asusrouter/modules/log/translate"
)

func engineEvent(program log.ARProgram, data map[log.AREventKey]any) *ARLogEvent {
	return newEvent(eventOpts{
		program:     program,
		programName: strPtr(string(program)),
		eventType:   translate.AREventNtpStartUpdate,
		raw:         "",
		data:        data,
	})
}

func sameEvents(a, b []*ARLogEvent) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestFind(t *testing.T) {
	t.Run("test_filters_and_preserves_order", func(t *testing.T) {
		events := []*ARLogEvent{
			engineEvent(log.ARProgramNtp, nil),
			engineEvent(log.ARProgramKernel, nil),
			engineEvent(log.ARProgramNtp, nil),
		}
		result := Find(events, NewARLogMatch(log.AREventKeyProgram, "ntp"))
		if !sameEvents(result, []*ARLogEvent{events[0], events[2]}) {
			t.Fatalf("got %v", result)
		}
	})
	t.Run("test_no_match", func(t *testing.T) {
		events := []*ARLogEvent{engineEvent(log.ARProgramNtp, nil)}
		if got := Find(events, NewARLogMatch(log.AREventKeyProgram, "kernel")); len(got) != 0 {
			t.Fatalf("got %v", got)
		}
	})
}

// groupsAsMap reads the ordered groups as the Python dict.
func groupsAsMap(groups []Group) map[any][]*ARLogEvent {
	out := map[any][]*ARLogEvent{}
	for _, g := range groups {
		out[pyKey(g.Key)] = g.Events
	}
	return out
}

func TestGroup(t *testing.T) {
	t.Run("test_by_program", func(t *testing.T) {
		events := []*ARLogEvent{
			engineEvent(log.ARProgramNtp, nil),
			engineEvent(log.ARProgramKernel, nil),
			engineEvent(log.ARProgramNtp, nil),
		}
		result := GroupBy(events, log.AREventKeyProgram)
		got := groupsAsMap(result)
		if len(got) != 2 ||
			!sameEvents(got[pyKey(log.ARProgramNtp)], []*ARLogEvent{events[0], events[2]}) ||
			!sameEvents(got[pyKey(log.ARProgramKernel)], []*ARLogEvent{events[1]}) {
			t.Fatalf("got %v", result)
		}
		// Python dicts keep first-seen order
		if result[0].Key != log.ARProgramNtp || result[1].Key != log.ARProgramKernel {
			t.Errorf("order %v", result)
		}
	})
	t.Run("test_multi_valued_repeats", func(t *testing.T) {
		wan, fw := common.ARServiceWanRestart, common.ARServiceFirewallRestart
		both := engineEvent(log.ARProgramRcService, map[log.AREventKey]any{
			log.AREventKeyServices: []common.ARService{wan, fw},
		})
		one := engineEvent(log.ARProgramRcService, map[log.AREventKey]any{log.AREventKeyService: fw})
		got := groupsAsMap(GroupBy([]*ARLogEvent{both, one}, log.AREventKeyService))
		if len(got) != 2 ||
			!sameEvents(got[pyKey(wan)], []*ARLogEvent{both}) ||
			!sameEvents(got[pyKey(fw)], []*ARLogEvent{both, one}) {
			t.Fatalf("got %v", got)
		}
	})
	t.Run("test_missing_axis_omitted", func(t *testing.T) {
		if got := GroupBy([]*ARLogEvent{engineEvent(log.ARProgramNtp, nil)}, log.AREventKeyService); len(got) != 0 {
			t.Fatalf("got %v", got)
		}
	})
}
