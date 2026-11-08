package poller

// The port between the poller and its outputs (MQTT, notifications,
// metrics, stdout): what the daemon feeds every sink after each poll, and
// how a sink runs commands. The sinks (internal/sink/*) implement it; the
// poller never imports them.

import (
	"context"
	"errors"
	"time"

	"github.com/eduardpaul/routerctl/routerctl/internal/service/events"
	"github.com/eduardpaul/routerctl/routerctl/internal/service/inventory"
	"github.com/eduardpaul/routerctl/routerkit"
)

// DeviceState is the merged, latest view of one configured device.
type DeviceState struct {
	// Name is the configured device name, Driver the driver name
	Name     string
	Driver   string
	Identity routerkit.Identity
	// Capabilities of the device (read after each Connect); its Actions
	// are what command topics may run
	Capabilities routerkit.Capabilities
	// Online: the last poll reached the device
	Online    bool
	LastPoll  time.Time
	LastError string
	// Snapshot holds the latest value of every part (parts are polled
	// on different intervals); Snapshot.Errors are the last part errors
	Snapshot routerkit.Snapshot
	// Presence is the debounced online state by client MAC
	Presence map[string]bool
	// Inventory is a copy of the inventory at the time of the state
	Inventory inventory.Map

	// Counters since start
	Polls        uint64
	PollFailures uint64
	PartErrors   map[routerkit.Part]uint64
	PollDuration time.Duration
}

// Supports reports whether the device supports part p (true while the
// capabilities are unknown, so outputs fall back to the data present).
func (st DeviceState) Supports(p routerkit.Part) bool {
	return len(st.Capabilities.Parts) == 0 || st.Capabilities.Supports(p)
}

// Control is what a transport may ask the daemon: the read side (devices,
// their state, native sources), actions and inventory edits. Every
// device operation goes through the daemon, which owns the session
// (Capabilities.SingleSession devices allow one).
type Control interface {
	// Devices lists the configured device names, sorted
	Devices() []string
	State(device string) (DeviceState, bool)
	// Do runs a bound invocation (service/commands binds it and applies
	// the danger policy); a result the device did not accept is an error
	// (routerkit.ErrRejected). Every call emits an action_done event.
	Do(ctx context.Context, device string, inv routerkit.Invocation) (routerkit.Result, error)
	// Get reads a native source of a device
	Get(ctx context.Context, device, source string) (any, error)
	Inventory() Inventory
}

// Inventory is the inventory as transports see it: reads, and edits
// that the daemon saves to the state file at once.
type Inventory interface {
	List() []inventory.Entry
	Get(mac string) (inventory.Entry, bool)
	// Label sets the user label (and marks the device known); "" clears it
	Label(mac, label string) error
	SetKnown(mac string, known bool) error
	Forget(mac string) error
}

// ErrUnknownDevice: no configured device has that name.
var ErrUnknownDevice = errors.New("unknown device")

// ErrNotReady: the device has not connected yet (no capabilities).
var ErrNotReady = errors.New("device not ready (not connected yet)")

type sourceKey struct{}

// WithSource tags ctx with the transport that runs an action ("mqtt",
// "api", "cli"); it is reported in the action_done event.
func WithSource(ctx context.Context, source string) context.Context {
	return context.WithValue(ctx, sourceKey{}, source)
}

func sourceOf(ctx context.Context) string {
	s, _ := ctx.Value(sourceKey{}).(string)
	return s
}

type requestKey struct{}

// WithRequestID tags ctx with the caller's request id (MQTT "_id", the
// API X-Request-Id); it is reported in the action_done event.
func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, requestKey{}, id)
}

func requestOf(ctx context.Context) string {
	s, _ := ctx.Value(requestKey{}).(string)
	return s
}

// Sink is one output.
type Sink interface {
	Name() string
	// Start is called once before any State/Event
	Start(ctx context.Context, ctl Control) error
	State(st DeviceState)
	Event(e events.Event)
	Close(ctx context.Context) error
}
