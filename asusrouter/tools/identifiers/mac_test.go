package identifiers

import (
	"encoding/hex"
	"strings"
	"testing"

	"github.com/eduardpaul/routerctl/asusrouter/tools/security"
)

const (
	correctMac       = "aa:bb:cc:dd:ee:ff"
	correctMacAsAsus = "AA:BB:CC:DD:EE:FF"
	correctMacInt    = 0xaabbccddeeff
)

var correctMacBytes, _ = hex.DecodeString("aabbccddeeff")

func mustMac(t *testing.T, v any) MacAddress {
	t.Helper()
	m, err := MacAddressFromValue(v)
	if err != nil {
		t.Fatalf("MacAddressFromValue(%#v): %v", v, err)
	}
	return m
}

func expectMacErr(t *testing.T, v any, msg string) {
	t.Helper()
	if _, err := MacAddressFromValue(v); err == nil || !strings.Contains(err.Error(), msg) {
		t.Errorf("MacAddressFromValue(%#v) error = %v, want %q", v, err, msg)
	}
	if _, err := NewMacAddress(v); err == nil || !strings.Contains(err.Error(), msg) {
		t.Errorf("NewMacAddress(%#v) error = %v, want %q", v, err, msg)
	}
}

func TestMacFromInstance(t *testing.T) {
	instance := mustMac(t, correctMac)
	if mustMac(t, instance) != instance || mustMac(t, &instance) != instance {
		t.Fatal("instance not passed through")
	}
}

func TestMacFromBytes(t *testing.T) {
	if got := mustMac(t, correctMacBytes).String(); got != correctMac {
		t.Fatalf("got %q", got)
	}
	if got := mustMac(t, [6]byte(correctMacBytes)).String(); got != correctMac {
		t.Fatalf("got %q", got)
	}
	for _, v := range [][]byte{{0x00}, {0xaa, 0xbb, 0xff}} {
		expectMacErr(t, v, ErrorMacByte)
	}
}

func TestMacFromInt(t *testing.T) {
	cases := []struct {
		value  any
		result string
	}{
		{correctMacInt, correctMac},
		{123, "00:00:00:00:00:7b"},
		{"  123  ", "00:00:00:00:00:7b"},
		// Python reads plain digits as a decimal integer first
		{"001122334455", "00:00:42:e5:76:f7"},
	}
	for _, c := range cases {
		if got := mustMac(t, c.value).String(); got != c.result {
			t.Errorf("MacAddress(%#v) = %q, want %q", c.value, got, c.result)
		}
	}
	for _, v := range []any{-1, 1 << 48} {
		expectMacErr(t, v, ErrorMacInt)
	}
}

func TestMacFromString(t *testing.T) {
	for _, v := range []string{correctMac, "AA-BB-CC-DD-EE-FF", "aabb.ccdd.eeff", "AABBCCDDEEFF", "  " + correctMac + "  "} {
		if got := mustMac(t, v).String(); got != correctMac {
			t.Errorf("MacAddress(%q) = %q", v, got)
		}
	}
	for _, v := range []string{"not a mac", "ff:aa:bb:cc:dd:ee:ff"} {
		expectMacErr(t, v, ErrorMacStr)
	}
}

func TestMacFromUnsupported(t *testing.T) {
	for _, v := range []any{nil, struct{}{}} {
		expectMacErr(t, v, ErrorMacUnsupportedType)
	}
}

func TestMacFromValueSafe(t *testing.T) {
	if got := MacAddressFromValueSafe(correctMac); got == nil || *got != mustMac(t, correctMac) {
		t.Fatalf("got %v", got)
	}
	if MacAddressFromValueSafe(nil) != nil || MacAddressFromValueSafe(struct{}{}) != nil {
		t.Fatal("expected nil")
	}
}

func TestMacToAsus(t *testing.T) {
	cases := map[string]string{correctMac: correctMacAsAsus, "00:aa:11:bb:22:cc": "00:AA:11:BB:22:CC"}
	for v, want := range cases {
		if got := mustMac(t, v).AsAsus(); got != want {
			t.Errorf("AsAsus(%q) = %q", v, got)
		}
	}
}

func TestMacToBytesAndInt(t *testing.T) {
	m := mustMac(t, correctMac)
	if string(m.ToBytes()) != string(correctMacBytes) || m.ToInt() != correctMacInt {
		t.Fatal("bad conversion")
	}
}

func TestMacSetLocallyAdministered(t *testing.T) {
	cases := map[string]string{
		"00:00:00:00:00:00": "02:00:00:00:00:00",
		"44:00:00:00:00:00": "46:00:00:00:00:00",
		"99:00:00:00:00:00": "9a:00:00:00:00:00",
		"ff:00:00:00:00:00": "fe:00:00:00:00:00",
	}
	for v, want := range cases {
		m := mustMac(t, v)
		m.SetLocallyAdministered()
		if m.String() != want {
			t.Errorf("%q -> %q, want %q", v, m.String(), want)
		}
	}
}

func TestMacRepr(t *testing.T) {
	m := mustMac(t, correctMac)
	if m.GoString() != correctMac || m.CanonicalValue() != correctMac {
		t.Fatal("bad repr")
	}
	if text, _ := m.MarshalText(); string(text) != correctMac {
		t.Fatal("bad MarshalText")
	}
}

func TestMacEq(t *testing.T) {
	m := mustMac(t, correctMac)
	cases := []struct {
		value  any
		result bool
	}{
		{correctMac, true},
		{"00:00:00:00:00:00", false},
		{mustMac(t, correctMac), true},
		{mustMac(t, "00:00:00:00:00:00"), false},
		// Uncomparable
		{struct{}{}, false},
	}
	for _, c := range cases {
		if got := m.Equal(c.value); got != c.result {
			t.Errorf("Equal(%#v) = %v", c.value, got)
		}
	}
}

func TestMacHashable(t *testing.T) {
	set := map[MacAddress]bool{mustMac(t, correctMac): true}
	if !set[mustMac(t, "AA-BB-CC-DD-EE-FF")] {
		t.Fatal("not usable as a map key")
	}
}

func TestMacMask(t *testing.T) {
	security.ConfigureKey([]byte(strings.Repeat("A", 32)))
	mac := mustMac(t, correctMac)
	masked := mac.Mask().(MacAddress)
	if masked == mac || masked != mac.Mask().(MacAddress) {
		t.Fatal("mask not deterministic or equal to the original")
	}
	b0 := masked.ToBytes()[0]
	if b0&0x02 == 0 || b0&0x01 != 0 {
		t.Fatal("mask not locally administered")
	}

	security.ConfigureKey([]byte(strings.Repeat("B", 32)))
	if mac.Mask().(MacAddress) == masked {
		t.Fatal("mask does not depend on the key")
	}
}

func TestMacSensitive(t *testing.T) {
	var _ security.ARSensitive = MacAddress{}
	m := mustMac(t, correctMac)
	if m.RevealLevel() != security.ARSecurityLevelReasonable || !m.Maskable() {
		t.Fatal("bad classification")
	}
	// Logged at the default (SANITIZED) log level: masked
	if got := m.LogValue().String(); got == correctMac || got != m.Mask().(MacAddress).String() {
		t.Fatalf("LogValue = %q", got)
	}
}
