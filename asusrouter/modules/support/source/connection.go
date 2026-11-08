package source

import (
	"github.com/eduardpaul/routerctl/asusrouter/modules/connection"
	"github.com/eduardpaul/routerctl/asusrouter/modules/support"
)

var translateConnection = support.MakeListTranslator([]support.TableEntry[connection.ARConnection]{
	{Key: string(support.ARSupportValueConnectionHttps), Value: connection.ARConnectionHttps},
	{Key: string(support.ARSupportValueConnectionSsh), Value: connection.ARConnectionSsh},
})
