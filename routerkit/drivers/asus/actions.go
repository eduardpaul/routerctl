package asus

import (
	"context"
	"fmt"

	"github.com/eduardpaul/routerctl/asusrouter/modules/action"
	"github.com/eduardpaul/routerctl/asusrouter/modules/common"
	"github.com/eduardpaul/routerctl/asusrouter/modules/led"
	pc "github.com/eduardpaul/routerctl/asusrouter/modules/parental_control"
	"github.com/eduardpaul/routerctl/asusrouter/modules/parental_control/block_all"
	"github.com/eduardpaul/routerctl/asusrouter/modules/parental_control/schedule"
	"github.com/eduardpaul/routerctl/asusrouter/modules/service"
	"github.com/eduardpaul/routerctl/asusrouter/modules/speedtest"
	"github.com/eduardpaul/routerctl/asusrouter/modules/vpn"
	vpnclient "github.com/eduardpaul/routerctl/asusrouter/modules/vpn/client"
	"github.com/eduardpaul/routerctl/asusrouter/modules/wifi"
	wifiaction "github.com/eduardpaul/routerctl/asusrouter/modules/wifi/action"
	"github.com/eduardpaul/routerctl/asusrouter/parity/canonical"
	"github.com/eduardpaul/routerctl/routerkit"
)

// Services that restart-service accepts without --force style escapes:
// the common, harmless ones. Any other rc_service name is refused, so an
// MQTT command cannot run arbitrary services (reboot has its own action).
var restartServices = []string{
	string(common.ARServiceWirelessRestart),
	"restart_wan", "restart_wan_if", "restart_dnsmasq", "restart_firewall",
	"restart_httpd", "restart_upnp", "restart_qos", "restart_vpnc",
	"restart_ddns", "restart_time", "restart_leds", "restart_net",
}

// bands lists the WiFi bands of the device (all bands before Connect).
func (d *Driver) bands() []string {
	var out []string
	if id := d.r.Description(); id != nil {
		for _, b := range id.WifiBands() {
			out = append(out, string(b))
		}
	}
	if len(out) == 0 {
		for _, b := range wifi.ARWiFiBandMembers {
			if b != wifi.ARWiFiBandUnknown {
				out = append(out, string(b))
			}
		}
	}
	return out
}

// actions lists the actions (Capabilities.Actions).
func (d *Driver) actions() []routerkit.ActionSpec {
	return []routerkit.ActionSpec{
		routerkit.Std(routerkit.ActBlockAll),
		routerkit.Std(routerkit.ActBlockClient,
			routerkit.WithHelp("block / unblock a client's internet access (parental control rule)"),
			routerkit.WithArg(routerkit.Arg{Name: "name", Help: "rule name (default: the MAC)", Type: routerkit.ArgString})),
		routerkit.Std(routerkit.ActLED),
		routerkit.Std(routerkit.ActReboot, routerkit.WithHelp("reboot the router")),
		{Name: "restart-service", Help: "restart a router service", Danger: routerkit.Disruptive, Args: []routerkit.Arg{
			{Name: "name", Help: "rc_service name", Type: routerkit.ArgEnum, Required: true, Choices: restartServices},
		}},
		routerkit.Std(routerkit.ActSpeedtest,
			routerkit.WithHelp("start an Ookla speedtest on the router (read the result with `get ARSpeedTestSource`)"),
			routerkit.WithArg(routerkit.Arg{Name: "server", Help: "server id (default: automatic)", Type: routerkit.ArgString}),
			routerkit.WithArg(routerkit.Arg{Name: "iface", Help: "WAN interface (default: automatic)", Type: routerkit.ArgString})),
		routerkit.Std(routerkit.ActVPN,
			routerkit.WithChoices("protocol", string(vpn.ARVpnProtocolOpenvpn), string(vpn.ARVpnProtocolWireguard)),
			routerkit.WithArg(routerkit.Arg{Name: "unit", Help: "client unit (default 1)", Type: routerkit.ArgInt, Default: "1"})),
		routerkit.Std(routerkit.ActWiFi, routerkit.WithChoices("band", d.bands()...)),
	}
}

// Do implements routerkit.Driver. The result is the canonical form of the
// library's action result; Accepted mirrors its Python truthiness.
func (d *Driver) Do(ctx context.Context, inv routerkit.Invocation) (routerkit.Result, error) {
	if err := d.checkOpen(); err != nil {
		return routerkit.Result{}, err
	}
	_, args, err := routerkit.BindInvocation(d.actions(), inv)
	if err != nil {
		return routerkit.Result{}, err
	}
	state := args.Bool("state")
	var a action.ARAction
	switch inv.Name {
	case routerkit.ActLED:
		a = &led.ARLedAction{State: state}
	case routerkit.ActReboot:
		a = service.NewARServiceAction(common.ARServiceReboot, nil)
	case "restart-service":
		a = service.NewARServiceAction(common.ARService(args.String("name")), nil)
	case routerkit.ActWiFi:
		band, ok := wifi.ARWiFiBandFromValueOK(args.String("band"))
		if !ok {
			return routerkit.Result{}, routerkit.Classify(fmt.Errorf("wifi: unknown band %q", args.String("band")), routerkit.ErrInvalidArgs)
		}
		a = wifiaction.NewARWiFiAction(band, state)
	case routerkit.ActBlockAll:
		a = &blockall.ARBlockAllAction{State: state}
	case routerkit.ActBlockClient:
		mac := args.MAC("mac")
		if state {
			rname := args.String("name")
			if rname == "" {
				rname = mac
			}
			a = &schedule.ARParentalControlAction{Command: pc.ARParentalControlCommandAdd, Rules: []schedule.ARParentalControlRule{{
				pc.ARParentalControlFieldMac:  mac,
				pc.ARParentalControlFieldName: rname,
				pc.ARParentalControlFieldType: pc.ARParentalControlTypeBlock,
			}}}
		} else {
			a = &schedule.ARParentalControlAction{Command: pc.ARParentalControlCommandRemove, Rules: []schedule.ARParentalControlRule{{
				pc.ARParentalControlFieldMac: mac,
			}}}
		}
	case routerkit.ActSpeedtest:
		var server any
		if v := args.String("server"); v != "" {
			server = v
		}
		var iface *string
		if v := args.String("iface"); v != "" {
			iface = &v
		}
		a = speedtest.NewARSpeedTestAction(server, iface)
	case routerkit.ActVPN:
		proto, ok := vpn.ARVpnProtocolFromValueOK(args.String("protocol"))
		if !ok {
			return routerkit.Result{}, routerkit.Classify(fmt.Errorf("vpn: unknown protocol %q", args.String("protocol")), routerkit.ErrInvalidArgs)
		}
		a = vpnclient.NewARVpnClientAction(proto, state, args.Int("unit"))
	default:
		return routerkit.Result{}, fmt.Errorf("unknown action %q: %w", inv.Name, routerkit.ErrUnsupported)
	}
	res, err := d.r.RunAction(ctx, a, nil)
	if err != nil {
		return routerkit.Result{}, mapErr(err)
	}
	out := canonical.Of(res)
	return routerkit.Result{
		Accepted:         truthy(out),
		Data:             out,
		Pending:          inv.Name == routerkit.ActSpeedtest,
		ExpectDisconnect: inv.Name == routerkit.ActReboot,
	}, nil
}

// truthy mirrors the Python truthiness of an action result (a canonical
// ARServiceResult is {"success": bool, ...}).
func truthy(v any) bool {
	switch t := v.(type) {
	case nil:
		return false
	case bool:
		return t
	case map[string]any:
		if s, ok := t["success"].(bool); ok {
			return s
		}
		return len(t) > 0
	case []any:
		return len(t) > 0
	case string:
		return t != ""
	case int64:
		return t != 0
	}
	return true
}
