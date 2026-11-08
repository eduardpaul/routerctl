package firmware

import (
	"strings"

	"github.com/eduardpaul/routerctl/asusrouter/tools/converters"
)

// Header lines that are not part of the actual change list
var noteHeaders = []string{"Firmware version", "Release Note"}

// bom mirrors `asusrouter.tools.converters.raw._BOM`
const bom = "\uFEFF"

// ReadFirmwareNote parses a firmware release note into cleaned change text.
//
// Strips the BOM and the header lines, keeping only the change lines.
// Returns nil for an empty note.
func ReadFirmwareNote(content string) *string {
	rawNote := strings.ReplaceAll(content, bom, "")

	var cleanLines []string
	for _, rawLine := range pySplitLines(rawNote) {
		line := converters.RawToStr(rawLine)
		if line == nil || *line == "" {
			continue
		}
		header := false
		for _, h := range noteHeaders {
			if strings.Contains(*line, h) {
				header = true
				break
			}
		}
		if !header {
			cleanLines = append(cleanLines, *line)
		}
	}

	note := strings.Join(cleanLines, "\n")
	if note == "" {
		return nil
	}
	return &note
}

// pySplitLines mirrors Python `str.splitlines()` (no keepends).
func pySplitLines(s string) []string {
	var lines []string
	start := 0
	rs := []rune(s)
	for i := 0; i < len(rs); i++ {
		switch rs[i] {
		case '\n', '\v', '\f', '\x1c', '\x1d', '\x1e', '\x85', '\u2028', '\u2029':
			lines = append(lines, string(rs[start:i]))
			start = i + 1
		case '\r':
			lines = append(lines, string(rs[start:i]))
			if i+1 < len(rs) && rs[i+1] == '\n' {
				i++
			}
			start = i + 1
		}
	}
	if start < len(rs) {
		lines = append(lines, string(rs[start:]))
	}
	return lines
}
