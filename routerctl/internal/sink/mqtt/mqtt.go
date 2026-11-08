// Package mqtt is the MQTT sink: retained state topics per device and
// per client (presence), events, Home Assistant discovery and command
// topics mapped to driver actions (through service/commands, like every
// transport). See topics.go for the layout.
package mqtt

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	paho "github.com/eclipse/paho.mqtt.golang"

	"github.com/eduardpaul/routerctl/routerctl/internal/service/commands"
	"github.com/eduardpaul/routerctl/routerctl/internal/service/events"
	"github.com/eduardpaul/routerctl/routerctl/internal/service/poller"
	"github.com/eduardpaul/routerctl/routerctl/internal/service/view"
)

// Client is the subset of an MQTT client the sink uses (tests use a fake).
type Client interface {
	// Connect connects; onConnect runs after every (re)connection
	Connect(onConnect func()) error
	Publish(topic string, qos byte, retained bool, payload []byte) error
	Subscribe(topic string, qos byte, handler func(topic string, payload []byte)) error
	Disconnect()
}

// Sink is the MQTT sink.
type Sink struct {
	opts   Options
	qos    byte
	client Client
	ctl    poller.Control
	ctx    context.Context
	cancel context.CancelFunc

	mu sync.Mutex
	// last published retained payloads (dedup) by topic
	last map[string][]byte
	// discovery topics we believe exist on the broker, per device
	disc map[string]map[string]bool
	// latest state per device (for republish and commands)
	states map[string]poller.DeviceState
	// client topics published per device (to clear vanished ones)
	clientTopics map[string]map[string]bool
	subscribed   map[string]bool
	wg           sync.WaitGroup
}

// New builds the sink over a client. An empty o.DiscoveryPrefix turns
// Home Assistant discovery off.
func New(o Options, client Client) *Sink {
	return &Sink{
		opts:         o,
		qos:          o.QoS,
		client:       client,
		last:         map[string][]byte{},
		disc:         map[string]map[string]bool{},
		states:       map[string]poller.DeviceState{},
		clientTopics: map[string]map[string]bool{},
		subscribed:   map[string]bool{},
	}
}

// discoveryEnabled is false when the prefix is cleared (discovery: false).
func (s *Sink) discoveryEnabled() bool { return s.opts.DiscoveryPrefix != "" }

// Name implements poller.Sink.
func (s *Sink) Name() string { return "mqtt" }

// Start connects and subscribes.
func (s *Sink) Start(ctx context.Context, ctl poller.Control) error {
	s.ctl = ctl
	s.ctx, s.cancel = context.WithCancel(context.Background())
	return s.client.Connect(s.onConnect)
}

func (s *Sink) onConnect() {
	t := s.opts.Topics
	_ = s.client.Publish(t.Status(), 1, true, []byte("online"))
	s.mu.Lock()
	s.subscribed = map[string]bool{}
	devs := make([]string, 0, len(s.states))
	for d := range s.states {
		devs = append(devs, d)
	}
	s.mu.Unlock()
	if s.discoveryEnabled() {
		// HA restarted: push everything again
		_ = s.client.Subscribe(s.opts.DiscoveryPrefix+"/status", 0, func(_ string, p []byte) {
			if string(p) == "online" {
				s.republish()
			}
		})
	}
	for _, d := range devs {
		s.subscribeDevice(d)
	}
	s.republish()
}

func (s *Sink) subscribeDevice(dev string) {
	s.mu.Lock()
	if s.subscribed[dev] {
		s.mu.Unlock()
		return
	}
	s.subscribed[dev] = true
	s.mu.Unlock()
	if s.opts.Commands {
		if err := s.client.Subscribe(s.opts.Topics.CommandWildcard(dev), s.qos, s.handleCommand(dev)); err != nil {
			slog.Warn("mqtt subscribe failed", "device", dev, "err", err)
		}
	}
	if s.discoveryEnabled() {
		// Learn retained discovery configs left from earlier runs, so
		// entities that are no longer wanted get cleared
		pattern := s.opts.DiscoveryPrefix + "/+/" + NodeID(dev) + "/+/config"
		_ = s.client.Subscribe(pattern, 0, func(topic string, p []byte) {
			s.mu.Lock()
			defer s.mu.Unlock()
			if s.disc[dev] == nil {
				s.disc[dev] = map[string]bool{}
			}
			if len(p) == 0 {
				delete(s.disc[dev], topic)
			} else if _, ours := s.last[topic]; !ours {
				s.disc[dev][topic] = true
			}
		})
	}
}

// republish forgets the dedup cache and publishes every state again.
func (s *Sink) republish() {
	s.mu.Lock()
	s.last = map[string][]byte{}
	states := make([]poller.DeviceState, 0, len(s.states))
	for _, st := range s.states {
		states = append(states, st)
	}
	s.mu.Unlock()
	sort.Slice(states, func(i, j int) bool { return states[i].Name < states[j].Name })
	for _, st := range states {
		s.publishState(st)
	}
}

// publish sends a payload; retained payloads equal to the last one are
// skipped.
func (s *Sink) publish(topic string, retained bool, payload []byte) {
	if retained {
		s.mu.Lock()
		prev, ok := s.last[topic]
		if ok && bytes.Equal(prev, payload) {
			s.mu.Unlock()
			return
		}
		s.last[topic] = payload
		s.mu.Unlock()
	}
	if err := s.client.Publish(topic, s.qos, retained, payload); err != nil {
		slog.Debug("mqtt publish failed", "topic", topic, "err", err)
		s.mu.Lock()
		delete(s.last, topic)
		s.mu.Unlock()
	}
}

func (s *Sink) publishJSON(topic string, retained bool, v any) {
	b, err := json.Marshal(v)
	if err != nil {
		slog.Warn("mqtt: cannot encode payload", "topic", topic, "err", err)
		return
	}
	s.publish(topic, retained, b)
}

// State implements poller.Sink.
func (s *Sink) State(st poller.DeviceState) {
	s.mu.Lock()
	s.states[st.Name] = st
	s.mu.Unlock()
	s.subscribeDevice(st.Name)
	s.publishState(st)
}

func (s *Sink) publishState(st poller.DeviceState) {
	t := s.opts.Topics
	dev := st.Name
	avail := "offline"
	if st.Online {
		avail = "online"
	}
	s.publish(t.Availability(dev), true, []byte(avail))
	if st.Identity.Vendor != "" || st.Identity.Model != "" {
		s.publishJSON(t.Identity(dev), true, st.Identity)
	}
	if !st.LastPoll.IsZero() {
		// last_poll changes every time; publish without dedup noise
		s.publishJSON(t.State(dev), true, view.BuildState(st))
	}

	// Clients: attributes + presence for every client in the list
	want := map[string]bool{}
	for _, c := range st.Snapshot.Clients {
		cp := view.BuildClient(st, c)
		pres := NotHome
		if cp.Present {
			pres = Home
		}
		s.publishJSON(t.Client(dev, c.MAC), true, cp)
		s.publish(t.Presence(dev, c.MAC), true, []byte(pres))
		want[t.Client(dev, c.MAC)] = true
		want[t.Presence(dev, c.MAC)] = true
	}
	s.mu.Lock()
	prev := s.clientTopics[dev]
	if st.Snapshot.Clients != nil {
		s.clientTopics[dev] = want
	}
	s.mu.Unlock()
	if st.Snapshot.Clients != nil {
		for topic := range prev {
			if !want[topic] {
				s.clear(topic)
			}
		}
	}
	for _, n := range st.Snapshot.Nodes {
		s.publishJSON(t.Node(dev, n.MAC), true, n)
	}

	if s.discoveryEnabled() {
		s.syncDiscovery(st)
	}
}

// clear removes a retained topic.
func (s *Sink) clear(topic string) {
	s.mu.Lock()
	delete(s.last, topic)
	s.mu.Unlock()
	_ = s.client.Publish(topic, s.qos, true, nil)
}

func (s *Sink) syncDiscovery(st poller.DeviceState) {
	desired := Discovery(s.opts, st)
	topics := make([]string, 0, len(desired))
	for topic := range desired {
		topics = append(topics, topic)
	}
	sort.Strings(topics)
	for _, topic := range topics {
		s.publishJSON(topic, true, desired[topic])
	}
	s.mu.Lock()
	known := s.disc[st.Name]
	next := map[string]bool{}
	var stale []string
	for topic := range known {
		if _, ok := desired[topic]; !ok {
			stale = append(stale, topic)
		}
	}
	for topic := range desired {
		next[topic] = true
	}
	s.disc[st.Name] = next
	s.mu.Unlock()
	sort.Strings(stale)
	for _, topic := range stale {
		s.clear(topic)
	}
}

// Event implements poller.Sink.
func (s *Sink) Event(e events.Event) {
	if e.Device == "" {
		return
	}
	s.publishJSON(s.opts.Topics.Event(e.Device, string(e.Type)), false, e)
}

// Close implements poller.Sink: marks the bridge offline and disconnects.
func (s *Sink) Close(ctx context.Context) error {
	if s.cancel != nil {
		s.cancel()
	}
	done := make(chan struct{})
	go func() { s.wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-ctx.Done():
	}
	_ = s.client.Publish(s.opts.Topics.Status(), 1, true, []byte("offline"))
	s.client.Disconnect()
	return nil
}

// ---- commands ----

func (s *Sink) handleCommand(dev string) func(topic string, payload []byte) {
	return func(topic string, payload []byte) {
		action := topic[strings.LastIndex(topic, "/")+1:]
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			s.runCommand(dev, action, payload)
		}()
	}
}

func (s *Sink) runCommand(dev, action string, payload []byte) {
	t := s.opts.Topics
	res := commands.Run(s.ctx, commands.FromControl(s.ctl), s.opts.Policy, commands.Request{
		Device: dev, Action: action, Payload: payload, Source: "mqtt",
	})
	// The result goes out last: whoever reacts to it sees the switch
	// state already updated
	defer s.publishJSON(t.Result(dev, action), false, res)
	if res.Error != nil {
		if res.Error.Code == commands.CodeForbidden {
			slog.Warn("mqtt: command refused by the policy", "device", dev, "action", action, "reason", res.Error.Message)
		} else {
			slog.Warn("mqtt command failed", "device", dev, "action", action, "err", res.Error.Message)
		}
		return
	}
	slog.Info("mqtt command", "device", dev, "action", action, "args", res.Args)
	st, ok := s.ctl.State(dev)
	if !ok {
		return
	}
	spec, _ := st.Capabilities.Action(action)
	if obj := SwitchObject(st.Capabilities.Actions, action, res.Args); obj != "" && spec.Toggle != "" {
		if v, ok := res.Args[spec.Toggle]; ok {
			s.publish(t.Switch(dev, obj), true, []byte(v))
		}
	}
}

// ---- paho adapter ----

// Broker is how the paho client connects.
type Broker struct {
	URL      string // tcp://host:1883, ssl://host:8883, ws://...
	Username string
	Password string
	// ClientID: "" picks a random routerctl-<n>
	ClientID string
	// WillTopic receives a retained "offline" when the connection drops
	WillTopic string
	// Insecure skips the broker certificate verification
	Insecure bool
}

// String masks the password.
func (b Broker) String() string {
	pw := ""
	if b.Password != "" {
		pw = "********"
	}
	return fmt.Sprintf("{URL:%s Username:%s Password:%s ClientID:%s WillTopic:%s Insecure:%t}",
		b.URL, b.Username, pw, b.ClientID, b.WillTopic, b.Insecure)
}

// GoString masks the password.
func (b Broker) GoString() string { return "mqtt.Broker" + b.String() }

type pahoClient struct {
	cfg Broker
	c   paho.Client
}

// NewPaho builds a Client over paho.mqtt.golang.
func NewPaho(b Broker) Client { return &pahoClient{cfg: b} }

func (p *pahoClient) Connect(onConnect func()) error {
	c := p.cfg
	id := c.ClientID
	if id == "" {
		id = "routerctl-" + strconv.FormatInt(time.Now().UnixNano()%1e6, 36)
	}
	o := paho.NewClientOptions().
		AddBroker(c.URL).
		SetClientID(id).
		SetUsername(c.Username).
		SetPassword(c.Password).
		SetAutoReconnect(true).
		SetConnectRetry(true).
		SetConnectRetryInterval(5*time.Second).
		SetMaxReconnectInterval(time.Minute).
		SetKeepAlive(30*time.Second).
		SetCleanSession(true).
		SetOrderMatters(false).
		SetWill(c.WillTopic, "offline", 1, true).
		SetOnConnectHandler(func(paho.Client) {
			slog.Info("mqtt connected", "broker", c.URL)
			go onConnect()
		}).
		SetConnectionLostHandler(func(_ paho.Client, err error) {
			slog.Warn("mqtt connection lost", "err", err)
		})
	if c.Insecure {
		o.SetTLSConfig(&tls.Config{InsecureSkipVerify: true}) //nolint:gosec // opt-in
	}
	p.c = paho.NewClient(o)
	tok := p.c.Connect()
	// With ConnectRetry the token completes on the first success; do not
	// block startup on an unreachable broker
	if tok.WaitTimeout(10*time.Second) && tok.Error() != nil {
		return fmt.Errorf("mqtt connect: %w", tok.Error())
	}
	return nil
}

func (p *pahoClient) Publish(topic string, qos byte, retained bool, payload []byte) error {
	if p.c == nil || !p.c.IsConnectionOpen() {
		return errors.New("not connected")
	}
	tok := p.c.Publish(topic, qos, retained, payload)
	if !tok.WaitTimeout(10 * time.Second) {
		return errors.New("publish timeout")
	}
	return tok.Error()
}

func (p *pahoClient) Subscribe(topic string, qos byte, h func(string, []byte)) error {
	if p.c == nil {
		return errors.New("not connected")
	}
	tok := p.c.Subscribe(topic, qos, func(_ paho.Client, m paho.Message) { h(m.Topic(), m.Payload()) })
	if !tok.WaitTimeout(10 * time.Second) {
		return errors.New("subscribe timeout")
	}
	return tok.Error()
}

func (p *pahoClient) Disconnect() {
	if p.c != nil {
		p.c.Disconnect(500)
	}
}
