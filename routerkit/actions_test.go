package routerkit

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func wifiSpec() ActionSpec { return Std(ActWiFi, WithChoices("band", "2g1", "5g1")) }

func TestBind(t *testing.T) {
	spec := wifiSpec()
	raw := map[string]string{"band": "5G1", "state": "TRUE"}
	args, err := spec.Bind(raw)
	if err != nil {
		t.Fatal(err)
	}
	if args.String("band") != "5g1" || !args.Bool("state") || args.String("state") != "on" {
		t.Errorf("args %v", args.Strings())
	}
	if raw["band"] != "5G1" {
		t.Error("Bind modified its input")
	}
	for _, bad := range []map[string]string{
		{"band": "5g1"},
		{"band": "9g", "state": "on"},
		{"band": "5g1", "state": "maybe"},
		{"band": "5g1", "state": "on", "extra": "x"},
	} {
		if _, err := spec.Bind(bad); !errors.Is(err, ErrInvalidArgs) {
			t.Errorf("Bind(%v) = %v, want ErrInvalidArgs", bad, err)
		}
	}
	if _, err := spec.Bind(map[string]string{"band": "9g", "state": "on"}); err == nil || err.Error() != `wifi: band="9g" is not one of 2g1|5g1` {
		t.Errorf("message: %v", err)
	}
}

func TestBindTypes(t *testing.T) {
	spec := ActionSpec{Name: "x", Args: []Arg{
		{Name: "mac", Type: ArgMAC, Required: true},
		{Name: "n", Type: ArgInt, Default: "1"},
		{Name: "ip", Type: ArgIP},
		{Name: "d", Type: ArgDuration},
		{Name: "s", Type: ArgString},
	}}
	args, err := spec.Bind(map[string]string{"mac": "AA-BB-CC-DD-EE-FF", "ip": "2001:DB8::1", "d": "90s"})
	if err != nil {
		t.Fatal(err)
	}
	if args.MAC("mac") != "aa:bb:cc:dd:ee:ff" || args.Int("n") != 1 || args.String("ip") != "2001:db8::1" ||
		args.Duration("d") != 90*time.Second || args.String("d") != "1m30s" || args.Has("s") || args.Len() != 4 {
		t.Errorf("args %v", args.Strings())
	}
	for _, bad := range []map[string]string{
		{"mac": "nope"}, {"mac": "aabbccddeeff", "n": "x"}, {"mac": "aabbccddeeff", "ip": "1.2.3"}, {"mac": "aabbccddeeff", "d": "soon"},
	} {
		if _, err := spec.Bind(bad); !errors.Is(err, ErrInvalidArgs) {
			t.Errorf("Bind(%v) = %v", bad, err)
		}
	}
}

func TestInvocationJSON(t *testing.T) {
	args, _ := wifiSpec().Bind(map[string]string{"band": "2g1", "state": "off"})
	b, err := json.Marshal(Invocation{Name: ActWiFi, Args: args})
	if err != nil || string(b) != `{"name":"wifi","args":{"band":"2g1","state":"off"}}` {
		t.Fatalf("%s %v", b, err)
	}
	var inv Invocation
	if err := json.Unmarshal(b, &inv); err != nil {
		t.Fatal(err)
	}
	spec, bound, err := BindInvocation([]ActionSpec{wifiSpec()}, inv)
	if err != nil || spec.Name != ActWiFi || bound.Bool("state") || bound.String("band") != "2g1" {
		t.Errorf("BindInvocation %v %v", bound.Strings(), err)
	}
	if _, _, err := BindInvocation([]ActionSpec{wifiSpec()}, Invocation{Name: "nope"}); !errors.Is(err, ErrUnsupported) {
		t.Errorf("unknown action: %v", err)
	}
	if _, _, err := BindInvocation([]ActionSpec{wifiSpec()}, Invocation{Name: ActWiFi, Args: RawArgs(map[string]string{"band": "2g1"})}); !errors.Is(err, ErrInvalidArgs) {
		t.Errorf("missing state: %v", err)
	}
	c := Capabilities{Actions: []ActionSpec{wifiSpec()}}
	if _, inv, err := c.Invoke(ActWiFi, map[string]string{"band": "5g1", "state": "1"}); err != nil || inv.Args.String("state") != "on" {
		t.Errorf("Invoke %v %v", inv, err)
	}
}

func TestStdCatalog(t *testing.T) {
	for _, c := range Catalog() {
		// catalog enums get their choices from the driver
		var opts []StdOption
		for _, a := range c.Args {
			if a.Type == ArgEnum {
				opts = append(opts, WithChoices(a.Name, "x"))
			}
		}
		if err := CheckSpec(Std(c.Name, opts...)); err != nil {
			t.Error(err)
		}
	}
	reboot := Std(ActReboot)
	if !reboot.IsDangerous() || reboot.Danger != Destructive {
		t.Errorf("reboot %+v", reboot)
	}
	b, _ := json.Marshal(reboot)
	if !strings.Contains(string(b), `"danger":"destructive"`) || strings.Contains(string(b), `"dangerous"`) {
		t.Errorf("reboot JSON %s", b)
	}
	bc := Std(ActBlockClient, WithArg(Arg{Name: "name", Type: ArgString, Required: true}))
	if a, _ := bc.Arg("name"); a.Required {
		t.Error("WithArg must make the argument optional")
	}
	if err := CheckSpec(bc); err != nil {
		t.Error(err)
	}
	for name, bad := range map[string]ActionSpec{
		"reused name":       {Name: ActReboot, Danger: Destructive},
		"changed danger":    func() ActionSpec { s := Std(ActLED); s.Danger = Disruptive; return s }(),
		"enum w/o choices":  Std(ActWiFi),
		"toggle not bool":   {Name: "x", Toggle: "y", Args: []Arg{{Name: "y", Type: ArgString}}},
		"bool with choices": {Name: "x", Args: []Arg{{Name: "y", Type: ArgBool, Choices: []string{"on", "off"}}}},
		"untyped arg":       {Name: "x", Args: []Arg{{Name: "y"}}},
		"bad name":          {Name: "Bad Name"},
		"unknown standard":  {Name: "zz", Standard: true},
		"changed arg type":  func() ActionSpec { s := Std(ActBlockClient); s.Args[0].Type = ArgString; return s }(),
	} {
		if err := CheckSpec(bad); err == nil {
			t.Errorf("%s: accepted %+v", name, bad)
		}
	}
	defer func() {
		if recover() == nil {
			t.Error("Std of an unknown name did not panic")
		}
	}()
	Std("zz")
}

func TestResultErr(t *testing.T) {
	if (Result{Accepted: true}).Err("led") != nil {
		t.Error("accepted result is an error")
	}
	if err := (Result{}).Err("led"); !errors.Is(err, ErrRejected) || err.Error() != "led: the device did not accept the action" {
		t.Errorf("rejected: %v", err)
	}
	var b Base
	if _, err := b.Do(context.Background(), Invocation{Name: "x"}); !errors.Is(err, ErrUnsupported) {
		t.Errorf("Base.Do %v", err)
	}
}

func TestBindOptions(t *testing.T) {
	specs := []OptionSpec{
		{Name: "cache", Type: ArgDuration, Default: "5s"},
		{Name: "pon", Type: ArgBool, Default: "true"},
		{Name: "mode", Type: ArgEnum, Choices: []string{"a", "b"}},
	}
	if err := CheckOptionSpecs(specs); err != nil {
		t.Fatal(err)
	}
	o, err := BindOptions(specs, map[string]string{"pon": "no", "mode": "B"})
	if err != nil {
		t.Fatal(err)
	}
	if o.Duration("cache") != 5*time.Second || o.Bool("pon") || o.String("mode") != "b" {
		t.Errorf("options %v", o.Strings())
	}
	_, err = BindOptions(specs, map[string]string{"cache": "soon", "typo": "1"})
	if err == nil || !strings.Contains(err.Error(), `option cache="soon" is not a duration`) || !strings.Contains(err.Error(), "unknown option(s) typo (accepted: cache, pon, mode)") {
		t.Errorf("errors: %v", err)
	}
	if _, err := BindOptions(nil, map[string]string{"x": "1"}); err == nil || !strings.Contains(err.Error(), "accepted: none") {
		t.Errorf("no options: %v", err)
	}
	for _, bad := range [][]OptionSpec{
		{{Name: "a", Type: ArgBool}, {Name: "a", Type: ArgBool}},
		{{Name: "a", Type: "float"}},
		{{Name: "a", Type: ArgEnum}},
		{{Name: "a", Type: ArgInt, Default: "x"}},
	} {
		if CheckOptionSpecs(bad) == nil {
			t.Errorf("accepted %+v", bad)
		}
	}
	r, _ := NewRegistry(Factory{Info: DriverInfo{Name: "zz"}, Options: specs, New: newStub})
	if _, err := r.Open("zz", Config{Name: "home", Options: map[string]string{"typo": "1"}}); err == nil || !strings.HasPrefix(err.Error(), "home: zz: unknown option") {
		t.Errorf("Open with a bad option: %v", err)
	}
	if _, err := NewRegistry(Factory{Info: DriverInfo{Name: "zz"}, Options: []OptionSpec{{Name: "a"}}, New: newStub}); err == nil {
		t.Error("NewRegistry accepted an untyped option")
	}
}
