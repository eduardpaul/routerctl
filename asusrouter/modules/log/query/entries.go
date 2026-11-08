package query

// Log query helpers for AsusRouter.

import (
	"time"

	"github.com/eduardpaul/routerctl/asusrouter/modules/log"
)

// programTally holds running per-program totals collected in a single
// pass.
type programTally struct {
	// `pids` is an ordered set
	pids    []int
	seen    map[int]struct{}
	pidless bool
	count   int
}

// Summarize mirrors `summarize`: build the log data dict; `events` swaps
// the list key and format. events nil is Python `events=None`; a
// non-nil (even empty) slice is a list.
func Summarize(entries []*log.ARLogEntry, total int, events []*ARLogEvent, programs bool) map[log.ARLogField]any {
	data := map[log.ARLogField]any{
		log.ARLogFieldTotal: total,
	}
	if events == nil {
		data[log.ARLogFieldEntryCount] = len(entries)
		if len(entries) > 0 {
			data[log.ARLogFieldLastEntry] = entries[len(entries)-1].Timestamp
		} else {
			data[log.ARLogFieldLastEntry] = (*time.Time)(nil)
		}
		data[log.ARLogFieldEntryList] = entries
	} else {
		data[log.ARLogFieldEntryCount] = len(events)
		if len(events) > 0 {
			data[log.ARLogFieldLastEntry] = events[len(events)-1].Timestamp
		} else {
			data[log.ARLogFieldLastEntry] = (*time.Time)(nil)
		}
		data[log.ARLogFieldEventList] = events
	}
	if programs {
		data[log.ARLogFieldProgramList] = GroupPrograms(entries)
	}

	return data
}

// LastEntries mirrors `last_entries`: all items, or only the last
// `count` when given (nil is Python None).
func LastEntries[T any](items []T, count *int) []T {
	if count == nil || *count >= len(items) {
		return items
	}
	if *count <= 0 {
		return []T{}
	}

	return items[len(items)-*count:]
}

// GroupPrograms mirrors `group_programs`: group programs by name into
// `ARLogProgramGroup`s (one per name, in first-seen order).
func GroupPrograms(entries []*log.ARLogEntry) []log.ARLogProgramGroup {
	seen := map[string]*programTally{}
	var order []string
	for _, entry := range entries {
		program := entry.Program
		if program == nil {
			continue
		}
		tally, ok := seen[program.Name]
		if !ok {
			tally = &programTally{seen: map[int]struct{}{}}
			seen[program.Name] = tally
			order = append(order, program.Name)
		}
		tally.count++
		if program.Pid == nil {
			tally.pidless = true
		} else if _, dup := tally.seen[*program.Pid]; !dup {
			tally.seen[*program.Pid] = struct{}{}
			tally.pids = append(tally.pids, *program.Pid)
		}
	}

	groups := make([]log.ARLogProgramGroup, 0, len(order))
	for _, name := range order {
		tally := seen[name]
		kind, index := log.ParseProgram(name)
		pids := tally.pids
		if pids == nil {
			pids = []int{}
		}
		groups = append(groups, log.ARLogProgramGroup{
			Name:    name,
			Program: kind,
			Index:   index,
			Pids:    pids,
			Pidless: tally.pidless,
			Count:   tally.count,
		})
	}

	return groups
}

// UniqueEntries mirrors `unique_entries`: entries with a unique content,
// keeping first occurrence.
func UniqueEntries(entries []*log.ARLogEntry) []*log.ARLogEntry {
	seen := map[string]struct{}{}
	result := []*log.ARLogEntry{}
	for _, entry := range entries {
		content := entry.ContentValue()
		if _, ok := seen[content]; ok {
			continue
		}
		seen[content] = struct{}{}
		result = append(result, entry)
	}

	return result
}
