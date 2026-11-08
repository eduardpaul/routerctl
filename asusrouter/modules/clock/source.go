package clock

import (
	"context"
	"errors"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/eduardpaul/routerctl/asusrouter/consts"
	"github.com/eduardpaul/routerctl/asusrouter/modules/endpoint"
	"github.com/eduardpaul/routerctl/asusrouter/modules/source"
	"github.com/eduardpaul/routerctl/asusrouter/registry"
	"github.com/eduardpaul/routerctl/asusrouter/tools/converters"
)

// Boot times within this many seconds are the same boot (1s counter
// precision jitters); a larger forward jump means an actual reboot
const rebootDeltaThreshold = 2

// Consider no device online more than 100 years. If so, reboot it right away
const uptimeMaxSeconds = 100 * 365 * 24 * 3600

// Old firmware returns the uptime hook as invalid JSON (the value is not
// quoted), so the JSON reader drops it - pull it out of the raw content
var uptimeRe = regexp.MustCompile(`"uptime"` + converters.PyWhitespaceClass + `*:` + converters.PyWhitespaceClass + `*([^}\n]+)`)

// The seconds-since-boot count, inside the parenthesised tail
var secondsRe = regexp.MustCompile(`\d+`)

// extractUptime extracts the uptime value from a raw (possibly non-JSON)
// response.
func extractUptime(content any) *string {
	var text string
	switch v := content.(type) {
	case string:
		text = v
	case *string:
		if v == nil {
			return nil
		}
		text = *v
	default:
		return nil
	}
	m := uptimeRe.FindStringSubmatch(text)
	if m == nil {
		return nil
	}
	value := strings.Trim(converters.PyStrip(m[1]), `"`)
	if value == "" {
		return nil
	}
	return &value
}

// ARBoottime is a boot time - a datetime tagged so the identity sync is
// unambiguous. It always states its offset.
type ARBoottime struct {
	time.Time
}

// errNaiveBoottime mirrors the ValueError of `ARBoottime.__new__`.
var errNaiveBoottime = errors.New("a boot time must state its offset")

// isAware reports whether a time states its offset (is not a Python
// naive datetime).
func isAware(t time.Time) bool {
	return t.Location() != consts.NaiveLocation
}

// ARBoottimeFromDatetime builds a boot time from a plain datetime,
// rejecting one that states no offset.
func ARBoottimeFromDatetime(value time.Time) (ARBoottime, error) {
	if !isAware(value) {
		return ARBoottime{}, errNaiveBoottime
	}
	return ARBoottime{Time: value}, nil
}

// CanonicalValue renders the boot time as a datetime.
func (b ARBoottime) CanonicalValue() any { return b.Time }

// asBoottime tags a datetime as a boot time (nil passes through).
func asBoottime(value *time.Time) *ARBoottime {
	if value == nil {
		return nil
	}
	b, err := ARBoottimeFromDatetime(*value)
	if err != nil {
		return nil
	}
	return &b
}

// ReadUptime reads an uptime string (`<when>(<secs> ...)`) into what it
// states. Values are ARBoottime, time.Time and int.
func ReadUptime(uptime string) map[ARClockField]any {
	before, after, found := strings.Cut(uptime, "(")
	// Python `split("(", 1)` gives fewer than uptimeMinParts parts
	if !found {
		return map[ARClockField]any{}
	}
	match := secondsRe.FindString(after)
	if match == "" {
		return map[ARClockField]any{}
	}
	// The match is digits only
	// Parse as int64 so the bound check also works on 32-bit platforms
	seconds64, err := strconv.ParseInt(match, 10, 64)
	if err != nil || seconds64 > uptimeMaxSeconds {
		return map[ARClockField]any{}
	}
	seconds := int(seconds64)

	when := converters.RawToDatetime(before)
	// A clock that does not state its offset is not a clock
	if when == nil || !isAware(*when) {
		return map[ARClockField]any{ARClockFieldUptime: seconds}
	}

	boot := when.Add(-time.Duration(seconds) * time.Second)
	// A clock this early cannot have been running that long
	if boot.Year() < 1 {
		return map[ARClockField]any{ARClockFieldUptime: seconds}
	}
	boottime := asBoottime(&boot)

	return map[ARClockField]any{
		ARClockFieldBoottime:   *boottime,
		ARClockFieldDeviceTime: *when,
		ARClockFieldUptime:     seconds,
	}
}

// aware drops a datetime that states no offset.
func aware(value *time.Time) *time.Time {
	if value != nil && isAware(*value) {
		return value
	}
	return nil
}

// Stabilize keeps the boot time stable across jitter; update only on
// reboot.
func Stabilize(candidate, prev *time.Time) *ARBoottime {
	candidate = aware(candidate)
	prev = aware(prev)

	if candidate == nil {
		return asBoottime(prev)
	}
	if prev != nil && candidate.Sub(*prev).Seconds() < rebootDeltaThreshold {
		return asBoottime(prev)
	}
	return asBoottime(candidate)
}

// ARClockSource is the clock data source for the connected router.
type ARClockSource struct {
	source.Base
}

// SourceName returns the Python class name.
func (*ARClockSource) SourceName() string { return "ARClockSource" }

// ARClockSourceUniversal is the universal instance - preferred.
var ARClockSourceUniversal = &ARClockSource{}

// fetchState fetches the uptime hook and reads the device's clock out of
// it.
func fetchState(ctx context.Context, c *registry.Context, src source.DataSource) (any, error) {
	request := endpoint.HookRequest(endpoint.ARHookUptime)

	// Prefer the raw content: the uptime value is not valid JSON on old
	// firmware, so the JSON reader would log a spurious decode error
	var uptime *string
	if c.FetchRaw != nil {
		content, err := c.FetchRaw(ctx, endpoint.AREndpointFetchData, request)
		if err != nil {
			return nil, err
		}
		uptime = extractUptime(content)
	}

	if uptime == nil {
		raw, err := c.Read(ctx, endpoint.AREndpointFetchData, request)
		if err != nil {
			return nil, err
		}
		if m, ok := raw.(map[string]any); ok {
			if s, ok := m["uptime"].(string); ok {
				uptime = &s
			}
		}
	}

	state := map[ARClockField]any{}
	if uptime != nil {
		state = ReadUptime(*uptime)
	}

	// Only the boot time is stabilized
	var prev *time.Time
	if c.Identity != nil {
		prev = c.Identity.Boottime()
	}
	var candidate *time.Time
	if b, ok := state[ARClockFieldBoottime].(ARBoottime); ok {
		candidate = &b.Time
	}
	if boottime := Stabilize(candidate, prev); boottime != nil {
		state[ARClockFieldBoottime] = *boottime
	}

	return state, nil
}

func init() {
	registry.RegisterSource(registry.SourceEntry{
		Name:    "ARClockSource",
		Default: func() source.DataSource { return &ARClockSource{} },
		Fetch:   fetchState,
	})
}
