// IP address tools (ip.py).

package identifiers

import (
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"net"
	"net/netip"
	"strconv"
	"strings"

	"github.com/eduardpaul/routerctl/asusrouter/tools/converters"
	"github.com/eduardpaul/routerctl/asusrouter/tools/security"
)

const (
	IpVersionV4      = 4
	IpLengthBytesV4  = 4
	IpLengthBytesV6  = 16
	ipMaxPrefixlenV4 = 32
	ipMaxPrefixlenV6 = 128
)

const (
	ErrorIpBytes           = "IP address must be 4 bytes (IPv4) or 16 bytes (IPv6)"
	ErrorIpInt             = "IP address integer out of range"
	ErrorIpStr             = "Invalid IP address string"
	ErrorIpUnsupportedType = "Unsupported IP address type"
)

// IpAddress mirrors `IpAddress`: an IP address representation supporting
// IPv4 and IPv6 (IPv6 may carry a zone, like Python's scope id).
//
// It is comparable (usable as a map key).
type IpAddress struct {
	addr netip.Addr
}

// NewIpAddress mirrors `IpAddress(ip)`.
func NewIpAddress(ip any) (IpAddress, error) {
	addr, err := ipToAddr(ip)
	if err != nil {
		return IpAddress{}, err
	}
	return IpAddress{addr: addr}, nil
}

// ipToAddr mirrors `_to_addr`: convert supported values to an address.
//
// Supported inputs:
//   - IpAddress (or pointer) -> returns underlying address
//   - netip.Addr / net.IP -> used directly (net.IP holding an IPv4
//     address is IPv4)
//   - []byte: 4 bytes (IPv4) or 16 bytes (IPv6)
//   - integers (Go ints, *big.Int): in [0, 2**128)
//   - string: any valid IPv4 or IPv6 notation
func ipToAddr(value any) (netip.Addr, error) {
	if p, ok := value.(*string); ok && p != nil {
		value = *p
	}
	switch v := value.(type) {
	// Already an IpAddress instance
	case IpAddress:
		return v.addr, nil
	case *IpAddress:
		if v != nil {
			return v.addr, nil
		}
	// Stdlib address types
	case netip.Addr:
		if v.IsValid() {
			return v, nil
		}
		return netip.Addr{}, errors.New(ErrorIpStr)
	case net.IP:
		if v4 := v.To4(); v4 != nil {
			return netip.AddrFrom4([4]byte(v4)), nil
		}
		return ipFromBytes(v)
	// Bytes-like
	case []byte:
		return ipFromBytes(v)
	// Integer
	case bool:
		if v {
			return ipFromInt(big.NewInt(1))
		}
		return ipFromInt(big.NewInt(0))
	case *big.Int:
		if v != nil {
			return ipFromInt(v)
		}
	case big.Int:
		return ipFromInt(&v)
	case uint:
		return ipFromInt(new(big.Int).SetUint64(uint64(v)))
	case uint64:
		return ipFromInt(new(big.Int).SetUint64(v))
	// String
	case string:
		return parseIpAddress(converters.PyStrip(v))
	}
	if i, ok := goInt(value); ok {
		return ipFromInt(big.NewInt(i))
	}

	return netip.Addr{}, fmt.Errorf("%s: %s", ErrorIpUnsupportedType, pyType(value))
}

// goInt returns the value of the signed (and small unsigned) Go integer
// types.
func goInt(value any) (int64, bool) {
	switch v := value.(type) {
	case int:
		return int64(v), true
	case int8:
		return int64(v), true
	case int16:
		return int64(v), true
	case int32:
		return int64(v), true
	case int64:
		return v, true
	case uint8:
		return int64(v), true
	case uint16:
		return int64(v), true
	case uint32:
		return int64(v), true
	}
	return 0, false
}

func ipFromBytes(b []byte) (netip.Addr, error) {
	if len(b) != IpLengthBytesV4 && len(b) != IpLengthBytesV6 {
		return netip.Addr{}, errors.New(ErrorIpBytes)
	}
	addr, _ := netip.AddrFromSlice(b)
	return addr, nil
}

var (
	ipMaxV4 = new(big.Int).Lsh(big.NewInt(1), 32)
	ipMaxV6 = new(big.Int).Lsh(big.NewInt(1), 128)
)

// ipFromInt mirrors `ip_address(int)`: IPv4 below 2**32, IPv6 below
// 2**128.
func ipFromInt(n *big.Int) (netip.Addr, error) {
	if n.Sign() < 0 || n.Cmp(ipMaxV6) >= 0 {
		return netip.Addr{}, errors.New(ErrorIpInt)
	}
	if n.Cmp(ipMaxV4) < 0 {
		var b [4]byte
		n.FillBytes(b[:])
		return netip.AddrFrom4(b), nil
	}
	var b [16]byte
	n.FillBytes(b[:])
	return netip.AddrFrom16(b), nil
}

// parseIpAddress mirrors `ip_address(str)`.
func parseIpAddress(text string) (netip.Addr, error) {
	// Python takes the scope id after the first `%` and refuses `/`
	if strings.Contains(text, "/") || strings.Count(text, "%") > 1 {
		return netip.Addr{}, errors.New(ErrorIpStr)
	}
	addr, err := netip.ParseAddr(text)
	if err != nil {
		return netip.Addr{}, errors.New(ErrorIpStr)
	}
	return addr, nil
}

// IpAddressFromValue mirrors `IpAddress.from_value`.
func IpAddressFromValue(value any) (IpAddress, error) {
	return NewIpAddress(value)
}

// IpAddressFromValueSafe mirrors `IpAddress.from_value_safe`: nil when
// the value cannot be parsed as an IP address.
func IpAddressFromValueSafe(value any) *IpAddress {
	ip, err := IpAddressFromValue(value)
	if err != nil {
		return nil
	}
	return &ip
}

// Addr returns the underlying netip address.
func (ip IpAddress) Addr() netip.Addr { return ip.addr }

// Version returns the IP version (4 or 6).
func (ip IpAddress) Version() int {
	if ip.addr.Is4() {
		return 4
	}
	return 6
}

var (
	ipv4PrivateNetworks = mustPrefixes(
		"0.0.0.0/8", "10.0.0.0/8", "127.0.0.0/8", "169.254.0.0/16",
		"172.16.0.0/12", "192.0.0.0/24", "192.0.0.170/31", "192.0.2.0/24",
		"192.168.0.0/16", "198.18.0.0/15", "198.51.100.0/24",
		"203.0.113.0/24", "240.0.0.0/4", "255.255.255.255/32",
	)
	ipv4PrivateNetworksExceptions = mustPrefixes("192.0.0.9/32", "192.0.0.10/32")
	ipv6PrivateNetworks           = mustPrefixes(
		"::1/128", "::/128", "::ffff:0:0/96", "64:ff9b:1::/48", "100::/64",
		"2001::/23", "2001:db8::/32", "2002::/16", "3fff::/20", "fc00::/7",
		"fe80::/10",
	)
	ipv6PrivateNetworksExceptions = mustPrefixes(
		"2001:1::1/128", "2001:1::2/128", "2001:3::/32", "2001:4:112::/48",
		"2001:20::/28", "2001:30::/28",
	)
)

func mustPrefixes(prefixes ...string) []netip.Prefix {
	out := make([]netip.Prefix, len(prefixes))
	for i, p := range prefixes {
		out[i] = netip.MustParsePrefix(p)
	}
	return out
}

func inAny(addr netip.Addr, networks []netip.Prefix) bool {
	for _, network := range networks {
		if network.Contains(addr) {
			return true
		}
	}
	return false
}

// IsPrivate mirrors Python's `is_private` (CPython 3.13 tables; an
// IPv4-mapped IPv6 address follows its IPv4 address).
func (ip IpAddress) IsPrivate() bool {
	addr := ip.addr.WithZone("")
	if addr.Is4In6() {
		addr = addr.Unmap()
	}
	if addr.Is4() {
		return inAny(addr, ipv4PrivateNetworks) && !inAny(addr, ipv4PrivateNetworksExceptions)
	}
	return inAny(addr, ipv6PrivateNetworks) && !inAny(addr, ipv6PrivateNetworksExceptions)
}

// IsLoopback mirrors Python's `is_loopback`.
func (ip IpAddress) IsLoopback() bool {
	addr := ip.addr.WithZone("")
	if addr.Is4In6() {
		addr = addr.Unmap()
	}
	if addr.Is4() {
		return addr.As4()[0] == 127
	}
	return addr == netip.IPv6Loopback()
}

// ToBytes returns the packed bytes (4 bytes for IPv4, 16 bytes for IPv6).
func (ip IpAddress) ToBytes() []byte {
	return ip.addr.AsSlice()
}

// ToInt returns the integer representation of the IP address.
func (ip IpAddress) ToInt() *big.Int {
	return new(big.Int).SetBytes(ip.addr.AsSlice())
}

// RevealLevel returns REASONABLE.
func (ip IpAddress) RevealLevel() security.ARSecurityLevel {
	return security.ARSecurityLevelReasonable
}

// Maskable reports true.
func (ip IpAddress) Maskable() bool { return true }

// Mask returns a deterministic pseudo-address of the same version (an
// IpAddress).
func (ip IpAddress) Mask() any {
	size := IpLengthBytesV6
	if ip.addr.Is4() {
		size = IpLengthBytesV4
	}
	addr, _ := netip.AddrFromSlice(security.HmacDigest(ip.addr.AsSlice(), nil)[:size])
	return IpAddress{addr: addr}
}

// CanonicalValue returns the raw string form.
func (ip IpAddress) CanonicalValue() any { return ip.String() }

// String returns the string representation of the IP address.
func (ip IpAddress) String() string {
	if !ip.addr.IsValid() {
		return ""
	}
	return ip.addr.String()
}

// GoString returns the Python repr (same as String).
func (ip IpAddress) GoString() string { return ip.String() }

// MarshalText returns the raw string form.
func (ip IpAddress) MarshalText() ([]byte, error) { return []byte(ip.String()), nil }

// LogValue renders the address at the log security level.
func (ip IpAddress) LogValue() slog.Value { return security.LogValue(ip) }

// Equal mirrors `__eq__`: other is parsed as an IP address; values that
// cannot be parsed are not equal.
func (ip IpAddress) Equal(other any) bool {
	o, err := IpAddressFromValue(other)
	return err == nil && o == ip
}

// IpInterface mirrors `IpInterface`: an IP interface (address with a
// network prefix, e.g. `10.55.0.1/24`). It is comparable.
type IpInterface struct {
	addr      netip.Addr
	prefixlen int
}

// NewIpInterface mirrors `IpInterface(iface)`.
func NewIpInterface(iface any) (IpInterface, error) {
	return ipToIface(iface)
}

// ipToIface mirrors `_to_iface`.
//
// Supported inputs:
//   - IpInterface (or pointer) -> returns underlying interface
//   - netip.Prefix -> used directly (host bits kept)
//   - IpAddress / netip.Addr / net.IP -> host address (`/32`, `/128`)
//   - string: `address` or `address/prefix` notation
func ipToIface(value any) (IpInterface, error) {
	if p, ok := value.(*string); ok && p != nil {
		value = *p
	}
	switch v := value.(type) {
	// Already an IpInterface instance
	case IpInterface:
		return v, nil
	case *IpInterface:
		if v != nil {
			return *v, nil
		}
	// Stdlib interface types
	case netip.Prefix:
		if v.IsValid() {
			return IpInterface{addr: v.Addr(), prefixlen: v.Bits()}, nil
		}
		return IpInterface{}, errors.New(ErrorIpStr)
	// Address types collapse to a host interface
	case IpAddress:
		value = v.String()
	case *IpAddress:
		if v != nil {
			value = v.String()
		}
	case netip.Addr:
		value = v.String()
	case net.IP:
		if addr, err := ipToAddr(v); err == nil {
			value = addr.String()
		}
	}

	if s, ok := value.(string); ok {
		iface, ok := parseIpInterface(converters.PyStrip(s))
		if !ok {
			return IpInterface{}, errors.New(ErrorIpStr)
		}
		return iface, nil
	}

	return IpInterface{}, fmt.Errorf("%s: %s", ErrorIpUnsupportedType, pyType(value))
}

// parseIpInterface mirrors `ip_interface(str)`.
func parseIpInterface(text string) (IpInterface, bool) {
	parts := strings.Split(text, "/")
	if len(parts) > 2 {
		return IpInterface{}, false
	}
	addr, err := parseIpAddress(parts[0])
	if err != nil {
		return IpInterface{}, false
	}
	maxPrefixlen := ipMaxPrefixlenV6
	if addr.Is4() {
		maxPrefixlen = ipMaxPrefixlenV4
	}
	if len(parts) == 1 {
		return IpInterface{addr: addr, prefixlen: maxPrefixlen}, true
	}
	prefixlen, ok := parsePrefixlen(parts[1], addr.Is4(), maxPrefixlen)
	if !ok {
		return IpInterface{}, false
	}
	return IpInterface{addr: addr, prefixlen: prefixlen}, true
}

// parsePrefixlen mirrors `_make_netmask` for a string: a decimal prefix
// length, or (IPv4 only) a netmask or hostmask.
func parsePrefixlen(text string, v4 bool, maxPrefixlen int) (int, bool) {
	digits := text != ""
	for _, c := range text {
		if c < '0' || c > '9' {
			digits = false
			break
		}
	}
	if digits {
		n, err := strconv.Atoi(text)
		if err != nil || n < 0 || n > maxPrefixlen {
			return 0, false
		}
		return n, true
	}
	if !v4 {
		return 0, false
	}
	mask, err := netip.ParseAddr(text)
	if err != nil || !mask.Is4() {
		return 0, false
	}
	b := mask.As4()
	n := uint32(b[0])<<24 | uint32(b[1])<<16 | uint32(b[2])<<8 | uint32(b[3])
	if prefix, ok := prefixFromMask(n); ok {
		return prefix, true
	}
	// Try matching a hostmask
	return prefixFromMask(^n)
}

// prefixFromMask returns the prefix length of a contiguous netmask.
func prefixFromMask(n uint32) (int, bool) {
	ones := 0
	for ones < 32 && n&(1<<(31-ones)) != 0 {
		ones++
	}
	if ones < 32 && n<<ones != 0 {
		return 0, false
	}
	return ones, true
}

// IpInterfaceFromValue mirrors `IpInterface.from_value`.
func IpInterfaceFromValue(value any) (IpInterface, error) {
	return ipToIface(value)
}

// IpInterfaceFromValueSafe mirrors `IpInterface.from_value_safe`: nil if
// the value cannot be parsed.
func IpInterfaceFromValueSafe(value any) *IpInterface {
	iface, err := IpInterfaceFromValue(value)
	if err != nil {
		return nil
	}
	return &iface
}

// Version returns the IP version (4 or 6).
func (i IpInterface) Version() int { return i.Ip().Version() }

// Ip returns the host address without the prefix.
func (i IpInterface) Ip() IpAddress { return IpAddress{addr: i.addr} }

// Prefixlen returns the network prefix length.
func (i IpInterface) Prefixlen() int { return i.prefixlen }

// RevealLevel returns REASONABLE.
func (i IpInterface) RevealLevel() security.ARSecurityLevel {
	return security.ARSecurityLevelReasonable
}

// Maskable reports true.
func (i IpInterface) Maskable() bool { return true }

// Mask returns a pseudo-interface: masked address, original prefix (an
// IpInterface).
func (i IpInterface) Mask() any {
	masked := i.Ip().Mask().(IpAddress)
	return IpInterface{addr: masked.addr, prefixlen: i.prefixlen}
}

// CanonicalValue returns the raw `address/prefix` string.
func (i IpInterface) CanonicalValue() any { return i.String() }

// String returns the `address/prefix` representation.
func (i IpInterface) String() string {
	if !i.addr.IsValid() {
		return ""
	}
	return i.addr.String() + "/" + strconv.Itoa(i.prefixlen)
}

// GoString returns the Python repr (same as String).
func (i IpInterface) GoString() string { return i.String() }

// MarshalText returns the raw `address/prefix` string.
func (i IpInterface) MarshalText() ([]byte, error) { return []byte(i.String()), nil }

// LogValue renders the interface at the log security level.
func (i IpInterface) LogValue() slog.Value { return security.LogValue(i) }

// Equal mirrors `__eq__`: other is parsed as an IP interface; values that
// cannot be parsed are not equal.
func (i IpInterface) Equal(other any) bool {
	o, err := IpInterfaceFromValue(other)
	return err == nil && o == i
}

// ReadIpList mirrors `read_ip_list`: read a whitespace-separated string
// into a list of IP addresses.
//
// Returns every address that parses; an empty list when none do.
func ReadIpList(value any) []IpAddress {
	text, ok := value.(string)
	if !ok {
		return []IpAddress{}
	}
	result := []IpAddress{}
	for _, part := range converters.PySplit(text) {
		if ip := IpAddressFromValueSafe(part); ip != nil {
			result = append(result, *ip)
		}
	}
	return result
}

// ReadIpInterfaceList mirrors `read_ip_interface_list`: read a
// comma/whitespace-separated string into a list of IP interfaces.
//
// Returns every interface that parses; an empty list when none do.
func ReadIpInterfaceList(value any) []IpInterface {
	text, ok := value.(string)
	if !ok {
		return []IpInterface{}
	}
	result := []IpInterface{}
	for _, part := range converters.PySplit(strings.ReplaceAll(text, ",", " ")) {
		if iface := IpInterfaceFromValueSafe(part); iface != nil {
			result = append(result, *iface)
		}
	}
	return result
}
