package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/eduardpaul/routerctl/routerctl/internal/apiclient"
	"github.com/eduardpaul/routerctl/routerctl/internal/config"
	"github.com/eduardpaul/routerctl/routerctl/internal/output"
	cmdsvc "github.com/eduardpaul/routerctl/routerctl/internal/service/commands"
	"github.com/eduardpaul/routerctl/routerctl/internal/service/inventory"
	"github.com/eduardpaul/routerctl/routerctl/internal/service/view"
	"github.com/eduardpaul/routerctl/routerctl/internal/sink/api"
	"github.com/eduardpaul/routerctl/routerkit"
	"github.com/eduardpaul/routerctl/routerkit/detect"
)

func cmdDetect(ctx context.Context, args []string) error {
	fs := newFlagSet("detect", "<host> [--ssl] [--port N] [-v]")
	verbose := fs.Bool("v", false, "list every candidate driver with its score and reason")
	ssl := fs.Bool("ssl", false, "use HTTPS")
	port := fs.Int("port", 0, "port")
	timeout := fs.Duration("timeout", 10*time.Second, "timeout")
	out := fs.String("output", "table", "output: table | json")
	fs.StringVar(out, "o", "table", "output (shorthand)")
	pos, err := parse(fs, args)
	if err != nil {
		return err
	}
	if len(pos) != 1 {
		return usagef("want exactly one host")
	}
	f, err := output.ParseFormat(*out)
	if err != nil {
		return err
	}
	setupLogging("", "warn", "text")
	res, perr := detect.Probe(ctx, registry(), pos[0], *ssl, *port, *timeout)
	rows := output.Rows{Header: []string{"field", "value"}, Rows: [][]string{
		{"url", res.URL}, {"status", strconv.Itoa(res.Status)}, {"title", res.Title}, {"server", res.Server}, {"driver", res.Driver},
	}}
	if res.Error != "" {
		rows.Rows = append(rows.Rows, []string{"error", res.Error})
	}
	if *verbose {
		for _, c := range res.Candidates {
			rows.Rows = append(rows.Rows, []string{"candidate", c.String()})
		}
	} else {
		res.Candidates = nil
	}
	if err := output.Write(os.Stdout, f, res, rows); err != nil {
		return err
	}
	return perr
}

func cmdSources(ctx context.Context, args []string) error {
	fs := newFlagSet("sources", "")
	df := addDevFlags(fs, true)
	if _, err := parse(fs, args); err != nil {
		return err
	}
	f, err := df.format()
	if err != nil {
		return err
	}
	drv, _, _, err := df.connect(ctx)
	if err != nil {
		return err
	}
	defer closeQuiet(drv)
	caps, err := drv.Capabilities(ctx)
	if err != nil {
		return err
	}
	specs := caps.Sources
	rows := output.Rows{Header: []string{"source", "help"}}
	for _, s := range specs {
		rows.Rows = append(rows.Rows, []string{s.Name, s.Help})
	}
	return output.Write(os.Stdout, f, specs, rows)
}

func cmdGet(ctx context.Context, args []string) error {
	fs := newFlagSet("get", "<source>... [-o json|table|csv]")
	df := addDevFlags(fs, true)
	pos, err := parse(fs, args)
	if err != nil {
		return err
	}
	if len(pos) == 0 {
		return usagef("name at least one source (see `routerctl sources`)")
	}
	// get defaults to JSON (native data is nested)
	if !flagSet(fs, "o") && !flagSet(fs, "output") {
		df.output = "json"
	}
	f, err := df.format()
	if err != nil {
		return err
	}
	drv, _, _, err := df.connect(ctx)
	if err != nil {
		return err
	}
	defer closeQuiet(drv)
	results := map[string]any{}
	var errs []error
	for _, name := range pos {
		v, err := drv.Get(ctx, name)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", name, err))
			continue
		}
		results[name] = v
	}
	var v any = results
	if len(pos) == 1 {
		v = results[pos[0]]
	}
	if len(results) > 0 {
		if err := output.Write(os.Stdout, f, v, output.Tabulate(v)); err != nil {
			return err
		}
	}
	return errors.Join(errs...)
}

func flagSet(fs *flag.FlagSet, name string) bool {
	found := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == name {
			found = true
		}
	})
	return found
}

func fmtPtr[T any](p *T) string {
	if p == nil {
		return ""
	}
	return fmt.Sprint(*p)
}

func fmtFloat(p *float64, unit string) string {
	if p == nil {
		return ""
	}
	return strconv.FormatFloat(*p, 'f', -1, 64) + unit
}

func fmtUptime(p *int64) string {
	if p == nil {
		return ""
	}
	d := time.Duration(*p) * time.Second
	days := int(d.Hours()) / 24
	rest := d - time.Duration(days)*24*time.Hour
	if days > 0 {
		return fmt.Sprintf("%dd %s", days, rest.Round(time.Second))
	}
	return rest.Round(time.Second).String()
}

type statusView struct {
	Device   string                                 `json:"device"`
	Identity routerkit.Identity                     `json:"identity"`
	WAN      *routerkit.WAN                         `json:"wan,omitempty"`
	System   *routerkit.System                      `json:"system,omitempty"`
	Firmware *routerkit.Firmware                    `json:"firmware,omitempty"`
	Errors   map[routerkit.Part]routerkit.PartError `json:"errors,omitempty"`
}

func cmdStatus(ctx context.Context, args []string) error {
	fs := newFlagSet("status", "")
	df := addDevFlags(fs, true)
	if _, err := parse(fs, args); err != nil {
		return err
	}
	f, err := df.format()
	if err != nil {
		return err
	}
	drv, _, dev, err := df.connect(ctx)
	if err != nil {
		return err
	}
	defer closeQuiet(drv)
	id, err := drv.Identity(ctx)
	if err != nil {
		return err
	}
	snap, err := drv.Poll(ctx, routerkit.Poll(routerkit.PartWAN, routerkit.PartSystem, routerkit.PartFirmware))
	if err != nil {
		return err
	}
	v := statusView{Device: dev.Name, Identity: id, WAN: snap.WAN, System: snap.System, Firmware: snap.Firmware, Errors: snap.Errors}
	rows := output.Rows{Header: []string{"field", "value"}}
	add := func(k, val string) {
		if val != "" {
			rows.Rows = append(rows.Rows, []string{k, val})
		}
	}
	add("device", dev.Name)
	add("vendor", id.Vendor)
	add("model", id.Model)
	add("firmware", id.Firmware)
	add("serial", id.Serial)
	add("mac", id.MAC)
	if w := snap.WAN; w != nil {
		add("wan.connected", fmtPtr(w.Connected))
		add("wan.ip", w.IP)
		add("wan.public_ip", w.PublicIP)
		add("wan.gateway", w.Gateway)
		add("wan.dns", strings.Join(w.DNS, ", "))
		add("wan.protocol", w.Protocol)
		add("wan.uptime", fmtUptime(w.Uptime))
	}
	if s := snap.System; s != nil {
		add("cpu", fmtFloat(s.CPUUsage, "%"))
		add("ram", fmtFloat(s.RAMUsage, "%"))
		keys := make([]string, 0, len(s.Temperatures))
		for k := range s.Temperatures {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			add("temperature."+k, strconv.FormatFloat(s.Temperatures[k], 'f', 1, 64)+"°C")
		}
		add("uptime", fmtUptime(s.Uptime))
	}
	if fw := snap.Firmware; fw != nil {
		add("firmware.update_available", fmtPtr(fw.UpdateAvailable))
		add("firmware.available", fw.Available)
	}
	parts := make([]string, 0, len(snap.Errors))
	for p := range snap.Errors {
		parts = append(parts, string(p))
	}
	sort.Strings(parts)
	for _, p := range parts {
		add("error."+p, snap.Errors[routerkit.Part(p)].Message)
	}
	return output.Write(os.Stdout, f, v, rows)
}

// inventoryFor opens the state file read-only (nil when absent).
func inventoryFor(path string) inventory.Lookup {
	if path == "" {
		return inventory.Map{}
	}
	st, err := inventory.Open(path)
	if err != nil {
		return inventory.Map{}
	}
	return st.Snapshot()
}

func cmdClients(ctx context.Context, args []string) error {
	fs := newFlagSet("clients", "[--online]")
	df := addDevFlags(fs, true)
	onlineOnly := fs.Bool("online", false, "only online clients")
	if _, err := parse(fs, args); err != nil {
		return err
	}
	f, err := df.format()
	if err != nil {
		return err
	}
	if via, err := df.viaClient(); err != nil {
		return err
	} else if via != nil {
		dev, err := df.viaDevice(ctx, via)
		if err != nil {
			return err
		}
		cs, err := via.Clients(ctx, dev)
		if err != nil {
			return err
		}
		list := []view.Client{}
		rows := output.Rows{Header: []string{"mac", "name", "ip", "online", "connection", "node", "rssi", "vendor", "known"}}
		for _, c := range cs {
			if *onlineOnly && !c.Present {
				continue
			}
			list = append(list, c)
			rows.Rows = append(rows.Rows, []string{c.MAC, c.Name, c.IP, strconv.FormatBool(c.Present), c.Connection, c.Node, fmtPtr(c.RSSI), c.Vendor, strconv.FormatBool(c.Known)})
		}
		return output.Write(os.Stdout, f, list, rows)
	}
	drv, cfg, _, err := df.connect(ctx)
	if err != nil {
		return err
	}
	defer closeQuiet(drv)
	snap, err := drv.Poll(ctx, routerkit.Poll(routerkit.PartClients))
	if err != nil {
		return err
	}
	if pe, ok := snap.Errors[routerkit.PartClients]; ok {
		return pe
	}
	inv := inventoryFor(cfg.StateFile)
	type row struct {
		routerkit.Client
		Label string `json:"label,omitempty"`
		Known *bool  `json:"known,omitempty"`
	}
	var list []row
	rows := output.Rows{Header: []string{"mac", "name", "ip", "online", "connection", "node", "rssi", "vendor", "known"}}
	for _, c := range snap.Clients {
		if *onlineOnly && !c.Online {
			continue
		}
		r := row{Client: c}
		name := c.Name
		known := ""
		if e, ok := inv.Get(c.MAC); ok {
			r.Label = e.Label
			r.Known = routerkit.Ptr(e.Known)
			if e.Label != "" {
				name = e.Label
			}
			known = strconv.FormatBool(e.Known)
		}
		list = append(list, r)
		rows.Rows = append(rows.Rows, []string{c.MAC, name, c.IP, strconv.FormatBool(c.Online), c.Connection, c.Node, fmtPtr(c.RSSI), c.Vendor, known})
	}
	if list == nil {
		list = []row{}
	}
	return output.Write(os.Stdout, f, list, rows)
}

func cmdNodes(ctx context.Context, args []string) error {
	fs := newFlagSet("nodes", "")
	df := addDevFlags(fs, true)
	if _, err := parse(fs, args); err != nil {
		return err
	}
	f, err := df.format()
	if err != nil {
		return err
	}
	drv, _, _, err := df.connect(ctx)
	if err != nil {
		return err
	}
	defer closeQuiet(drv)
	snap, err := drv.Poll(ctx, routerkit.Poll(routerkit.PartNodes))
	if err != nil {
		return err
	}
	if pe, ok := snap.Errors[routerkit.PartNodes]; ok {
		return pe
	}
	nodes := snap.Nodes
	if nodes == nil {
		nodes = []routerkit.Node{}
	}
	rows := output.Rows{Header: []string{"mac", "name", "model", "ip", "online", "clients"}}
	for _, n := range nodes {
		rows.Rows = append(rows.Rows, []string{n.MAC, n.Name, n.Model, n.IP, strconv.FormatBool(n.Online), fmtPtr(n.Clients)})
	}
	return output.Write(os.Stdout, f, nodes, rows)
}

func actionRows(specs []routerkit.ActionSpec, allowed func(string) string) output.Rows {
	rows := output.Rows{Header: []string{"action", "args", "danger", "allowed", "help"}}
	for _, s := range specs {
		var as []string
		for _, a := range s.Args {
			x := a.Name + "="
			switch {
			case len(a.Choices) > 0:
				x += strings.Join(a.Choices, "|")
			case a.Type == routerkit.ArgBool:
				x += "on|off"
			default:
				x += "..."
			}
			if !a.Required {
				x = "[" + x + "]"
			}
			as = append(as, x)
		}
		rows.Rows = append(rows.Rows, []string{s.Name, strings.Join(as, " "), s.Danger.String(), allowed(s.Name), s.Help})
	}
	return rows
}

func cmdDo(ctx context.Context, args []string) error {
	fs := newFlagSet("do", "<action> [key=value...] [--yes]   (no action: list actions)")
	df := addDevFlags(fs, true)
	yes := fs.Bool("yes", false, "confirm a destructive action (reboot, ...)")
	fs.BoolVar(yes, "y", false, "confirm (shorthand)")
	pos, err := parse(fs, args)
	if err != nil {
		return err
	}
	f, err := df.format()
	if err != nil {
		return err
	}
	via, err := df.viaClient()
	if err != nil {
		return err
	}
	if via != nil {
		return doVia(ctx, df, via, f, pos)
	}
	drv, _, dev, err := df.connect(ctx)
	if err != nil {
		return err
	}
	defer closeQuiet(drv)
	caps, err := drv.Capabilities(ctx)
	if err != nil {
		return err
	}
	// The CLI is the console: everything up to disruptive, destructive
	// with --yes (the commands section of the config is for automations)
	pol := cmdsvc.DefaultPolicy
	if *yes {
		pol.MaxDanger = routerkit.Destructive
	}
	if len(pos) == 0 {
		rows := actionRows(caps.Actions, func(name string) string {
			spec, _ := caps.Action(name)
			if pol.Check(spec) != nil {
				return "with --yes"
			}
			return "yes"
		})
		return output.Write(os.Stdout, f, caps.Actions, rows)
	}
	name := pos[0]
	spec, ok := caps.Action(name)
	if !ok {
		return fmt.Errorf("%s does not support action %q (run `routerctl do` to list actions)", dev.Name, name)
	}
	kv, err := cmdsvc.ParseFields(spec, pos[1:])
	if err != nil {
		return usagef("%v", err)
	}
	if _, err := spec.Bind(kv); err != nil {
		return usagef("%v", err)
	}
	res := cmdsvc.Run(ctx, cmdsvc.FromDriver(dev.Name, drv, caps), pol, cmdsvc.Request{Device: dev.Name, Action: name, Args: kv, Source: "cli"})
	return writeResult(f, res, spec.IsDangerous())
}

// writeResult prints a command result; a failure is the error.
func writeResult(f output.Format, res cmdsvc.Result, dangerous bool) error {
	if res.Error != nil {
		if res.Error.Code == cmdsvc.CodeForbidden && dangerous {
			return fmt.Errorf("%q is a dangerous action on %s; re-run with --yes to confirm", res.Action, res.Device)
		}
		if res.Error.Code == cmdsvc.CodeInvalidArgs {
			return usagef("%s", res.Error.Message)
		}
		return res.Error
	}
	out := map[string]any{"device": res.Device, "action": res.Action, "args": res.Args, "ok": res.OK, "result": res.Result}
	if res.Pending {
		out["pending"] = true
	}
	return output.Write(os.Stdout, f, out, output.Tabulate(out))
}

// doVia runs or lists actions through the daemon (its policy applies).
func doVia(ctx context.Context, df *devFlags, c *apiclient.Client, f output.Format, pos []string) error {
	dev, err := df.viaDevice(ctx, c)
	if err != nil {
		return err
	}
	acts, err := c.Actions(ctx, dev)
	if err != nil {
		return err
	}
	specs := make([]routerkit.ActionSpec, len(acts))
	verdict := map[string]string{}
	for i, a := range acts {
		specs[i] = a.ActionSpec
		verdict[a.Name] = "yes"
		if !a.Allowed {
			verdict[a.Name] = "no: " + a.Reason
		}
	}
	if len(pos) == 0 {
		return output.Write(os.Stdout, f, acts, actionRows(specs, func(n string) string { return verdict[n] }))
	}
	spec, ok := routerkit.FindAction(specs, pos[0])
	if !ok {
		return fmt.Errorf("%s does not support action %q (run `routerctl do --via ...` to list actions)", dev, pos[0])
	}
	kv, err := cmdsvc.ParseFields(spec, pos[1:])
	if err != nil {
		return usagef("%v", err)
	}
	res, err := c.Do(ctx, dev, spec.Name, kv)
	if err != nil {
		return err
	}
	if res.Error != nil && res.Error.Code == cmdsvc.CodeForbidden {
		return fmt.Errorf("the daemon's command policy refuses it: %s", res.Error.Message)
	}
	return writeResult(f, res, false)
}

func cmdDevices(ctx context.Context, args []string) error {
	fs := newFlagSet("devices", "[list | label <mac> <name> | forget <mac> | approve <mac>]")
	df := addDevFlags(fs, true)
	stateFile := fs.String("state", "", "state file (default: state_file from the config)")
	unknown := fs.Bool("unknown", false, "list: only unknown devices")
	pos, err := parse(fs, args)
	if err != nil {
		return err
	}
	f, err := df.format()
	if err != nil {
		return err
	}
	setupLogging(df.logLevel, "warn", "text")
	if via, err := df.viaClient(); err != nil {
		return err
	} else if via != nil {
		return devicesVia(ctx, via, f, pos, *unknown)
	}
	path := *stateFile
	if path == "" {
		if v := os.Getenv("ROUTERCTL_STATE_FILE"); v != "" {
			path = v
		} else if p, must := configPath(df.config); p != "" {
			// Only the state_file setting matters here; ignore device errors
			if cfg, err := loadRawState(p, must); err == nil && cfg != "" {
				path = cfg
			}
		}
	}
	if path == "" {
		path = "routerctl-state.json"
	}
	st, err := inventory.Open(path)
	if err != nil {
		return err
	}
	if len(pos) == 0 {
		pos = []string{"list"}
	}
	sub := pos[0]
	need := func(n int) error {
		if len(pos) != n+1 {
			return usagef("%s takes %d argument(s)", sub, n)
		}
		return nil
	}
	switch sub {
	case "list", "ls":
		if err := need(0); err != nil {
			return err
		}
		entries := st.List()
		rows := output.Rows{Header: []string{"mac", "label", "name", "ip", "vendor", "known", "online", "first_seen", "last_seen", "router"}}
		list := []inventory.Entry{}
		for _, e := range entries {
			if *unknown && e.Known {
				continue
			}
			list = append(list, e)
			rows.Rows = append(rows.Rows, []string{e.MAC, e.Label, e.Name, e.IP, e.Vendor, strconv.FormatBool(e.Known), strconv.FormatBool(e.Online), ts(e.FirstSeen), ts(e.LastSeen), e.Router})
		}
		return output.Write(os.Stdout, f, list, rows)
	case "label":
		if len(pos) < 3 {
			return usagef("label <mac> <name>")
		}
		err = st.Label(pos[1], strings.Join(pos[2:], " "))
	case "forget", "rm":
		if err = need(1); err == nil {
			err = st.Forget(pos[1])
		}
	case "approve":
		if err = need(1); err == nil {
			err = st.Approve(pos[1])
		}
	default:
		return usagef("unknown devices subcommand %q", sub)
	}
	if err != nil {
		return err
	}
	if err := st.Save(); err != nil {
		return err
	}
	fmt.Printf("%s %s: saved to %s\n", sub, routerkit.NormalizeMAC(pos[1]), path)
	return nil
}

func inventoryRows(entries []inventory.Entry) output.Rows {
	rows := output.Rows{Header: []string{"mac", "label", "name", "ip", "vendor", "known", "online", "first_seen", "last_seen", "router"}}
	for _, e := range entries {
		rows.Rows = append(rows.Rows, []string{e.MAC, e.Label, e.Name, e.IP, e.Vendor, strconv.FormatBool(e.Known), strconv.FormatBool(e.Online), ts(e.FirstSeen), ts(e.LastSeen), e.Router})
	}
	return rows
}

// devicesVia edits the inventory through the daemon (saved at once).
func devicesVia(ctx context.Context, c *apiclient.Client, f output.Format, pos []string, unknown bool) error {
	if len(pos) == 0 {
		pos = []string{"list"}
	}
	sub := pos[0]
	switch {
	case (sub == "list" || sub == "ls") && len(pos) == 1:
		list, err := c.Inventory(ctx, unknown)
		if err != nil {
			return err
		}
		return output.Write(os.Stdout, f, list, inventoryRows(list))
	case sub == "label" && len(pos) >= 3:
		label := strings.Join(pos[2:], " ")
		_, err := c.Patch(ctx, pos[1], api.InventoryPatch{Label: &label})
		if err != nil {
			return err
		}
	case sub == "approve" && len(pos) == 2:
		known := true
		if _, err := c.Patch(ctx, pos[1], api.InventoryPatch{Known: &known}); err != nil {
			return err
		}
	case (sub == "forget" || sub == "rm") && len(pos) == 2:
		if err := c.Forget(ctx, pos[1]); err != nil {
			return err
		}
	default:
		return usagef("devices [list | label <mac> <name> | forget <mac> | approve <mac>]")
	}
	fmt.Printf("%s %s: saved by the daemon\n", sub, routerkit.NormalizeMAC(pos[1]))
	return nil
}

func ts(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Local().Format("2006-01-02 15:04")
}

// loadRawState returns the state_file setting of a config file.
func loadRawState(path string, must bool) (string, error) {
	cfg, err := config.LoadRaw(path, must, os.Getenv)
	if err != nil {
		return "", err
	}
	return cfg.StateFile, nil
}
