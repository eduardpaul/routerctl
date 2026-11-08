package mqtt

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/eduardpaul/routerctl/routerctl/internal/service/commands"
	"github.com/eduardpaul/routerctl/routerctl/internal/service/poller"
	"github.com/eduardpaul/routerctl/routerctl/internal/service/view"
	"github.com/eduardpaul/routerctl/routerkit"
)

// Topics builds every topic name for one base topic.
//
//	<base>/status                         bridge availability (LWT), retained
//	<base>/<dev>/availability             device reachable: online|offline, retained
//	<base>/<dev>/identity                 identity JSON, retained
//	<base>/<dev>/state                    system/WAN/traffic/clients JSON, retained
//	<base>/<dev>/client/<macid>           client attributes JSON, retained
//	<base>/<dev>/client/<macid>/presence  home|not_home, retained
//	<base>/<dev>/node/<macid>             mesh node JSON, retained
//	<base>/<dev>/event/<type>             every event JSON (subscribe event/+)
//	<base>/<dev>/cmd/<action>             command (JSON args, "on"/"off", "k=v ...";
//	                                      a JSON "_id" is echoed in the result)
//	<base>/<dev>/cmd/<action>/result      commands.Result JSON
//	<base>/<dev>/switch/<object>          switch state on|off, retained
type Topics struct{ Base string }

// MacID is a MAC without separators (topic / id friendly).
func MacID(mac string) string { return strings.ReplaceAll(strings.ToLower(mac), ":", "") }

var unsafeRe = regexp.MustCompile(`[^a-z0-9_]+`)

// ObjectID sanitizes a string into an HA object id.
func ObjectID(s string) string {
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, ".", "_")
	s = unsafeRe.ReplaceAllString(s, "_")
	return strings.Trim(s, "_")
}

func (t Topics) Status() string                 { return t.Base + "/status" }
func (t Topics) Availability(dev string) string { return t.Base + "/" + dev + "/availability" }
func (t Topics) Identity(dev string) string     { return t.Base + "/" + dev + "/identity" }
func (t Topics) State(dev string) string        { return t.Base + "/" + dev + "/state" }
func (t Topics) Event(dev, typ string) string   { return t.Base + "/" + dev + "/event/" + typ }
func (t Topics) Client(dev, mac string) string  { return t.Base + "/" + dev + "/client/" + MacID(mac) }
func (t Topics) Presence(dev, mac string) string {
	return t.Client(dev, mac) + "/presence"
}
func (t Topics) Node(dev, mac string) string       { return t.Base + "/" + dev + "/node/" + MacID(mac) }
func (t Topics) Command(dev, action string) string { return t.Base + "/" + dev + "/cmd/" + action }
func (t Topics) CommandWildcard(dev string) string { return t.Base + "/" + dev + "/cmd/+" }
func (t Topics) Result(dev, action string) string  { return t.Command(dev, action) + "/result" }
func (t Topics) Switch(dev, object string) string  { return t.Base + "/" + dev + "/switch/" + object }

// Presence payloads.
const (
	Home    = "home"
	NotHome = "not_home"
)

// ---- Home Assistant discovery ----

// Options tune discovery.
type Options struct {
	Topics          Topics
	DiscoveryPrefix string
	// Trackers: known | all | none
	Trackers string
	Commands bool
	// Policy decides which actions command topics (and so HA buttons
	// and switches) may run; the zero Policy allows safe actions only
	Policy        commands.Policy
	BlockSwitches bool
	Version       string
	// QoS of every publish and subscription
	QoS byte
}

// Entity is one discovery config: topic and payload.
type Entity struct {
	Topic   string
	Payload map[string]any
}

// NodeID is the discovery node id of a device.
func NodeID(dev string) string { return "routerctl_" + ObjectID(dev) }

// tracked reports whether a client gets a device_tracker.
func tracked(o Options, st poller.DeviceState, mac string) bool {
	switch o.Trackers {
	case "all":
		return true
	case "none":
		return false
	}
	e, ok := st.Inventory[mac]
	return ok && (e.Known || e.Label != "")
}

// TrackedClients lists the clients that get trackers, sorted by MAC.
func TrackedClients(o Options, st poller.DeviceState) []routerkit.Client {
	var out []routerkit.Client
	for _, c := range st.Snapshot.Clients {
		if tracked(o, st, c.MAC) {
			out = append(out, c)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].MAC < out[j].MAC })
	return out
}

// switchSpec is one switch / button derived from a driver action.
type switchSpec struct {
	Object string
	Name   string
	Action string
	// Fixed args (besides the toggle)
	Args map[string]string
	// Toggle is the bool arg the switch sets ("state")
	Toggle    string
	Button    bool
	Dangerous bool
}

var titleWords = strings.NewReplacer("-", " ", "_", " ")

func titleCase(s string) string {
	s = titleWords.Replace(s)
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// actionEntities maps driver actions to switches and buttons:
//   - an action with a Toggle (bool) arg and no other required arg is a
//     switch; with one other required enum arg, one switch per choice
//     (wifi band=2g1|5g1 ...)
//   - an action with no required args is a button
//
// Actions with free-form required args (mac=..., name=...) get no entity
// (except per-client block switches, see Options.BlockSwitches).
// Dangerous (Destructive) actions need allowDangerous.
func actionEntities(specs []routerkit.ActionSpec, pol commands.Policy) []switchSpec {
	var out []switchSpec
	for _, a := range specs {
		dangerous := a.IsDangerous()
		if pol.Check(a) != nil {
			continue
		}
		var toggle *routerkit.Arg
		var others []routerkit.Arg
		for i := range a.Args {
			if a.Toggle != "" && a.Args[i].Name == a.Toggle && a.Args[i].Type == routerkit.ArgBool {
				toggle = &a.Args[i]
			} else if a.Args[i].Required {
				others = append(others, a.Args[i])
			}
		}
		switch {
		case toggle != nil && len(others) == 0:
			out = append(out, switchSpec{Object: ObjectID(a.Name), Name: titleCase(a.Name), Action: a.Name, Toggle: toggle.Name, Dangerous: dangerous})
		case toggle != nil && len(others) == 1 && others[0].Type == routerkit.ArgEnum && len(others[0].Choices) > 0:
			for _, ch := range others[0].Choices {
				out = append(out, switchSpec{
					Object: ObjectID(a.Name + "_" + ch), Name: titleCase(a.Name) + " " + ch, Action: a.Name,
					Args: map[string]string{others[0].Name: ch}, Toggle: toggle.Name, Dangerous: dangerous,
				})
			}
		case toggle == nil && len(others) == 0:
			out = append(out, switchSpec{Object: ObjectID(a.Name), Name: titleCase(a.Name), Action: a.Name, Button: true, Dangerous: dangerous})
		}
	}
	return out
}

// SwitchObject returns the switch object id a command maps to ("" when
// the command is not a switch), so the state topic can be updated.
func SwitchObject(specs []routerkit.ActionSpec, action string, args map[string]string) string {
	if mac := routerkit.NormalizeMAC(args["mac"]); mac != "" && action == "block-client" {
		return "block_" + MacID(mac)
	}
	for _, s := range actionEntities(specs, commands.Policy{MaxDanger: routerkit.Destructive}) {
		if s.Button || s.Action != action {
			continue
		}
		match := true
		for k, v := range s.Args {
			if !strings.EqualFold(args[k], v) {
				match = false
			}
		}
		if match {
			return s.Object
		}
	}
	return ""
}

func commandTemplate(fixed map[string]string, toggle string) string {
	keys := make([]string, 0, len(fixed))
	for k := range fixed {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	b.WriteByte('{')
	for _, k := range keys {
		kb, _ := json.Marshal(k)
		vb, _ := json.Marshal(fixed[k])
		fmt.Fprintf(&b, "%s:%s,", kb, vb)
	}
	tb, _ := json.Marshal(toggle)
	fmt.Fprintf(&b, `%s:"{{ value }}"}`, tb)
	return b.String()
}

// Discovery builds every discovery config of a device, keyed by topic.
// Only the parts the device supports (Capabilities) get entities.
func Discovery(o Options, st poller.DeviceState) map[string]map[string]any {
	t := o.Topics
	dev := st.Name
	node := NodeID(dev)
	id := st.Identity
	device := map[string]any{
		"identifiers":  []string{node},
		"name":         dev,
		"manufacturer": manufacturer(id.Vendor, st.Driver),
	}
	if id.Model != "" {
		device["model"] = id.Model
	}
	if id.Firmware != "" {
		device["sw_version"] = id.Firmware
	}
	if id.Serial != "" {
		device["serial_number"] = id.Serial
	}
	if id.MAC != "" {
		device["connections"] = [][]string{{"mac", id.MAC}}
	}
	origin := map[string]any{"name": "routerctl"}
	if o.Version != "" {
		origin["sw_version"] = o.Version
	}
	avail := []map[string]string{{"topic": t.Status()}, {"topic": t.Availability(dev)}}

	out := map[string]map[string]any{}
	add := func(component, object string, p map[string]any, dev map[string]any) {
		p["unique_id"] = node + "_" + object
		p["availability"] = avail
		p["availability_mode"] = "all"
		p["device"] = dev
		p["origin"] = origin
		out[o.DiscoveryPrefix+"/"+component+"/"+node+"/"+object+"/config"] = p
	}
	state := t.State(dev)
	s := st.Snapshot

	// System
	if s.System != nil && st.Supports(routerkit.PartSystem) {
		if s.System.CPUUsage != nil {
			add("sensor", "cpu_usage", map[string]any{"name": "CPU usage", "state_topic": state, "value_template": "{{ value_json.cpu_usage }}", "unit_of_measurement": "%", "state_class": "measurement", "icon": "mdi:cpu-64-bit"}, device)
		}
		if s.System.RAMUsage != nil {
			add("sensor", "ram_usage", map[string]any{"name": "RAM usage", "state_topic": state, "value_template": "{{ value_json.ram_usage }}", "unit_of_measurement": "%", "state_class": "measurement", "icon": "mdi:memory"}, device)
		}
		sensors := make([]string, 0, len(s.System.Temperatures))
		for k := range s.System.Temperatures {
			sensors = append(sensors, k)
		}
		sort.Strings(sensors)
		for _, k := range sensors {
			kb, _ := json.Marshal(k)
			add("sensor", "temperature_"+ObjectID(k), map[string]any{
				"name": "Temperature " + k, "state_topic": state,
				"value_template":      fmt.Sprintf("{{ (value_json.temperature or {}).get(%s) }}", kb),
				"unit_of_measurement": "°C", "device_class": "temperature", "state_class": "measurement",
			}, device)
		}
		if s.System.Uptime != nil {
			add("sensor", "uptime", map[string]any{"name": "Uptime", "state_topic": state, "value_template": "{{ value_json.uptime }}", "unit_of_measurement": "s", "device_class": "duration", "entity_category": "diagnostic"}, device)
		}
	}
	// WAN
	if s.WAN != nil && st.Supports(routerkit.PartWAN) {
		if s.WAN.Connected != nil {
			add("binary_sensor", "wan", map[string]any{"name": "WAN", "state_topic": state, "value_template": "{{ 'ON' if value_json.wan_connected else 'OFF' }}", "device_class": "connectivity"}, device)
		}
		add("sensor", "wan_ip", map[string]any{"name": "WAN IP", "state_topic": state, "value_template": "{{ value_json.wan_ip }}", "icon": "mdi:ip-network"}, device)
		if s.WAN.PublicIP != "" {
			add("sensor", "public_ip", map[string]any{"name": "Public IP", "state_topic": state, "value_template": "{{ value_json.public_ip }}", "icon": "mdi:earth"}, device)
		}
	}
	// Traffic (uplink)
	if w := view.WANInterface(s.Traffic); w != nil && st.Supports(routerkit.PartTraffic) {
		if w.RxRate != nil {
			add("sensor", "download_rate", map[string]any{"name": "Download rate", "state_topic": state, "value_template": "{{ value_json.rx_rate | round(0) }}", "unit_of_measurement": "B/s", "device_class": "data_rate", "state_class": "measurement"}, device)
		}
		if w.TxRate != nil {
			add("sensor", "upload_rate", map[string]any{"name": "Upload rate", "state_topic": state, "value_template": "{{ value_json.tx_rate | round(0) }}", "unit_of_measurement": "B/s", "device_class": "data_rate", "state_class": "measurement"}, device)
		}
		if w.RxBytes != nil {
			add("sensor", "downloaded", map[string]any{"name": "Downloaded", "state_topic": state, "value_template": "{{ value_json.rx_bytes }}", "unit_of_measurement": "B", "device_class": "data_size", "state_class": "total_increasing"}, device)
		}
		if w.TxBytes != nil {
			add("sensor", "uploaded", map[string]any{"name": "Uploaded", "state_topic": state, "value_template": "{{ value_json.tx_bytes }}", "unit_of_measurement": "B", "device_class": "data_size", "state_class": "total_increasing"}, device)
		}
	}
	// Clients
	if s.Clients != nil && st.Supports(routerkit.PartClients) {
		add("sensor", "clients_online", map[string]any{"name": "Clients online", "state_topic": state, "value_template": "{{ value_json.clients_online }}", "state_class": "measurement", "icon": "mdi:devices"}, device)
	}
	// Firmware
	if s.Firmware != nil && st.Supports(routerkit.PartFirmware) {
		if s.Firmware.UpdateAvailable != nil {
			add("binary_sensor", "firmware_update", map[string]any{"name": "Firmware update", "state_topic": state, "value_template": "{{ 'ON' if value_json.firmware_update else 'OFF' }}", "device_class": "update", "entity_category": "diagnostic"}, device)
		}
		add("sensor", "firmware_available", map[string]any{"name": "Firmware available", "state_topic": state, "value_template": "{{ value_json.firmware_available | default('', true) }}", "icon": "mdi:package-up", "entity_category": "diagnostic"}, device)
	}
	// Mesh nodes
	nodes := s.Nodes
	if !st.Supports(routerkit.PartNodes) {
		nodes = nil
	}
	for _, n := range nodes {
		name := n.Name
		if name == "" {
			name = n.MAC
		}
		add("binary_sensor", "node_"+MacID(n.MAC), map[string]any{
			"name": "Node " + name, "state_topic": t.Node(dev, n.MAC), "json_attributes_topic": t.Node(dev, n.MAC),
			"value_template": "{{ 'ON' if value_json.online else 'OFF' }}", "device_class": "connectivity",
		}, device)
	}

	// Device trackers (each client is its own HA device, via the router)
	acts := st.Capabilities.Actions
	blockSpec, canBlock := routerkit.FindAction(acts, routerkit.ActBlockClient)
	canBlock = canBlock && blockSpec.Toggle != ""
	for _, c := range TrackedClients(o, st) {
		cp := view.BuildClient(st, c)
		cdev := map[string]any{
			"identifiers": []string{"routerctl_client_" + MacID(c.MAC)},
			"name":        cp.Name,
			"connections": [][]string{{"mac", c.MAC}},
			"via_device":  node,
		}
		if c.Vendor != "" {
			cdev["manufacturer"] = c.Vendor
		}
		add("device_tracker", "client_"+MacID(c.MAC), map[string]any{
			"name": nil, "state_topic": t.Presence(dev, c.MAC), "json_attributes_topic": t.Client(dev, c.MAC),
			"payload_home": Home, "payload_not_home": NotHome, "source_type": "router", "icon": "mdi:lan-connect",
		}, cdev)
		if o.Commands && o.BlockSwitches && canBlock {
			obj := "block_" + MacID(c.MAC)
			add("switch", obj, map[string]any{
				"name": "Block internet", "command_topic": t.Command(dev, "block-client"),
				"command_template": commandTemplate(map[string]string{"mac": c.MAC}, blockSpec.Toggle),
				"payload_on":       "on", "payload_off": "off", "state_topic": t.Switch(dev, obj),
				"state_on": "on", "state_off": "off", "icon": "mdi:web-cancel",
			}, cdev)
		}
	}

	// Switches and buttons from actions
	if o.Commands {
		for _, sw := range actionEntities(acts, o.Policy) {
			if sw.Button {
				p := map[string]any{"name": sw.Name, "command_topic": t.Command(dev, sw.Action), "payload_press": "PRESS"}
				if sw.Action == "reboot" {
					p["device_class"] = "restart"
				}
				if sw.Dangerous {
					p["entity_category"] = "config"
				}
				add("button", sw.Object, p, device)
				continue
			}
			add("switch", sw.Object, map[string]any{
				"name": sw.Name, "command_topic": t.Command(dev, sw.Action),
				"command_template": commandTemplate(sw.Args, sw.Toggle),
				"payload_on":       "on", "payload_off": "off",
				"state_topic": t.Switch(dev, sw.Object), "state_on": "on", "state_off": "off",
			}, device)
		}
	}
	return out
}

// manufacturer is the identity vendor (DriverInfo.Vendor), or the driver
// name before the identity is known.
func manufacturer(vendor, drv string) string {
	if vendor == "" {
		return drv
	}
	return vendor
}
