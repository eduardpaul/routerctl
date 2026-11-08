package ddns

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/eduardpaul/routerctl/asusrouter/modules/action"
	"github.com/eduardpaul/routerctl/asusrouter/modules/common"
	"github.com/eduardpaul/routerctl/asusrouter/modules/endpoint"
	"github.com/eduardpaul/routerctl/asusrouter/modules/nvram"
	nvramsource "github.com/eduardpaul/routerctl/asusrouter/modules/nvram/source"
	"github.com/eduardpaul/routerctl/asusrouter/modules/service"
	"github.com/eduardpaul/routerctl/asusrouter/modules/source"
	"github.com/eduardpaul/routerctl/asusrouter/registry"
	"github.com/eduardpaul/routerctl/asusrouter/tools/converters"
	"github.com/eduardpaul/routerctl/asusrouter/tools/identifiers"
	"github.com/eduardpaul/routerctl/asusrouter/tools/poll"
)

// ARDdnsConfig is the config: it shares the shape the read source emits,
// for a clean round-trip.
type ARDdnsConfig = map[ARDdnsField]any

// The action type reported in `asusddns_reg_result` by a deregistration
const unregisterPrefix = "unregister,"

// Deregistration result polling, mirroring the web UI cadence
const pollAttempts = 6

// pollInterval is a variable so tests can shorten it
var pollInterval = 500 * time.Millisecond

// ARDdnsAction mirrors `ARDdnsAction`: mutate the DDNS configuration or
// trigger a DDNS operation.
type ARDdnsAction struct {
	Command ARDdnsCommand
	State   *bool
	// SET: the config fields to write; other commands ignore it
	Config ARDdnsConfig
}

// ActionName returns the Python class name.
func (*ARDdnsAction) ActionName() string { return "ARDdnsAction" }

// ActionKey keys by command, state and the config fields.
func (a *ARDdnsAction) ActionKey() []any {
	var state any
	if a.State != nil {
		state = *a.State
	}
	items := make([]string, 0, len(a.Config))
	for field, value := range a.Config {
		items = append(items, fmt.Sprintf("(%s, %s)", converters.PyRepr(string(field)), converters.PyRepr(value)))
	}
	sort.Strings(items)
	return []any{a.Command, state, "(" + strings.Join(items, ", ") + ")"}
}

func serializeStr(value any) (any, bool) {
	// Serialize a text config value
	switch v := value.(type) {
	case identifiers.Password:
		return v.Value(), true
	case *identifiers.Password:
		if v != nil {
			return v.Value(), true
		}
	case fmt.Stringer:
		return v.String(), true
	}
	return converters.PyStr(value), true
}

func serializeBool(value any) (any, bool) {
	// Serialize a flag config value
	flag := converters.RawToBool(value)
	if flag == nil {
		return nil, false
	}
	return boolInt(*flag), true
}

func serializeServer(value any) (any, bool) {
	// Serialize a server config value
	server := ARDdnsServerFromValue(value)
	if server == ARDdnsServerUnknown {
		return nil, false
	}
	return string(server), true
}

func serializeInt(value any) (any, bool) {
	i := converters.RawToInt(value)
	if i == nil {
		return nil, false
	}
	return *i, true
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// Writable config fields: field -> nvram key, serializer
var write = []struct {
	field     ARDdnsField
	key       nvram.ARNvramType
	serialize func(any) (any, bool)
}{
	{ARDdnsFieldHostname, nvram.ARNvramTypeDdnsHostname, serializeStr},
	{ARDdnsFieldIpv6Update, nvram.ARNvramTypeDdnsIpv6Update, serializeBool},
	{ARDdnsFieldPassword, nvram.ARNvramTypeDdnsPassword, serializeStr},
	{ARDdnsFieldRefreshInterval, nvram.ARNvramTypeDdnsRefreshInterval, serializeInt},
	{ARDdnsFieldServer, nvram.ARNvramTypeDdnsServer, serializeServer},
	{ARDdnsFieldUsername, nvram.ARNvramTypeDdnsUsername, serializeStr},
	{ARDdnsFieldVerification, nvram.ARNvramTypeDdnsRegularCheck, serializeBool},
	{ARDdnsFieldVerificationPeriod, nvram.ARNvramTypeDdnsRegularPeriod, serializeInt},
	{ARDdnsFieldWanUnit, nvram.ARNvramTypeDdnsWanUnit, serializeInt},
	{ARDdnsFieldWildcard, nvram.ARNvramTypeDdnsWildcard, serializeBool},
}

// The fields a SET can write; STATE rides along via `action.state`
var writable = func() map[ARDdnsField]bool {
	out := map[ARDdnsField]bool{}
	for _, row := range write {
		out[row.field] = true
	}
	return out
}()

func fetchCurrent(ctx context.Context, c *registry.Context) (ARDdnsConfig, error) {
	// Read the current DDNS data, or nil when it cannot be fetched
	if c.FetchData == nil {
		return nil, nil
	}
	fetched, err := c.FetchData(ctx, ARDdnsSourceUniversal, nil)
	if err != nil {
		return nil, err
	}
	data, _ := fetched.(ARDdnsConfig)
	return data, nil
}

func stateArguments(a *ARDdnsAction) [][2]any {
	// Build the arguments for the master toggle
	if a.State == nil {
		slog.Debug("STATE rejected: no state given")
		return nil
	}
	return [][2]any{{string(nvram.ARNvramTypeDdnsState), boolInt(*a.State)}}
}

func configArguments(ctx context.Context, a *ARDdnsAction, c *registry.Context) ([][2]any, error) {
	// Build the arguments for a config write

	// Surface config the SET cannot apply instead of dropping it silently
	var dropped []string
	for field := range a.Config {
		if !writable[field] {
			dropped = append(dropped, string(field))
		}
	}
	if len(dropped) > 0 {
		slog.Debug("SET ignores non-writable config fields", "fields", dropped)
	}

	var arguments [][2]any
	for _, row := range write {
		value, ok := a.Config[row.field]
		if !ok {
			continue
		}
		// No value clears the setting
		var serialized any = ""
		if value != nil {
			s, valid := row.serialize(value)
			if !valid {
				slog.Debug("SET rejected: bad value", "field", row.field, "value", value)
				return nil, nil
			}
			serialized = s
		}
		arguments = append(arguments, [2]any{string(row.key), serialized})
	}

	if len(arguments) == 0 && a.State == nil {
		slog.Debug("SET rejected: no writable fields given")
		return nil, nil
	}

	// The toggle can ride along with a config write
	if a.State != nil {
		arguments = append(arguments, [2]any{string(nvram.ARNvramTypeDdnsState), boolInt(*a.State)})
	}

	// A hostname change unbinds the registered one, as the web UI does
	for _, kv := range arguments {
		if kv[0] != string(nvram.ARNvramTypeDdnsHostname) {
			continue
		}
		current, err := fetchCurrent(ctx, c)
		if err != nil {
			return nil, err
		}
		if current != nil && !hostnameEqual(current[ARDdnsFieldHostname], kv[1]) {
			arguments = append(arguments, [2]any{string(nvram.ARNvramTypeDdnsReplaceStatus), 0})
		}
		break
	}

	return arguments, nil
}

// hostnameEqual mirrors `current.get(HOSTNAME) == hostname` (a Hostname
// equals a string of the same value).
func hostnameEqual(current any, hostname any) bool {
	switch h := current.(type) {
	case identifiers.Hostname:
		return h.Equal(hostname)
	case *identifiers.Hostname:
		return h != nil && h.Equal(hostname)
	case nil:
		return hostname == nil
	}
	return current == hostname
}

func dispatched(value *string, isRaw bool, read any) bool {
	// Whether a raw endpoint call reached the device.
	//
	// A raw fetch returns the body (possibly empty) on success and nil on
	// failure, while async_read collapses a failed request to {} instead.
	if isRaw {
		return value != nil
	}
	if read == nil {
		return false
	}
	if m, ok := read.(map[string]any); ok && len(m) == 0 {
		return false
	}
	return true
}

// post sends a request with fetch_raw_callback (or the read callback)
// and reports whether it was dispatched.
func post(ctx context.Context, c *registry.Context, ep endpoint.AREndpoint) (bool, error) {
	if c.FetchRaw != nil {
		raw, err := c.FetchRaw(ctx, ep, "")
		if err != nil {
			return false, err
		}
		return dispatched(raw, true, nil), nil
	}
	read, err := c.Read(ctx, ep, "")
	if err != nil {
		return false, err
	}
	return dispatched(nil, false, read), nil
}

func deregister(ctx context.Context, c *registry.Context) (bool, error) {
	// Release the registered ASUS DDNS hostname

	// A hostname bound to a Router-app account cannot be released here
	current, err := fetchCurrent(ctx, c)
	if err != nil {
		return false, err
	}
	if current != nil {
		if bound, ok := current[ARDdnsFieldTokenState].(bool); ok && bound {
			slog.Debug("DEREGISTER rejected: the hostname is bound to an app account")
			return false, nil
		}
	}

	ok, err := post(ctx, c, endpoint.AREndpointDdnsUnregister)
	if err != nil || !ok {
		return false, err
	}

	// Wait for the device to report the deregistration outcome
	key := nvram.ARNvramTypeDdnsRegistrationResult
	request := endpoint.HookRequest(key)

	probe := func(ctx context.Context) (any, error) {
		return c.Read(ctx, endpoint.AREndpointFetchData, request)
	}

	ready := func(result any) bool {
		value := endpoint.HookValue(result, key)
		text := ""
		if value != nil {
			text = converters.PyStr(value)
		}
		return strings.HasPrefix(text, unregisterPrefix)
	}

	outcome, err := poll.AsyncPollUntil(ctx, probe, ready, pollInterval, pollAttempts)
	if err != nil {
		return false, err
	}
	if outcome == nil {
		slog.Debug("DEREGISTER failed: no result from the device")
		return false, nil
	}
	result := endpoint.HookValue(*outcome, key)
	if ReadStatus(result) != ARDdnsStatusSuccess {
		slog.Debug("DEREGISTER failed", "result", result)
		return false, nil
	}

	// Wipe the released hostname from the device config
	return post(ctx, c, endpoint.AREndpointDdnsClean)
}

// RunAction mirrors `run_action`: apply the DDNS action.
func RunAction(ctx context.Context, c *registry.Context, a action.ARAction) (any, error) {
	act := a.(*ARDdnsAction)

	// UPDATE and DEREGISTER carry no config; flag any that would be dropped
	if (act.Command == ARDdnsCommandUpdate || act.Command == ARDdnsCommandDeregister) &&
		(act.State != nil || len(act.Config) > 0) {
		slog.Debug("DDNS command ignores state and config", "command", act.Command)
	}

	var result service.ARServiceResult
	switch act.Command {
	case ARDdnsCommandDeregister:
		ok, err := deregister(ctx, c)
		if err != nil {
			return nil, err
		}
		result = service.ARServiceResult{Success: ok}
	case ARDdnsCommandUpdate:
		// The web UI forces an update outside the apply flow
		r, err := service.AsyncRunService(ctx, c.Read, common.ARServiceDdnsClient, nil, common.ARActionModeUpdate, c.FetchRaw)
		if err != nil {
			return nil, err
		}
		result = r
	default:
		var arguments [][2]any
		switch act.Command {
		case ARDdnsCommandState:
			arguments = stateArguments(act)
		case ARDdnsCommandSet:
			args, err := configArguments(ctx, act, c)
			if err != nil {
				return nil, err
			}
			arguments = args
		default:
			slog.Debug("Unknown DDNS command", "command", act.Command)
			return service.ARServiceResult{Success: false}, nil
		}

		if len(arguments) == 0 {
			return service.ARServiceResult{Success: false}, nil
		}

		r, err := service.AsyncRunService(ctx, c.Read, common.ARServiceDdnsRestart, arguments, common.ARActionModeApply, c.FetchRaw)
		if err != nil {
			return nil, err
		}
		result = r
	}

	// Drop the now-stale cached config and its per-item nvram values,
	// so the next read refetches from the device
	if result.Success && c.Expire != nil {
		c.Expire(ctx, ARDdnsSourceUniversal)
		nvramsource.AsyncExpireValues(ctx, c.Expire, DdnsRequest)
	}

	return result, nil
}

var _ source.DataSource = ARDdnsSourceUniversal

func init() {
	registry.RegisterAction(registry.ActionEntry{
		Name: "ARDdnsAction",
		Run:  RunAction,
	})
}
