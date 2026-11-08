// Package service mirrors asusrouter/modules/service/.
package service

import (
	"context"
	"encoding/json"
	"io"
	"strconv"
	"strings"

	"github.com/eduardpaul/routerctl/asusrouter/modules/action"
	"github.com/eduardpaul/routerctl/asusrouter/modules/common"
	"github.com/eduardpaul/routerctl/asusrouter/modules/endpoint"
	"github.com/eduardpaul/routerctl/asusrouter/registry"
	"github.com/eduardpaul/routerctl/asusrouter/tools/converters"
)

// Response keys of a service run
const (
	RunServiceKey        = "run_service"
	RestartNeededTimeKey = "restart_needed_time"
	IDKey                = "id"
)

// Fallback wait (seconds) when the device reports an id but no restart time
const defaultNeededTime = 5

// ARServiceInput is a service, or something coercible into one (a raw
// string may carry an inline argument, e.g. `restart_sdn 3`).
//
// Python `ARService | str`; ARService is a string type in Go, so a raw
// string converts with `ARServiceInput("restart_sdn 3")`.
type ARServiceInput = common.ARService

// Services that drop the current session by rebooting the device
var rebootServices = map[ARServiceInput]bool{common.ARServiceReboot: true}

// triggersReboot mirrors `_triggers_reboot`: whether any requested
// service reboots the device.
func triggersReboot(services []ARServiceInput) bool {
	for _, service := range services {
		if rebootServices[service] {
			return true
		}
	}
	return false
}

// asList mirrors `_as_list`: coerce a single service or an iterable of
// them into a list. Accepts ARService, string, their slices and []any.
func asList(services any) []ARServiceInput {
	switch v := services.(type) {
	case nil:
		return []ARServiceInput{}
	case common.ARService:
		return []ARServiceInput{v}
	case string:
		return []ARServiceInput{ARServiceInput(v)}
	case []common.ARService:
		return append([]ARServiceInput{}, v...)
	case []string:
		out := make([]ARServiceInput, 0, len(v))
		for _, s := range v {
			out = append(out, ARServiceInput(s))
		}
		return out
	case []any:
		out := make([]ARServiceInput, 0, len(v))
		for _, item := range v {
			out = append(out, asList(item)...)
		}
		return out
	}
	return []ARServiceInput{ARServiceInput(converters.PyStr(services))}
}

// ARServiceResult mirrors `ARServiceResult`: the outcome of a service run.
type ARServiceResult struct {
	Success    bool `json:"success"`
	NeededTime *int `json:"needed_time"`
	LastID     *int `json:"last_id"`
}

// Bool mirrors `__bool__`: truthy when the run succeeded.
func (r ARServiceResult) Bool() bool {
	return r.Success
}

// ARServiceAction mirrors `ARServiceAction`: run one or more device
// services via `rc_service`.
type ARServiceAction struct {
	action.Base

	Services []ARServiceInput
	// Arguments keep the Python dict order (they end up in the payload)
	Arguments [][2]any
}

// ActionName returns the Python class name.
func (*ARServiceAction) ActionName() string { return "ARServiceAction" }

// NewARServiceAction mirrors `ARServiceAction.__init__`: the service(s)
// (see asList) and optional arguments.
func NewARServiceAction(services any, arguments [][2]any) *ARServiceAction {
	var args [][2]any
	if len(arguments) > 0 {
		args = append([][2]any{}, arguments...)
	}
	return &ARServiceAction{Services: asList(services), Arguments: args}
}

// joinServices renders services as the `;`-joined rc_service string.
func joinServices(services []ARServiceInput) string {
	names := make([]string, 0, len(services))
	for _, service := range services {
		names = append(names, string(service))
	}
	return strings.Join(names, ";")
}

// BuildServiceRequest mirrors `build_service_request`: an applyapp
// request body that runs one or more services. An empty actionMode is
// `ARActionMode.APPLY`.
func BuildServiceRequest(services any, arguments [][2]any, actionMode common.ARActionMode) string {
	if actionMode == "" {
		actionMode = common.ARActionModeApply
	}
	names := joinServices(asList(services))
	payload := [][2]any{{common.RcServiceKey, names}}
	if len(arguments) > 0 {
		payload = append(payload, arguments...)
	}
	return endpoint.BuildPushRequest(string(actionMode), payload)
}

// tryJSON mirrors `_try_json`: parse a JSON object out of a string, or
// nil if it is not one.
func tryJSON(text string) map[string]any {
	parsed, ok := jsonLoads(text)
	if !ok {
		return nil
	}
	data, _ := parsed.(map[string]any)
	return data
}

// jsonLoads mirrors Python `json.loads` (integral literals become int64).
func jsonLoads(text string) (any, bool) {
	dec := json.NewDecoder(strings.NewReader(text))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, false
	}
	// Trailing data is an error in Python
	if _, err := dec.Token(); err != io.EOF {
		return nil, false
	}
	return pyNumbers(v), true
}

func pyNumbers(v any) any {
	switch t := v.(type) {
	case json.Number:
		if !strings.ContainsAny(string(t), ".eE") {
			if i, err := strconv.ParseInt(string(t), 10, 64); err == nil {
				return i
			}
		}
		f, _ := t.Float64()
		return f
	case map[string]any:
		for k, item := range t {
			t[k] = pyNumbers(item)
		}
	case []any:
		for i, item := range t {
			t[i] = pyNumbers(item)
		}
	}
	return v
}

// ReadServiceResult mirrors `read_service_result`: read a service-run
// response into a result. services may be nil (Python `None`).
func ReadServiceResult(result any, services any) ARServiceResult {
	var data map[string]any
	switch v := result.(type) {
	case *string:
		if v == nil {
			return ARServiceResult{Success: false}
		}
		return ReadServiceResult(*v, services)
	case string:
		data = tryJSON(v)
		if data == nil {
			// Non-JSON 200 body: dispatched, but carries no timing details
			return ARServiceResult{Success: converters.PyStrip(v) != ""}
		}
	case map[string]any:
		data = v
	default:
		return ARServiceResult{Success: false}
	}

	neededTime := converters.RawToInt(data[RestartNeededTimeKey])
	lastID := converters.RawToInt(data[IDKey])
	if neededTime == nil && lastID != nil {
		t := defaultNeededTime
		neededTime = &t
	}

	var success bool
	ran := data[RunServiceKey]
	switch {
	case ran != nil && services != nil:
		requested := joinServices(asList(services))
		success = strings.Trim(converters.PyStr(ran), ";") == strings.Trim(requested, ";")
	case ran != nil:
		success = converters.PyStr(ran) != ""
	default:
		// A reachable JSON 200 without an echo is dispatched unless the
		// device explicitly reports no modification
		modify := converters.RawToBool(data[common.ModifyKey])
		success = modify == nil || *modify
	}

	return ARServiceResult{Success: success, NeededTime: neededTime, LastID: lastID}
}

// AsyncRunService mirrors `async_run_service`: post a service run and
// read the outcome. An empty actionMode is `ARActionMode.APPLY`;
// fetchRaw may be nil.
func AsyncRunService(
	ctx context.Context,
	callback registry.ReadFunc,
	services any,
	arguments [][2]any,
	actionMode common.ARActionMode,
	fetchRaw registry.FetchRawFunc,
) (ARServiceResult, error) {
	request := BuildServiceRequest(services, arguments, actionMode)
	// Post raw when possible: async_fetch preserves a failed fetch as None,
	// while async_read would collapse it to {} and hide the failure
	var data any
	if fetchRaw != nil {
		raw, err := fetchRaw(ctx, endpoint.AREndpointPushData, request)
		if err != nil {
			return ARServiceResult{}, err
		}
		data = raw
	} else {
		read, err := callback(ctx, endpoint.AREndpointPushData, request)
		if err != nil {
			return ARServiceResult{}, err
		}
		data = read
	}
	return ReadServiceResult(data, services), nil
}

// runAction mirrors `run_action`: run the action's service(s) and read
// the outcome.
func runAction(ctx context.Context, c *registry.Context, a action.ARAction) (any, error) {
	act, ok := a.(*ARServiceAction)
	if !ok || len(act.Services) == 0 {
		return ARServiceResult{Success: false}, nil
	}

	var arguments [][2]any
	if len(act.Arguments) > 0 {
		arguments = act.Arguments
	}
	result, err := AsyncRunService(ctx, c.Read, act.Services, arguments, common.ARActionModeApply, c.FetchRaw)
	if err != nil {
		return nil, err
	}

	// A reboot drops the session; flag it so the caller reconnects
	if result.Success && c.Identity != nil && triggersReboot(act.Services) {
		c.Identity.MarkReboot()
	}

	return result, nil
}

func init() {
	registry.RegisterAction(registry.ActionEntry{
		Name: "ARServiceAction",
		Run:  runAction,
	})
}
