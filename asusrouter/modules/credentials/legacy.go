package credentials

import (
	"github.com/eduardpaul/routerctl/asusrouter/consts"
	"github.com/eduardpaul/routerctl/asusrouter/modules/common"
	"github.com/eduardpaul/routerctl/asusrouter/modules/nvram"
	"github.com/eduardpaul/routerctl/asusrouter/tools/writers"
)

// Services the WebUI runs on apply; the password persists from the nvram write
var legacyServices = []common.ARService{common.ARServiceTimeRestart, common.ARServiceUpnpRestart}
var legacyActionScript = func() string {
	script := ""
	for _, service := range legacyServices {
		script += string(service) + ";"
	}
	return script
}()

// BuildLegacyRequest mirrors `build_legacy_request`: build login
// credentials change body.
func BuildLegacyRequest(username string, password string) string {
	data := []writers.KeyValue{
		{Key: common.ActionModeKey, Value: string(common.ARActionModeApply)},
		{Key: "action_script", Value: legacyActionScript},
		{Key: string(nvram.ARNvramTypeHttpUsername), Value: username},
		{Key: string(nvram.ARNvramTypeHttpPassword), Value: password},
		// Confirmation fields the WebUI form submits alongside the password
		{Key: "http_passwd2", Value: password},
		{Key: "v_password2", Value: password},
	}
	return writers.DictToRequest(data, consts.RequestTypeGet)
}
