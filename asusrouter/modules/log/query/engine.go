package query

// Find and group processed log events.

import "github.com/eduardpaul/routerctl/asusrouter/modules/log"

// Find mirrors `find`: the events matching a predicate, order preserved.
func Find(events []*ARLogEvent, predicate ARLogPredicate) []*ARLogEvent {
	out := []*ARLogEvent{}
	for _, event := range events {
		if predicate.Matches(event) {
			out = append(out, event)
		}
	}
	return out
}

// Group is one partition of `group`: a dimension value and its events.
type Group struct {
	Key    any
	Events []*ARLogEvent
}

// GroupBy mirrors `group`: partition events by a dimension's value
// (multi-valued events repeat). Python returns a dict; Go returns its
// items in the same (first-seen) order.
func GroupBy(events []*ARLogEvent, by log.AREventKey) []Group {
	var result []Group
	index := map[any]int{}
	for _, event := range events {
		for _, value := range Resolve(event, by) {
			key := pyKey(value)
			i, ok := index[key]
			if !ok {
				i = len(result)
				index[key] = i
				result = append(result, Group{Key: value})
			}
			result[i].Events = append(result[i].Events, event)
		}
	}

	return result
}
