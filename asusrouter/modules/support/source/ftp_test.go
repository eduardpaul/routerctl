package source

import (
	"testing"

	"github.com/eduardpaul/routerctl/asusrouter/modules/ftp"
	"github.com/eduardpaul/routerctl/asusrouter/modules/support"
)

func TestTranslateFtp(t *testing.T) {
	check(t, "translateFtp", translateFtp, []boolCase{
		{d{k(support.ARSupportValueFtp): 1}, false},
		{d{k(support.ARSupportValueFtp): "0"}, true},
		{d{}, true},
	})
}

func TestTranslateFtpCapabilities(t *testing.T) {
	check(t, "translateFtpCapabilities", translateFtpCapabilities, []struct {
		data     d
		expected []ftp.ARFTPCapability
	}{
		{d{k(support.ARSupportValueFtpSsl): 1}, []ftp.ARFTPCapability{ftp.ARFTPCapabilitySsl}},
		{d{}, []ftp.ARFTPCapability{}},
	})
}
