package query

// Unified log filtering interface for AsusRouter.

import "github.com/eduardpaul/routerctl/asusrouter/modules/log"

// FilterOptions are the keyword arguments of `filter_log`.
type FilterOptions struct {
	Predicate ARLogPredicate
	// Last nil is Python None
	Last     *int
	Unique   bool
	Programs bool
}

// FilterLog mirrors `filter_log`: filter, limit and summarize parsed log
// entries into the data dict.
func FilterLog(entries []*log.ARLogEntry, opts FilterOptions) map[log.ARLogField]any {
	total := len(entries)
	if opts.Unique {
		entries = UniqueEntries(entries)
	}

	if opts.Predicate == nil {
		return Summarize(LastEntries(entries, opts.Last), total, nil, opts.Programs)
	}

	// Bound once: the lookup would otherwise repeat per entry
	prescreen, _ := opts.Predicate.(Prescreener)

	// The entries themselves are only needed to group the programs
	matched := []*log.ARLogEntry{}
	events := []*ARLogEvent{}
	for _, entry := range entries {
		if prescreen != nil && !prescreen.Prescreen(entry) {
			continue
		}
		event := BuildEvent(entry)
		if !opts.Predicate.Matches(event) {
			continue
		}
		events = append(events, event)
		if opts.Programs {
			matched = append(matched, entry)
		}
	}

	return Summarize(LastEntries(matched, opts.Last), total, LastEntries(events, opts.Last), opts.Programs)
}
