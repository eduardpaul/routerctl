// Probe report model for AsusRouter (report.py).

package probe

import (
	"github.com/eduardpaul/routerctl/asusrouter/tools/security"
)

// Level from which free text is shown as the device wrote it
const revealText = security.ARSecurityLevelReasonable

// ARProbeSection mirrors `ARProbeSection`: one titled block of
// label/value rows in a probe report.
type ARProbeSection struct {
	Title string      `json:"title"`
	Rows  [][2]string `json:"rows"`
}

// ARProbeReport mirrors `ARProbeReport`: the result of probing one source
// on a live device.
type ARProbeReport struct {
	Source   string                   `json:"source"`
	Level    security.ARSecurityLevel `json:"level"`
	Sections []ARProbeSection         `json:"sections"`
}

// Row is one `(label, value)` input row of BuildSection.
type Row struct {
	Label string
	Value any
}

// RenderValue mirrors `render_value`: render one row value for the
// report at level.
func RenderValue(value any, level security.ARSecurityLevel) string {
	if text, ok := value.(string); ok {
		if level >= revealText {
			return text
		}
		return ScrubText(text)
	}

	return security.RenderString(value, level)
}

// BuildSection mirrors `build_section`: build a section, rendering every
// row value at level.
func BuildSection(title string, rows []Row, level security.ARSecurityLevel) ARProbeSection {
	rendered := make([][2]string, 0, len(rows))
	for _, row := range rows {
		rendered = append(rendered, [2]string{row.Label, RenderValue(row.Value, level)})
	}
	return ARProbeSection{Title: title, Rows: rendered}
}
