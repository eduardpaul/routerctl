package routerkit

import (
	"strings"
	"testing"
)

func TestNormalizeMAC(t *testing.T) {
	for in, want := range map[string]string{
		"AA:BB:CC:DD:EE:FF":  "aa:bb:cc:dd:ee:ff",
		"aa-bb-cc-dd-ee-ff":  "aa:bb:cc:dd:ee:ff",
		"aabbccddeeff":       "aa:bb:cc:dd:ee:ff",
		" aa:bb:cc:dd:ee:ff": "aa:bb:cc:dd:ee:ff",
		"zz:bb:cc:dd:ee:ff":  "",
		"":                   "",
	} {
		if got := NormalizeMAC(in); got != want {
			t.Errorf("NormalizeMAC(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestParseOnOff(t *testing.T) {
	for _, s := range []string{"on", "ON", "true", "1", "yes"} {
		if v, err := ParseOnOff(s); err != nil || !v {
			t.Errorf("%q", s)
		}
	}
	for _, s := range []string{"off", "False", "0", "no"} {
		if v, err := ParseOnOff(s); err != nil || v {
			t.Errorf("%q", s)
		}
	}
	if _, err := ParseOnOff("maybe"); err == nil {
		t.Error("maybe accepted")
	}
}

func zzFingerprint(marker string, score int) Fingerprint {
	return Fingerprint{Match: func(ev *Evidence) (Match, bool) {
		if strings.Contains(ev.Home().Text(), marker) {
			return Match{Score: score, Model: ev.Home().Title, Reason: "marker"}, true
		}
		return Match{}, false
	}}
}

func TestFingerprints(t *testing.T) {
	f := Factory{Info: DriverInfo{Name: "zz-test"}, New: newStub, Fingerprints: []Fingerprint{zzFingerprint("zz", ScoreStrong)}}
	ev := HomeEvidence("<title> &#90;Z-1 </title>zz")
	ms := f.Matches(ev)
	if len(ms) != 1 || ms[0].Driver != "zz-test" || ms[0].Model != "ZZ-1" || ms[0].String() != "zz-test score=50 model=ZZ-1: marker" {
		t.Errorf("Matches = %+v", ms)
	}
	if ms := f.Matches(HomeEvidence("nothing")); len(ms) != 0 {
		t.Errorf("unrelated page matched: %+v", ms)
	}
	ev.Pages["/"] = Page{Body: "a", Redirects: []string{"/Main_Login.asp"}}
	if !strings.Contains(ev.Home().Text(), "/Main_Login.asp") {
		t.Error("redirects are part of the text")
	}
	for _, bad := range []Fingerprint{{}, {Probes: []string{"tr64desc.xml"}, Match: zzFingerprint("x", 1).Match}} {
		if _, err := NewRegistry(Factory{Info: DriverInfo{Name: "zz"}, New: newStub, Fingerprints: []Fingerprint{bad}}); err == nil {
			t.Errorf("NewRegistry accepted fingerprint %+v", bad)
		}
	}
}

func TestNewRegistry(t *testing.T) {
	r, err := NewRegistry(Factory{Info: DriverInfo{Name: "b"}, New: newStub}, Factory{Info: DriverInfo{Name: "a"}, New: newStub})
	if err != nil {
		t.Fatal(err)
	}
	if got := r.Names(); len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Errorf("Names = %v", got)
	}
	if _, ok := r.Lookup("a"); !ok {
		t.Error("Lookup a")
	}
	if _, ok := r.Lookup("c"); ok {
		t.Error("Lookup c")
	}
	for _, bad := range [][]Factory{
		{{Info: DriverInfo{Name: "a"}, New: newStub}, {Info: DriverInfo{Name: "a"}, New: newStub}},
		{{Info: DriverInfo{Name: ""}, New: newStub}},
		{{Info: DriverInfo{Name: "auto"}, New: newStub}},
		{{Info: DriverInfo{Name: "x"}}},
		{{Info: DriverInfo{Name: "Upper"}, New: newStub}},
	} {
		if _, err := NewRegistry(bad...); err == nil {
			t.Errorf("accepted %+v", bad)
		}
	}
	// Separate registries share nothing
	other, _ := NewRegistry()
	if len(other.Names()) != 0 {
		t.Errorf("empty registry lists %v", other.Names())
	}
}
