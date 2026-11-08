// Package log mirrors asusrouter/modules/log/.
//
// Log entry model for AsusRouter.
//
// Python `log/__init__.py` re-exports the query helpers and the source;
// Go cannot (the query and source packages import this one), so they are
// imported from `log/query` and `log/source` directly.
package log

import (
	"time"

	"github.com/eduardpaul/routerctl/asusrouter/tools/security"
)

// EmptyContent mirrors `EMPTY_CONTENT`.
// Nothing was written, so there is nothing to protect or to expose
var EmptyContent = security.NewSensitiveText("")

// ARLogProgram mirrors `ARLogProgram`: the program that emitted a log
// entry.
type ARLogProgram struct {
	Name string `json:"name"`
	Pid  *int   `json:"pid"`
}

// ARLogProgramGroup mirrors `ARLogProgramGroup`: a program name with
// every pid it ran under.
type ARLogProgramGroup struct {
	Name    string    `json:"name"` // raw syslog token
	Program ARProgram `json:"program"`
	// instance index for indexed programs (`vpnclient5` -> 5)
	Index   *int  `json:"index"`
	Pids    []int `json:"pids"`
	Pidless bool  `json:"pidless"`
	Count   int   `json:"count"` // total for this program (all pids)
}

// ARLogEntry mirrors `ARLogEntry`: a single parsed syslog entry.
//
// Python freezes the dataclass and only `resolve_datetimes` sets the
// timestamp afterwards; Go shares entries by pointer the same way.
type ARLogEntry struct {
	// Raw values
	TimestampStr string `json:"timestamp_str"`

	// Parsed values
	Timestamp *time.Time             `json:"timestamp"`
	Program   *ARLogProgram          `json:"program"`
	Content   security.SensitiveText `json:"content"`
}

// ProgramName mirrors `program_name`: the raw syslog tag, absent on a
// tag-less entry.
func (e *ARLogEntry) ProgramName() *string {
	if e.Program == nil {
		return nil
	}
	name := e.Program.Name
	return &name
}

// ContentValue returns `content.value`.
func (e *ARLogEntry) ContentValue() string { return e.Content.Value }
