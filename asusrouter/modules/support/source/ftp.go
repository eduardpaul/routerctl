package source

import (
	"github.com/eduardpaul/routerctl/asusrouter/modules/ftp"
	"github.com/eduardpaul/routerctl/asusrouter/modules/support"
)

var translateFtp = support.MakeBoolTranslator(string(support.ARSupportValueFtp), true)
var translateFtpCapabilities = support.MakeListTranslator([]support.TableEntry[ftp.ARFTPCapability]{
	{Key: string(support.ARSupportValueFtpSsl), Value: ftp.ARFTPCapabilitySsl},
})
