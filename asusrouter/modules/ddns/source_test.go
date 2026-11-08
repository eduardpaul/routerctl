package ddns

import (
	"context"
	"testing"

	"github.com/eduardpaul/routerctl/asusrouter/modules/nvram"
	"github.com/eduardpaul/routerctl/asusrouter/modules/source"
	"github.com/eduardpaul/routerctl/asusrouter/registry"
	"github.com/eduardpaul/routerctl/asusrouter/tools/identifiers"
)

// nvramData keys raw nvram values by their item ID, like the fetch result
func nvramData(values map[nvram.ARNvramType]any) map[source.ID]any {
	out := map[source.ID]any{}
	for k, v := range values {
		out[source.IDOf(k)] = v
	}
	return out
}

var fullData = map[nvram.ARNvramType]any{
	"ddns_enable_x":        "1",
	"ddns_server_x":        "WWW.ASUS.COM",
	"ddns_hostname_x":      "myhome.asuscomm.com",
	"ddns_hostname_old":    "taken.asuscomm.com",
	"ddns_username_x":      "user@example.com",
	"ddns_passwd_x":        "secret",
	"ddns_ipaddr":          "1.2.3.4",
	"ddns_ipv6_update":     "1",
	"ddns_refresh_x":       "21",
	"ddns_regular_check":   "1",
	"ddns_regular_period":  "60",
	"ddns_replace_status":  "1",
	"ddns_return_code_chk": "register,230",
	"ddns_updated":         "1",
	"ddns_wan_unit":        "-1",
	"ddns_wildcard_x":      "0",
	"asusddns_token_state": "0",
}

func TestFetchState(t *testing.T) {
	ctx := context.Background()

	var requested []source.DataSource
	values := nvramData(map[nvram.ARNvramType]any{nvram.ARNvramTypeDdnsState: "1"})
	c := &registry.Context{FetchDataMulti: func(_ context.Context, srcs []source.DataSource, _ registry.Kwargs) (map[source.ID]any, error) {
		requested = srcs
		return values, nil
	}}
	got, _ := fetchState(ctx, c, ARDdnsSourceUniversal)
	if len(got.(map[source.ID]any)) != 1 {
		t.Errorf("fetches_via_nvram: %v", got)
	}
	want := map[source.DataSource]bool{}
	for _, key := range []nvram.ARNvramType{
		nvram.ARNvramTypeDdnsHostname, nvram.ARNvramTypeDdnsHostnameOld, nvram.ARNvramTypeDdnsIpAddress,
		nvram.ARNvramTypeDdnsIpv6Update, nvram.ARNvramTypeDdnsPassword, nvram.ARNvramTypeDdnsRefreshInterval,
		nvram.ARNvramTypeDdnsRegularCheck, nvram.ARNvramTypeDdnsRegularPeriod, nvram.ARNvramTypeDdnsReplaceStatus,
		nvram.ARNvramTypeDdnsReturnCodeChk, nvram.ARNvramTypeDdnsServer, nvram.ARNvramTypeDdnsState,
		nvram.ARNvramTypeDdnsTokenState, nvram.ARNvramTypeDdnsUpdated, nvram.ARNvramTypeDdnsUsername,
		nvram.ARNvramTypeDdnsWanUnit, nvram.ARNvramTypeDdnsWildcard,
	} {
		want[key] = true
	}
	if len(requested) != len(want) {
		t.Errorf("requested %v", requested)
	}
	for _, src := range requested {
		if !want[src] {
			t.Errorf("unexpected %v", src)
		}
	}

	if got, _ := fetchState(ctx, &registry.Context{}, ARDdnsSourceUniversal); len(got.(map[source.ID]any)) != 0 {
		t.Errorf("no_get_data_callback: %v", got)
	}
	none := func(context.Context, []source.DataSource, registry.Kwargs) (map[source.ID]any, error) {
		return nil, nil
	}
	if got, _ := fetchState(ctx, &registry.Context{FetchDataMulti: none}, ARDdnsSourceUniversal); len(got.(map[source.ID]any)) != 0 {
		t.Errorf("non_dict_response: %v", got)
	}
}

func TestReadStatus(t *testing.T) {
	tests := []struct {
		raw  any
		want ARDdnsStatus
	}{
		{nil, ARDdnsStatusNone},
		{"", ARDdnsStatusNone},
		{"Time-out", ARDdnsStatusTimeout},
		{"unknown_error", ARDdnsStatusUnknownError},
		{"connect_fail", ARDdnsStatusConnectFailed},
		{"no_change", ARDdnsStatusNoChange},
		{"ddns_query", ARDdnsStatusQuery},
		{"auth_fail", ARDdnsStatusAuthFailed},
		{"200", ARDdnsStatusSuccess},
		{"register,200", ARDdnsStatusSuccess},
		{"register,230", ARDdnsStatusRegisteredNew},
		{"unregister,-1", ARDdnsStatusError},
		{"203", ARDdnsStatusDomainTaken},
		{"220", ARDdnsStatusRegisteredOriginal},
		{"233", ARDdnsStatusNewDomainTaken},
		{"296", ARDdnsStatusNotRegistered},
		{"297", ARDdnsStatusInvalidHostname},
		{"298", ARDdnsStatusInvalidDomain},
		{"299", ARDdnsStatusInvalidIp},
		{"390", ARDdnsStatusServerError},
		{"401", ARDdnsStatusUnauthorized},
		{"402", ARDdnsStatusFirmwareUpdateRequired},
		{"407", ARDdnsStatusProxyAuthRequired},
		{"something else", ARDdnsStatusUnknown},
		{42, ARDdnsStatusNone},
	}
	for _, tt := range tests {
		if got := ReadStatus(tt.raw); got != tt.want {
			t.Errorf("ReadStatus(%v) = %q, want %q", tt.raw, got, tt.want)
		}
	}
}

func TestTranslateState(t *testing.T) {
	for _, data := range []any{nil, map[source.ID]any{}, "text", []any{}} {
		if got := translateState(data, nil).(ARDdnsConfig); len(got) != 0 {
			t.Errorf("empty %v: %v", data, got)
		}
	}

	r := translateState(nvramData(fullData), nil).(ARDdnsConfig)
	str := func(v any) string {
		switch x := v.(type) {
		case identifiers.Hostname:
			return x.String()
		case identifiers.IpAddress:
			return x.String()
		}
		return "?"
	}
	checks := []struct {
		field ARDdnsField
		ok    bool
	}{
		{ARDdnsFieldState, r[ARDdnsFieldState] == true},
		{ARDdnsFieldServer, r[ARDdnsFieldServer] == ARDdnsServerAsus},
		{ARDdnsFieldHostname, str(r[ARDdnsFieldHostname]) == "myhome.asuscomm.com"},
		{ARDdnsFieldHostnameOld, str(r[ARDdnsFieldHostnameOld]) == "taken.asuscomm.com"},
		{ARDdnsFieldUsername, r[ARDdnsFieldUsername] == "user@example.com"},
		{ARDdnsFieldPassword, r[ARDdnsFieldPassword].(identifiers.Password).Value() == "secret"},
		{ARDdnsFieldIpAddress, str(r[ARDdnsFieldIpAddress]) == "1.2.3.4"},
		{ARDdnsFieldIpv6Update, r[ARDdnsFieldIpv6Update] == true},
		{ARDdnsFieldRefreshInterval, r[ARDdnsFieldRefreshInterval] == 21},
		{ARDdnsFieldVerification, r[ARDdnsFieldVerification] == true},
		{ARDdnsFieldVerificationPeriod, r[ARDdnsFieldVerificationPeriod] == 60},
		{ARDdnsFieldReplaceStatus, r[ARDdnsFieldReplaceStatus] == true},
		{ARDdnsFieldStatus, r[ARDdnsFieldStatus] == ARDdnsStatusRegisteredNew},
		{ARDdnsFieldTokenState, r[ARDdnsFieldTokenState] == false},
		{ARDdnsFieldUpdated, r[ARDdnsFieldUpdated] == true},
		{ARDdnsFieldWanUnit, r[ARDdnsFieldWanUnit] == -1},
		{ARDdnsFieldWildcard, r[ARDdnsFieldWildcard] == false},
	}
	for _, c := range checks {
		if !c.ok {
			t.Errorf("full %s: %#v", c.field, r[c.field])
		}
	}

	m := translateState(nvramData(map[nvram.ARNvramType]any{"ddns_enable_x": "0"}), nil).(ARDdnsConfig)
	if m[ARDdnsFieldState] != false || m[ARDdnsFieldStatus] != ARDdnsStatusNone {
		t.Errorf("missing_keys: %v", m)
	}
	for field, value := range m {
		if field != ARDdnsFieldState && field != ARDdnsFieldStatus && value != nil {
			t.Errorf("missing_keys %s: %v", field, value)
		}
	}

	for raw, want := range map[string]ARDdnsServer{
		"WWW.DYNDNS.ORG(CUSTOM)": ARDdnsServerDyndnsCustom,
		"NOT.A.SERVER":           ARDdnsServerUnknown,
	} {
		if got := translateState(nvramData(map[nvram.ARNvramType]any{"ddns_server_x": raw}), nil).(ARDdnsConfig)[ARDdnsFieldServer]; got != want {
			t.Errorf("server %s: %v", raw, got)
		}
	}
}

func TestActiveStatuses(t *testing.T) {
	if len(ActiveStatuses) != 3 || !ActiveStatuses[ARDdnsStatusSuccess] {
		t.Errorf("%v", ActiveStatuses)
	}
}
