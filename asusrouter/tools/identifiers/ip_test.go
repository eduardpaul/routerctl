package identifiers

import (
	"math/big"
	"net/netip"
	"reflect"
	"strings"
	"testing"

	"github.com/eduardpaul/routerctl/asusrouter/tools/security"
)

const (
	correctIPv4 = "192.168.1.1"
	correctIPv6 = "2001:db8::1"
)

var (
	correctIPv4Bytes = []byte{192, 168, 1, 1}
	correctIPv4Int   = big.NewInt(3232235777)
	correctIPv6Bytes = netip.MustParseAddr(correctIPv6).AsSlice()
	correctIPv6Int   = new(big.Int).SetBytes(correctIPv6Bytes)
)

func mustIP(t *testing.T, v any) IpAddress {
	t.Helper()
	ip, err := IpAddressFromValue(v)
	if err != nil {
		t.Fatalf("IpAddressFromValue(%#v): %v", v, err)
	}
	return ip
}

func mustIface(t *testing.T, v any) IpInterface {
	t.Helper()
	iface, err := IpInterfaceFromValue(v)
	if err != nil {
		t.Fatalf("IpInterfaceFromValue(%#v): %v", v, err)
	}
	return iface
}

func expectIPErr(t *testing.T, v any, msg string) {
	t.Helper()
	if _, err := IpAddressFromValue(v); err == nil || !strings.Contains(err.Error(), msg) {
		t.Errorf("IpAddressFromValue(%#v) error = %v, want %q", v, err, msg)
	}
	if _, err := NewIpAddress(v); err == nil || !strings.Contains(err.Error(), msg) {
		t.Errorf("NewIpAddress(%#v) error = %v, want %q", v, err, msg)
	}
}

func TestIPFromInstance(t *testing.T) {
	instance := mustIP(t, correctIPv4)
	if mustIP(t, instance) != instance {
		t.Fatal("instance not passed through")
	}
}

func TestIPFromStdlib(t *testing.T) {
	cases := map[netip.Addr]string{
		netip.MustParseAddr(correctIPv4): correctIPv4,
		netip.MustParseAddr(correctIPv6): correctIPv6,
	}
	for addr, want := range cases {
		if got := mustIP(t, addr).String(); got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	}
}

func TestIPFromBytes(t *testing.T) {
	cases := []struct {
		value    []byte
		expected string
	}{
		{correctIPv4Bytes, correctIPv4},
		{correctIPv6Bytes, correctIPv6},
	}
	for _, c := range cases {
		if got := mustIP(t, c.value).String(); got != c.expected {
			t.Errorf("got %q, want %q", got, c.expected)
		}
	}
	for _, v := range [][]byte{make([]byte, 3), make([]byte, 5), make([]byte, 17)} {
		expectIPErr(t, v, ErrorIpBytes)
	}
}

func TestIPFromInt(t *testing.T) {
	two32 := new(big.Int).Lsh(big.NewInt(1), 32)
	cases := []struct {
		value    any
		expected string
	}{
		{correctIPv4Int, correctIPv4},
		{correctIPv6Int, correctIPv6},
		{0, "0.0.0.0"},
		{int64(1<<32 - 1), "255.255.255.255"},
		{two32, "::1:0:0"},
		{1 << 32, "::1:0:0"},
	}
	for _, c := range cases {
		if got := mustIP(t, c.value).String(); got != c.expected {
			t.Errorf("IpAddress(%v) = %q, want %q", c.value, got, c.expected)
		}
	}
	for _, v := range []any{-1, new(big.Int).Lsh(big.NewInt(1), 128)} {
		expectIPErr(t, v, ErrorIpInt)
	}
}

func TestIPFromString(t *testing.T) {
	cases := map[string]string{
		correctIPv4:               correctIPv4,
		"  " + correctIPv4 + "  ": correctIPv4,
		"::1":                     "::1",
		correctIPv6:               correctIPv6,
		"2001:DB8:0:0:1:0:0:1":    "2001:db8::1:0:0:1",
		"::ffff:0102:0304":        "::ffff:1.2.3.4",
		"fe80::1%eth0":            "fe80::1%eth0",
	}
	for v, want := range cases {
		if got := mustIP(t, v).String(); got != want {
			t.Errorf("IpAddress(%q) = %q, want %q", v, got, want)
		}
	}
	for _, v := range []string{"not an ip", "999.999.999.999", "192.168.1", "01.2.3.4", "1.2.3.4/24"} {
		expectIPErr(t, v, ErrorIpStr)
	}
}

func TestIPFromUnsupported(t *testing.T) {
	for _, v := range []any{nil, struct{}{}} {
		expectIPErr(t, v, ErrorIpUnsupportedType)
	}
}

func TestIPFromValueSafe(t *testing.T) {
	if got := IpAddressFromValueSafe(correctIPv4); got == nil || *got != mustIP(t, correctIPv4) {
		t.Fatalf("got %v", got)
	}
	if IpAddressFromValueSafe(nil) != nil || IpAddressFromValueSafe(struct{}{}) != nil {
		t.Fatal("expected nil")
	}
}

func TestIPVersion(t *testing.T) {
	if mustIP(t, correctIPv4).Version() != 4 || mustIP(t, correctIPv6).Version() != 6 {
		t.Fatal("bad version")
	}
}

func TestIPIsPrivate(t *testing.T) {
	cases := map[string]bool{
		"192.168.1.1": true, "8.8.8.8": false, "fc00::1": true, "2606:4700::1": false,
		// Python tables
		"100.64.0.1": false, "192.0.0.9": false, "192.0.0.8": true, "::ffff:10.0.0.1": true,
		"2001:4:112::1": false, "2001:2::1": true,
	}
	for v, want := range cases {
		if got := mustIP(t, v).IsPrivate(); got != want {
			t.Errorf("IsPrivate(%q) = %v", v, got)
		}
	}
}

func TestIPIsLoopback(t *testing.T) {
	cases := map[string]bool{"127.0.0.1": true, "192.168.1.1": false, "::1": true, "::2": false, "::ffff:127.0.0.1": true}
	for v, want := range cases {
		if got := mustIP(t, v).IsLoopback(); got != want {
			t.Errorf("IsLoopback(%q) = %v", v, got)
		}
	}
}

func TestIPToBytesAndInt(t *testing.T) {
	if !reflect.DeepEqual(mustIP(t, correctIPv4).ToBytes(), correctIPv4Bytes) ||
		!reflect.DeepEqual(mustIP(t, correctIPv6).ToBytes(), correctIPv6Bytes) {
		t.Fatal("bad bytes")
	}
	if mustIP(t, correctIPv4).ToInt().Cmp(correctIPv4Int) != 0 || mustIP(t, correctIPv6).ToInt().Cmp(correctIPv6Int) != 0 {
		t.Fatal("bad int")
	}
}

func TestIPRepr(t *testing.T) {
	if mustIP(t, correctIPv4).GoString() != correctIPv4 || mustIP(t, correctIPv4).CanonicalValue() != correctIPv4 {
		t.Fatal("bad repr")
	}
}

func TestIPEq(t *testing.T) {
	ip := mustIP(t, correctIPv4)
	cases := []struct {
		value  any
		result bool
	}{
		{correctIPv4, true}, {"8.8.8.8", false},
		{mustIP(t, correctIPv4), true}, {mustIP(t, "8.8.8.8"), false},
		{struct{}{}, false},
	}
	for _, c := range cases {
		if got := ip.Equal(c.value); got != c.result {
			t.Errorf("Equal(%#v) = %v", c.value, got)
		}
	}
}

func TestReadIpList(t *testing.T) {
	v4, v6 := mustIP(t, correctIPv4), mustIP(t, correctIPv6)
	cases := []struct {
		value    any
		expected []IpAddress
	}{
		{correctIPv4, []IpAddress{v4}},
		{correctIPv4 + "  " + correctIPv6, []IpAddress{v4, v6}},
		{correctIPv4 + " nope", []IpAddress{v4}},
		{"", []IpAddress{}}, {"   ", []IpAddress{}}, {"nope", []IpAddress{}},
		{nil, []IpAddress{}}, {42, []IpAddress{}}, {[]any{}, []IpAddress{}},
	}
	for _, c := range cases {
		if got := ReadIpList(c.value); !reflect.DeepEqual(got, c.expected) {
			t.Errorf("ReadIpList(%#v) = %v", c.value, got)
		}
	}
}

func TestIPMask(t *testing.T) {
	security.ConfigureKey([]byte(strings.Repeat("A", 32)))
	for _, v := range []string{correctIPv4, correctIPv6} {
		ip := mustIP(t, v)
		masked := ip.Mask().(IpAddress)
		if masked.Version() != ip.Version() || masked == ip || masked != ip.Mask().(IpAddress) {
			t.Errorf("bad mask for %q", v)
		}
	}
	ip := mustIP(t, correctIPv4)
	m1 := ip.Mask()
	security.ConfigureKey([]byte(strings.Repeat("B", 32)))
	if ip.Mask() == m1 {
		t.Fatal("mask does not depend on the key")
	}
}

func TestIpInterface(t *testing.T) {
	// A stdlib interface is used directly
	if mustIface(t, netip.MustParsePrefix("10.0.0.1/24")).Prefixlen() != 24 {
		t.Fatal("stdlib prefix")
	}
	// from_value returns the same instance
	instance := mustIface(t, "10.0.0.1/24")
	if mustIface(t, instance) != instance {
		t.Fatal("instance not passed through")
	}
	// A CIDR string keeps its prefix
	iface := mustIface(t, "10.55.0.1/24")
	if iface.String() != "10.55.0.1/24" || iface.Prefixlen() != 24 || iface.Version() != 4 || iface.Ip() != mustIP(t, "10.55.0.1") {
		t.Fatal("cidr")
	}
	// A bare address becomes a host interface
	if mustIface(t, "10.55.0.1").Prefixlen() != 32 {
		t.Fatal("bare address")
	}
	// Address types collapse to host interfaces
	if mustIface(t, mustIP(t, "10.0.0.9")).Prefixlen() != 32 || mustIface(t, netip.MustParseAddr("10.0.0.9")).Prefixlen() != 32 {
		t.Fatal("address types")
	}
	// IPv6 interfaces are supported
	if v6 := mustIface(t, "2001:db8::1/64"); v6.Version() != 6 || v6.Prefixlen() != 64 {
		t.Fatal("ipv6")
	}
	// Netmask and hostmask notations
	for _, v := range []string{"1.2.3.4/255.255.255.0", "1.2.3.4/0.0.0.255"} {
		if got := mustIface(t, v).String(); got != "1.2.3.4/24" {
			t.Errorf("%q -> %q", v, got)
		}
	}
	// Invalid values
	if _, err := IpInterfaceFromValue("nonsense"); err == nil || !strings.Contains(err.Error(), ErrorIpStr) {
		t.Fatalf("invalid: %v", err)
	}
	if _, err := IpInterfaceFromValue(struct{}{}); err == nil || !strings.Contains(err.Error(), ErrorIpUnsupportedType) {
		t.Fatalf("unsupported: %v", err)
	}
	if IpInterfaceFromValueSafe("10.0.0.1/24") == nil || IpInterfaceFromValueSafe("nope") != nil {
		t.Fatal("safe")
	}
	if mustIface(t, "10.0.0.1/24").GoString() != "10.0.0.1/24" {
		t.Fatal("repr")
	}
}

func TestIpInterfaceEquality(t *testing.T) {
	iface := mustIface(t, "10.0.0.1/24")
	if iface != mustIface(t, "10.0.0.1/24") || !iface.Equal("10.0.0.1/24") || iface == mustIface(t, "10.0.0.1/25") {
		t.Fatal("equality")
	}
	if iface.Equal(123) || iface.Equal("not-an-ip") {
		t.Fatal("uncomparable")
	}
	if !(map[IpInterface]bool{iface: true})[mustIface(t, "10.0.0.1/24")] {
		t.Fatal("hashable")
	}
}

func TestIpInterfaceMask(t *testing.T) {
	security.ConfigureKey([]byte(strings.Repeat("A", 32)))
	iface := mustIface(t, "10.55.0.1/24")
	masked := iface.Mask().(IpInterface)
	if masked.Prefixlen() != 24 || masked.Ip() == iface.Ip() || masked != iface.Mask().(IpInterface) {
		t.Fatal("mask")
	}
}

func TestReadIpInterfaceList(t *testing.T) {
	cases := []struct {
		value    any
		expected []string
	}{
		{"10.0.0.1/32, 192.168.0.0/24", []string{"10.0.0.1/32", "192.168.0.0/24"}},
		{"10.0.0.1/32 192.168.0.0/24", []string{"10.0.0.1/32", "192.168.0.0/24"}},
		{"10.0.0.1/32,192.168.0.0/24", []string{"10.0.0.1/32", "192.168.0.0/24"}},
		{"10.0.0.1/32, nonsense", []string{"10.0.0.1/32"}},
		{"", []string{}}, {"   ", []string{}}, {nil, []string{}}, {123, []string{}},
	}
	for _, c := range cases {
		got := []string{}
		for _, i := range ReadIpInterfaceList(c.value) {
			got = append(got, i.String())
		}
		if !reflect.DeepEqual(got, c.expected) {
			t.Errorf("ReadIpInterfaceList(%#v) = %v", c.value, got)
		}
	}
}
