package clients

import (
	"testing"

	"github.com/eduardpaul/routerctl/asusrouter/modules/common"
	"github.com/eduardpaul/routerctl/asusrouter/modules/wifi"
)

func TestARClientLinkDefaults(t *testing.T) {
	link := NewARClientLink()
	if link.Frequency != wifi.ARWiFiFrequencyUnknown || link.Band != nil || link.Mac != nil ||
		link.Rssi != nil || link.RxSpeed != nil || link.TxSpeed != nil || link.ConnectedSince != nil {
		t.Errorf("link = %+v", link)
	}
}

func TestARClientConnectionDefaults(t *testing.T) {
	conn := NewARClientConnection()
	if conn.Type != common.ARConnectionTypeUnknown || conn.Ip != nil || conn.IpMethod != common.ARIPMethodUnknown ||
		conn.InternetAccess != nil || conn.Guest || conn.Mlo || conn.Links == nil || len(conn.Links) != 0 {
		t.Errorf("connection = %+v", conn)
	}

	// Links are independent between instances
	first := NewARClientConnection()
	first.Links = append(first.Links, NewARClientLink())
	if second := NewARClientConnection(); len(second.Links) != 0 {
		t.Error("links should be independent")
	}
}

func TestARClientDefaults(t *testing.T) {
	client := NewARClient(mac(c1))
	if client.Mac != mac(c1) || client.Online || client.Name != nil ||
		client.DeviceType != common.ARDeviceTypeUnknown || client.InternetMode != common.ARInternetModeUnknown ||
		client.Connection != nil {
		t.Errorf("client = %+v", client)
	}
}
