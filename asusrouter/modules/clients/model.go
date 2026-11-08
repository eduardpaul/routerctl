// Package clients mirrors asusrouter/modules/clients/.
//
// `__init__.py` only re-exports; the count subpackage is
// modules/clients/count (it imports this package, so this package cannot
// re-export it).
package clients

import (
	"time"

	"github.com/eduardpaul/routerctl/asusrouter/modules/common"
	"github.com/eduardpaul/routerctl/asusrouter/modules/wifi"
	"github.com/eduardpaul/routerctl/asusrouter/tools/identifiers"
)

// ARClientLink mirrors `ARClientLink`: one radio link of a client's
// wireless connection.
type ARClientLink struct {
	Frequency      wifi.ARWiFiFrequency    `json:"frequency"`
	Band           *wifi.ARWiFiBand        `json:"band"`
	Mac            *identifiers.MacAddress `json:"mac"`
	Rssi           *int                    `json:"rssi"`
	RxSpeed        *float64                `json:"rx_speed"`
	TxSpeed        *float64                `json:"tx_speed"`
	ConnectedSince *time.Time              `json:"connected_since"`
}

// NewARClientLink returns a link with the Python dataclass defaults.
func NewARClientLink() ARClientLink {
	return ARClientLink{Frequency: wifi.ARWiFiFrequencyUnknown}
}

// ARClientConnection mirrors `ARClientConnection`: the current connection
// of an online client.
type ARClientConnection struct {
	Type           common.ARConnectionType `json:"type"`
	Ip             *identifiers.IpAddress  `json:"ip"`
	IpMethod       common.ARIPMethod       `json:"ip_method"`
	InternetAccess *bool                   `json:"internet_access"`
	Node           *identifiers.MacAddress `json:"node"`
	Ssid           *string                 `json:"ssid"`
	Security       *wifi.ARWiFiAuth        `json:"security"`
	Guest          bool                    `json:"guest"`
	Sdn            *string                 `json:"sdn"`
	Mlo            bool                    `json:"mlo"`
	ConnectedSince *time.Time              `json:"connected_since"`
	Links          []ARClientLink          `json:"links"`
}

// NewARClientConnection returns a connection with the Python dataclass
// defaults.
func NewARClientConnection() ARClientConnection {
	return ARClientConnection{
		Type:     common.ARConnectionTypeUnknown,
		IpMethod: common.ARIPMethodUnknown,
		Links:    []ARClientLink{},
	}
}

// ARClient mirrors `ARClient`: a network client (identity and
// preferences).
type ARClient struct {
	Mac          identifiers.MacAddress  `json:"mac"`
	Online       bool                    `json:"online"`
	Name         *string                 `json:"name"`
	Vendor       *string                 `json:"vendor"`
	VendorClass  *string                 `json:"vendor_class"`
	DeviceType   common.ARDeviceType     `json:"device_type"`
	OsType       *int                    `json:"os_type"`
	BoundNode    *identifiers.MacAddress `json:"bound_node"`
	InternetMode common.ARInternetMode   `json:"internet_mode"`
	Connection   *ARClientConnection     `json:"connection"`
}

// NewARClient returns a client with the Python dataclass defaults.
func NewARClient(mac identifiers.MacAddress) ARClient {
	return ARClient{
		Mac:          mac,
		DeviceType:   common.ARDeviceTypeUnknown,
		InternetMode: common.ARInternetModeUnknown,
	}
}
