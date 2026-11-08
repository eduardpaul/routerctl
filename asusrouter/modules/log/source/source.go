// Package source mirrors asusrouter/modules/log/source.py.
//
// It is a subpackage (not a file of package log) because it imports
// log/query, which imports log: Go forbids that cycle.
package source

// Log data source for AsusRouter.

import (
	"context"
	"time"

	"github.com/eduardpaul/routerctl/asusrouter/config"
	"github.com/eduardpaul/routerctl/asusrouter/consts"
	"github.com/eduardpaul/routerctl/asusrouter/modules/clock"
	"github.com/eduardpaul/routerctl/asusrouter/modules/device"
	"github.com/eduardpaul/routerctl/asusrouter/modules/endpoint"
	"github.com/eduardpaul/routerctl/asusrouter/modules/log"
	"github.com/eduardpaul/routerctl/asusrouter/modules/log/query"
	"github.com/eduardpaul/routerctl/asusrouter/modules/source"
	"github.com/eduardpaul/routerctl/asusrouter/registry"
	"github.com/eduardpaul/routerctl/asusrouter/tools/converters"
	"github.com/eduardpaul/routerctl/asusrouter/tools/identifiers"
	"github.com/eduardpaul/routerctl/asusrouter/tools/security"
)

// A device clock further than this from the host's has not synced
const clockDriftLimit = 24 * time.Hour

// LogHook mirrors `_LOG_HOOK` (exported for the tests).
var LogHook = endpoint.ARHookCall{
	Hook: endpoint.ARHookNvramDump,
	Args: `"syslog.log","syslog.sh"`,
}

// ARLogSource mirrors `ARLogSource`: AsusRouter system log data source.
type ARLogSource struct {
	source.Base
}

// SourceName returns the Python class name.
func (*ARLogSource) SourceName() string { return "ARLogSource" }

// ARLogSourceUniversal is the universal instance - preferred.
var ARLogSourceUniversal = &ARLogSource{}

// now is the host clock (a seam for tests)
var now = time.Now

// asyncEnsureClock reads the device's clock unless the identity holds a
// usable one.
func asyncEnsureClock(ctx context.Context, identity *device.ARDeviceIdentity, fetchData registry.FetchDataFunc) error {
	if fetchData == nil || identity == nil {
		return nil
	}

	deviceTime := identity.DeviceTime()
	if deviceTime != nil &&
		// One stating no offset cannot be measured against the host's
		deviceTime.Location() != consts.NaiveLocation &&
		absDuration(now().Sub(*deviceTime)) <= clockDriftLimit {
		return nil
	}

	_, err := fetchData(ctx, clock.ARClockSourceUniversal, nil)
	return err
}

func absDuration(d time.Duration) time.Duration {
	if d < 0 {
		return -d
	}
	return d
}

// fetchState mirrors `fetch_state`: fetch the raw system log.
func fetchState(ctx context.Context, c *registry.Context, _ source.DataSource) (any, error) {
	// Before the log, so the stamps have a clock to be resolved against
	if err := asyncEnsureClock(ctx, c.Identity, c.FetchData); err != nil {
		return nil, err
	}

	if c.FetchRaw == nil {
		return "", nil
	}

	raw, err := c.FetchRaw(ctx, endpoint.AREndpointFetchData, endpoint.HookRequest(LogHook))
	if err != nil {
		return nil, err
	}
	var content any
	if raw != nil {
		content = *raw
	}
	if s := converters.RawToStr(content); s != nil {
		return *s, nil
	}

	return "", nil
}

// history reads the entries and the anchor an earlier read left behind.
func history(previous any) ([]*log.ARLogEntry, *string) {
	state, ok := previous.(map[log.ARLogField]any)
	if !ok {
		return nil, nil
	}

	kept, _ := state[log.ARLogFieldEntryList].([]*log.ARLogEntry)
	var anchor *string
	if text, ok := state[log.ARLogFieldAnchor].(security.SensitiveText); ok {
		value := text.Value
		anchor = &value
	}

	return kept, anchor
}

// clockAnchor reads the device's own clock - the year and offset a
// stamp lacks.
func clockAnchor(identity *device.ARDeviceIdentity) *time.Time {
	if identity == nil {
		return nil
	}
	return identity.DeviceTime()
}

// unchanged rebuilds the previous result when a reply carried no log
// text.
func unchanged(kept []*log.ARLogEntry, anchor *string) map[log.ARLogField]any {
	state := query.FilterLog(kept, query.FilterOptions{})
	text := ""
	if anchor != nil {
		text = *anchor
	}
	state[log.ARLogFieldAnchor] = security.NewSensitiveText(text)

	return state
}

// withoutSecrets cleans the values only we know out of the device's own
// text.
func withoutSecrets(text string, identity *device.ARDeviceIdentity, level security.ARSecurityLevel) string {
	if identity == nil {
		return text
	}

	var secrets []security.ARSensitive
	if mac := identity.Mac(); mac != nil {
		secrets = append(secrets, mac)
	}
	if username := identity.Username(); username != nil {
		secrets = append(secrets, username)
	}
	if serial := identity.Serial(); serial != nil {
		if s := identifiers.SerialFromValueSafe(*serial); s != nil {
			secrets = append(secrets, s)
		}
	}

	if len(secrets) == 0 {
		return text
	}
	return security.RemoveSecrets(text, secrets, level)
}

// configGetter is the part of a config the translator reads (Python
// takes `kwargs.get("config", ARConfig)`).
type configGetter interface {
	Get(key config.ARConfigKeyBase) any
}

func configOf(tc *registry.TranslateContext) configGetter {
	if tc != nil {
		if cfg, ok := tc.Kwargs["config"].(configGetter); ok {
			return cfg
		}
	}
	return config.ARConfig
}

// translateState mirrors `translate_state`: parse the raw log into the
// lean dict, continuing an earlier read.
func translateState(data any, tc *registry.TranslateContext) any {
	var identity *device.ARDeviceIdentity
	var previous any
	if tc != nil {
		identity, previous = tc.Identity, tc.Previous
	}

	kept, anchor := history(previous)
	cfg := configOf(tc)
	// Before parsing, so the anchor kept for the next read and the text
	// that read searches are cleaned the same way
	text := ""
	if raw, ok := data.(string); ok {
		level := security.ARSecurityLevelFromValue(cfg.Get(config.ARConfigKeySecurityLevelData))
		text = withoutSecrets(log.StripEnvelope(raw), identity, level)
	}
	// A reply with no text says nothing about the log, so the previous
	// result stands rather than being overwritten by an empty one
	if text == "" {
		return unchanged(kept, anchor)
	}

	// Reading only what is new keeps a poll off the whole buffer, and
	// an anchor that no longer matches simply reads all of it again
	entries := log.ResolveDatetimes(log.ParseText(text, anchor), clockAnchor(identity))
	if len(kept) > 0 {
		entries = append(append(make([]*log.ARLogEntry, 0, len(kept)+len(entries)), kept...), entries...)
	}

	if limit := converters.RawToInt(cfg.Get(config.ARConfigKeyLogHistoryLimit)); limit != nil &&
		*limit > 0 && len(entries) > *limit {
		entries = entries[len(entries)-*limit:]
	}

	state := query.FilterLog(entries, query.FilterOptions{})
	// Device text like any other: bookkeeping the next read reaches
	// through `.value`, not something to hand out
	state[log.ARLogFieldAnchor] = security.NewSensitiveText(log.AnchorOf(text))

	return state
}

func init() {
	registry.RegisterSource(registry.SourceEntry{
		Name:      "ARLogSource",
		Default:   func() source.DataSource { return &ARLogSource{} },
		Fetch:     fetchState,
		Translate: translateState,
	})
}
