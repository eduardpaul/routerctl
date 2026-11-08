package log

// Log parser for AsusRouter.

import (
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/eduardpaul/routerctl/asusrouter/modules/log/internal/pyre"
	"github.com/eduardpaul/routerctl/asusrouter/tools/converters"
	"github.com/eduardpaul/routerctl/asusrouter/tools/security"
)

// The reply is a broken-JSON object whose value is the raw, unquoted,
// multi-line syslog text: {"nvram_dump-syslog.log":<log>}
const envelopeKey = `"nvram_dump-syslog.log":`

// Split log by BusyBox syslog timestamp
var recordHead = pyre.Compile(`([A-Z][a-z]{2}\s+\d{1,2}\s+\d{2}:\d{2}:\d{2})\s+`)

// Firmware colours some kernel messages, clean it
var ansi = pyre.Compile(`\x1b\[[0-9;]*[A-Za-z]`)

// A log is written by a small number of daemons
const programCacheSize = 512

// AnchorLength mirrors `ANCHOR_LENGTH`.
// How much log text is kept to continue the next read from
const AnchorLength = 16384

// AnchorRecordsMin mirrors `ANCHOR_RECORDS_MIN`.
// How many whole records an overlap must span to be believed
const AnchorRecordsMin = 3

// A program token with `[pid]`
var pidRe = pyre.Compile(`^(?P<name>.*?)\[(?P<pid>\d+)\]$`)

// An indexed program token
var indexedRe = pyre.Compile(`^(?P<base>.*?)(?P<index>\d+)$`)

// Raw tokens that resolve to a different program than their own name
var aliases = map[string]ARProgram{
	// AiMesh lib firing rc_service commands
	// The most of there are already reported by rc_service
	"amas_lib": ARProgramRcService,
	// Case mismatch
	"HTTPD": ARProgramHttpDaemon,
}

// Freeform lifecycle lines carry no `tag:` prefix
var freeform = []ARProgram{ARProgramSyslog}

// A message whose tag is empty (`: fwver: ...`) still names its writer
// by how it starts - other firmware writes the same line as `init: ...`
var untagged = []struct {
	prefix  string
	program ARProgram
}{{"fwver:", ARProgramInit}}

type freeformPrefix struct {
	prefix  string
	program *ARLogProgram
}

// Trailing space avoids matching a real `tag:` prefix
var freeformPrefixes = func() []freeformPrefix {
	out := make([]freeformPrefix, 0, len(freeform))
	for _, program := range freeform {
		out = append(out, freeformPrefix{string(program) + " ", &ARLogProgram{Name: string(program)}})
	}
	return out
}()

func untaggedProgram(content string) *ARLogProgram {
	for _, item := range untagged {
		if strings.HasPrefix(content, item.prefix) {
			return program(string(item.program), nil)
		}
	}

	return nil
}

func freeformProgram(message string) *ARLogProgram {
	for _, item := range freeformPrefixes {
		if strings.HasPrefix(message, item.prefix) {
			return item.program
		}
	}

	return nil
}

type programKey struct {
	name   string
	pid    int
	hasPid bool
}

var (
	programMu    sync.Mutex
	programCache = map[programKey]*ARLogProgram{}
)

// The emitter is immutable and repeats on nearly every entry
// (Python `lru_cache`: entries of one emitter share one object)
func program(name string, pid *int) *ARLogProgram {
	key := programKey{name: name}
	if pid != nil {
		key.pid, key.hasPid = *pid, true
	}

	programMu.Lock()
	defer programMu.Unlock()
	if cached, ok := programCache[key]; ok {
		return cached
	}
	if len(programCache) >= programCacheSize {
		programCache = map[programKey]*ARLogProgram{}
	}
	created := &ARLogProgram{Name: name, Pid: pid}
	programCache[key] = created
	return created
}

type parsedProgram struct {
	kind  ARProgram
	index *int
}

var (
	parseProgramMu    sync.Mutex
	parseProgramCache = map[string]parsedProgram{}
)

// A log holds few distinct program tokens
const parseProgramCacheSize = 256

// ParseProgram mirrors `parse_program`: resolve a raw program token into
// a kind and optional index.
func ParseProgram(name string) (ARProgram, *int) {
	parseProgramMu.Lock()
	cached, ok := parseProgramCache[name]
	parseProgramMu.Unlock()
	if ok {
		return cached.kind, copyInt(cached.index)
	}

	kind, index := parseProgram(name)

	parseProgramMu.Lock()
	if len(parseProgramCache) >= parseProgramCacheSize {
		parseProgramCache = map[string]parsedProgram{}
	}
	parseProgramCache[name] = parsedProgram{kind, index}
	parseProgramMu.Unlock()
	return kind, copyInt(index)
}

func parseProgram(name string) (ARProgram, *int) {
	if alias, ok := aliases[name]; ok {
		return alias, nil
	}

	if match := indexedRe.FindStringSubmatchIndex(name); match != nil {
		baseText, _ := pyre.Group(indexedRe, name, match, "base")
		base := ARProgramFromValue(baseText)
		if base != ARProgramUnknown {
			indexText, _ := pyre.Group(indexedRe, name, match, "index")
			if index, ok := pyre.Int(indexText, 10); ok {
				return base, &index
			}
		}
	}

	return ARProgramFromValue(name), nil
}

func copyInt(v *int) *int {
	if v == nil {
		return nil
	}
	c := *v
	return &c
}

func rstrip(s string) string { return strings.TrimRightFunc(s, converters.PyIsSpace) }

func lstrip(s string) string { return strings.TrimLeftFunc(s, converters.PyIsSpace) }

// splitMessage splits a message into its program and content.
func splitMessage(message string) (*ARLogProgram, string) {
	if ff := freeformProgram(message); ff != nil {
		return ff, message
	}

	head, tail, separator := strings.Cut(message, ":")
	if !separator {
		return nil, message
	}

	// The record head consumed the space after the stamp, so a message
	// never starts on whitespace and only its tail needs trimming
	name := rstrip(head)
	var pid *int
	// only a `[pid]` suffix can make the token match, and most never do
	if strings.HasSuffix(name, "]") {
		if match := pidRe.FindStringSubmatchIndex(name); match != nil {
			pidText, _ := pyre.Group(pidRe, name, match, "pid")
			if value, ok := pyre.Int(pidText, 10); ok {
				name, _ = pyre.Group(pidRe, name, match, "name")
				pid = &value
			}
		}
	}

	if name == "" {
		content := lstrip(tail)
		return untaggedProgram(content), content
	}

	return program(name, pid), lstrip(tail)
}

// StripEnvelope mirrors `strip_envelope`: strip the reply wrapper and
// colour escapes from a log reply.
func StripEnvelope(content any) string {
	text := ""
	if s := converters.RawToStr(content); s != nil {
		text = *s
	}

	if index := strings.Index(text, envelopeKey); index != -1 {
		text = rstrip(text[index+len(envelopeKey):])
		text = strings.TrimSuffix(text, "}")
	}

	if strings.Contains(text, "\x1b") {
		text = ansi.ReplaceAllString(text, "")
	}

	return rstrip(text)
}

// AnchorOf mirrors `anchor_of`: return the tail of a log text, to
// continue from on the next read (the last AnchorLength characters).
func AnchorOf(text string) string {
	return text[lastRunes(text, AnchorLength):]
}

// lastRunes returns the byte offset where the last n characters start.
func lastRunes(text string, n int) int {
	offset := len(text)
	for ; n > 0 && offset > 0; n-- {
		_, size := utf8.DecodeLastRuneInString(text[:offset])
		offset -= size
	}
	return offset
}

// FindContinuation mirrors `find_continuation`: locate where the records
// new since `anchor` begin, as a character offset (nil when it cannot).
func FindContinuation(text, anchor string) *int {
	offset := findContinuation(text, anchor)
	if offset < 0 {
		return nil
	}
	chars := utf8.RuneCountInString(text[:offset])
	return &chars
}

// findContinuation is FindContinuation as a byte offset (-1 for None).
func findContinuation(text, anchor string) int {
	if anchor == "" {
		return -1
	}

	// A device drops the oldest records, never the newest, so the tail
	// anchored on usually survives whole
	if found := strings.LastIndex(text, anchor); found != -1 {
		return found + len(anchor)
	}

	// It dropped into the anchor itself (searched by character count)
	low, high, best := 1, utf8.RuneCountInString(anchor)-1, 0
	for low <= high {
		middle := (low + high) / 2
		if strings.LastIndex(text, anchor[lastRunes(anchor, middle):]) != -1 {
			best, low = middle, middle+1
		} else {
			high = middle - 1
		}
	}

	if best == 0 {
		return -1
	}

	tail := anchor[lastRunes(anchor, best):]
	// Matching a few characters is chance
	if len(recordHead.FindAllStringIndex(tail, -1)) < AnchorRecordsMin {
		return -1
	}

	return strings.LastIndex(text, tail) + len(tail)
}

// ParseLog mirrors `parse_log`: parse a raw log reply into ordered
// entries. after is nil for Python `after=None`.
func ParseLog(content any, after *string) []*ARLogEntry {
	return ParseText(StripEnvelope(content), after)
}

// ParseText mirrors `parse_text`: parse stripped log text into ordered
// entries. after is nil for Python `after=None`.
func ParseText(text string, after *string) []*ARLogEntry {
	if after != nil {
		if offset := findContinuation(text, *after); offset >= 0 {
			text = text[offset:]
		}
	}

	// The split yields `[prefix, stamp, message, stamp, message, ...]`
	parts := pyre.Split(recordHead, text)
	entries := make([]*ARLogEntry, 0, len(parts)/2)
	for index := 1; index < len(parts)-1; index += 2 {
		// The split already dropped the whitespace ahead of the message
		message := rstrip(parts[index+1])
		prog, content := splitMessage(message)
		entries = append(entries, &ARLogEntry{
			TimestampStr: parts[index],
			Program:      prog,
			Content:      security.NewSensitiveText(content),
		})
	}

	return entries
}
