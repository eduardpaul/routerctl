package commands

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/eduardpaul/routerctl/routerkit"
	"github.com/eduardpaul/routerctl/routerkit/drivers/fake"
)

func specs(t *testing.T) (*fake.Fake, routerkit.Capabilities) {
	t.Helper()
	f := fake.New()
	if err := f.Connect(context.Background()); err != nil {
		t.Fatal(err)
	}
	caps, err := f.Capabilities(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return f, caps
}

func TestParse(t *testing.T) {
	_, caps := specs(t)
	led, _ := caps.Action("led")
	block, _ := caps.Action("block-client")
	svc, _ := caps.Action("restart-service")
	reboot, _ := caps.Action("reboot")
	cases := []struct {
		spec    routerkit.ActionSpec
		payload string
		want    map[string]string
		id      string
		err     bool
	}{
		{led, "on", map[string]string{"state": "on"}, "", false},
		{led, `{"state": true, "_id": "r1"}`, map[string]string{"state": "on"}, "r1", false},
		{led, "state=off", map[string]string{"state": "off"}, "", false},
		{block, "mac=02:00:00:00:10:01&state=on", map[string]string{"mac": "02:00:00:00:10:01", "state": "on"}, "", false},
		{block, "mac=02:00:00:00:10:01 on", map[string]string{"mac": "02:00:00:00:10:01", "state": "on"}, "", false},
		{svc, "httpd", map[string]string{"name": "httpd"}, "", false},
		{reboot, "PRESS", map[string]string{}, "", false},
		{reboot, "", map[string]string{}, "", false},
		{reboot, "now", nil, "", true},
		{led, `{"state": [1]}`, nil, "", true},
		{led, `{bad json`, nil, "", true},
		{led, "on off", nil, "", true},
		{led, "on state=off", nil, "", true},
	}
	for _, c := range cases {
		got, id, err := Parse(c.spec, []byte(c.payload))
		if (err != nil) != c.err {
			t.Errorf("%s %q: err %v", c.spec.Name, c.payload, err)
			continue
		}
		if err != nil {
			if !errors.Is(err, routerkit.ErrInvalidArgs) {
				t.Errorf("%s %q: not ErrInvalidArgs: %v", c.spec.Name, c.payload, err)
			}
			continue
		}
		if !reflect.DeepEqual(got, c.want) || id != c.id {
			t.Errorf("%s %q: got %v id %q", c.spec.Name, c.payload, got, id)
		}
	}
}

func TestPolicy(t *testing.T) {
	_, caps := specs(t)
	reboot, _ := caps.Action("reboot")
	led, _ := caps.Action("led")
	wifi, _ := caps.Action("wifi")
	if err := DefaultPolicy.Check(reboot); !errors.Is(err, ErrForbidden) {
		t.Errorf("default allows reboot: %v", err)
	}
	if err := DefaultPolicy.Check(wifi); err != nil {
		t.Errorf("default refuses wifi: %v", err)
	}
	safe := DefaultPolicy.Restrict(Policy{MaxDanger: routerkit.Safe})
	if safe.Check(wifi) == nil || safe.Check(led) != nil {
		t.Error("safe policy")
	}
	all := Policy{MaxDanger: routerkit.Destructive, Deny: []string{"led"}}
	if all.Check(reboot) != nil || all.Check(led) == nil {
		t.Error("deny list")
	}
	only := Policy{MaxDanger: routerkit.Destructive, Allow: []string{"led", "wifi"}}
	if only.Check(led) != nil || only.Check(reboot) == nil {
		t.Error("allow list")
	}
	if r := only.Restrict(Policy{MaxDanger: routerkit.Destructive, Allow: []string{"reboot"}}); r.Check(led) == nil || r.Check(reboot) == nil {
		t.Error("disjoint allow lists must allow nothing")
	}
	for _, a := range DefaultPolicy.Actions(caps.Actions) {
		if (a.Name == "reboot") == a.Allowed {
			t.Errorf("actions: %s allowed=%v", a.Name, a.Allowed)
		}
		if !a.Allowed && a.Reason == "" {
			t.Errorf("%s: no reason", a.Name)
		}
	}
	if d, err := ParseDanger("Destructive", routerkit.Safe); err != nil || d != routerkit.Destructive {
		t.Errorf("parse danger: %v %v", d, err)
	}
	if _, err := ParseDanger("yolo", routerkit.Safe); err == nil {
		t.Error("bad danger accepted")
	}
}

func TestRun(t *testing.T) {
	f, caps := specs(t)
	tg := FromDriver("home", f, caps)
	ctx := context.Background()

	res := Run(ctx, tg, DefaultPolicy, Request{Device: "home", Action: "led", Payload: []byte(`{"state": false, "_id": "x"}`), Source: "test"})
	if !res.OK || res.ID != "x" || res.Args["state"] != "off" || res.Error != nil || res.Danger != "safe" {
		t.Errorf("led: %+v", res)
	}
	codes := []struct {
		req  Request
		code Code
	}{
		{Request{Device: "home", Action: "reboot"}, CodeForbidden},
		{Request{Device: "home", Action: "reboot", Payload: []byte("off")}, CodeForbidden},
		{Request{Device: "home", Action: "led", Payload: []byte("dim")}, CodeInvalidArgs},
		{Request{Device: "home", Action: "fly"}, CodeUnsupported},
		{Request{Device: "nope", Action: "led", Payload: []byte("on")}, CodeUnknownDevice},
	}
	for _, c := range codes {
		r := Run(ctx, tg, DefaultPolicy, c.req)
		if r.OK || r.Error == nil || r.Error.Code != c.code {
			t.Errorf("%s/%s: %+v", c.req.Device, c.req.Action, r)
		}
	}
	if r := Run(ctx, tg, Policy{MaxDanger: routerkit.Destructive}, Request{Device: "home", Action: "reboot"}); !r.OK || !r.ExpectDisconnect {
		t.Errorf("reboot with destructive policy: %+v", r)
	}
	if r := Run(ctx, tg, DefaultPolicy, Request{Device: "home", Action: "wifi", Args: map[string]string{"band": "5G1", "state": "off"}}); !r.OK || r.Args["band"] != "5g1" {
		t.Errorf("pre-parsed args: %+v", r)
	}
	if calls := f.Calls(); len(calls) != 3 {
		t.Errorf("calls: %+v", calls)
	}
}
