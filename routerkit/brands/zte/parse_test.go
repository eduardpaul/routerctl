package zte

import (
	"io/fs"
	"testing"

	"github.com/eduardpaul/routerctl/routerkit/brands/zte/ztefake"
)

func fixture(t *testing.T, name string) *Response {
	t.Helper()
	b, err := fs.ReadFile(ztefake.FixtureFS(), name)
	if err != nil {
		t.Fatal(err)
	}
	r, err := Parse(b)
	if err != nil {
		t.Fatalf("parse %s: %v", name, err)
	}
	return r
}

func TestParseTolerant(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		wantErr bool
		check   func(t *testing.T, r *Response)
	}{
		{
			name: "pairs and scalars",
			body: `<ajax_response_xml_root><IF_ERRORSTR>SUCC</IF_ERRORSTR><OBJ_A><Instance><ParaName>A</ParaName><ParaValue>1</ParaValue><ParaName>B</ParaName><ParaValue>x&#32;y</ParaValue></Instance><Instance><ParaName>A</ParaName><ParaValue>2</ParaValue></Instance></OBJ_A><encode>Pwd</encode></ajax_response_xml_root>`,
			check: func(t *testing.T, r *Response) {
				if got := len(r.Object("OBJ_A")); got != 2 {
					t.Fatalf("instances = %d", got)
				}
				if v := r.First("OBJ_A").Str("B"); v != "x y" {
					t.Errorf("entity decode: %q", v)
				}
				if r.ErrorStr() != "SUCC" || len(r.Encoded) != 1 || r.Encoded[0] != "Pwd" {
					t.Errorf("values %v encoded %v", r.Values, r.Encoded)
				}
			},
		},
		{
			name: "empty object, whitespace, xml declaration",
			body: "<?xml version=\"1.0\"?>\n<ajax_response_xml_root>\n  <OBJ_EMPTY></OBJ_EMPTY>\n  <OBJ_B>\n    <Instance>\n      <ParaName> N </ParaName>\n      <ParaValue></ParaValue>\n    </Instance>\n  </OBJ_B>\n</ajax_response_xml_root>",
			check: func(t *testing.T, r *Response) {
				if r.Object("OBJ_EMPTY") != nil || len(r.Objects) != 1 {
					t.Errorf("empty object: %+v", r)
				}
				if v, ok := r.First("OBJ_B")["N"]; !ok || v != "" {
					t.Errorf("trimmed name / empty value: %+v", r.First("OBJ_B"))
				}
			},
		},
		{
			name: "missing ParaValue and truncated document",
			body: `<ajax_response_xml_root><OBJ_C><Instance><ParaName>X</ParaName><ParaName>Y</ParaName><ParaValue>2</ParaValue></Instance><Instance><ParaName>Z</ParaName><ParaValue>3</Para`,
			check: func(t *testing.T, r *Response) {
				in := r.First("OBJ_C")
				if in["X"] != "" || in["Y"] != "2" {
					t.Errorf("got %+v", in)
				}
			},
		},
		{
			name: "json with Instance arrays and flat objects",
			body: `{"OBJ_J":{"Instance":[{"_InstID":"IGD","N":5}],"encode":"Pwd"},"flat":{"MacAddr":"AA:BB:CC:DD:EE:FF","n":2,"ok":true},"IF_ERRORID":0}`,
			check: func(t *testing.T, r *Response) {
				if r.First("OBJ_J").Str("N") != "5" || r.First("flat").Str("ok") != "1" || r.Values["IF_ERRORID"] != "0" {
					t.Errorf("got %+v", r)
				}
				if len(r.Encoded) != 1 {
					t.Errorf("encoded %v", r.Encoded)
				}
			},
		},
		{name: "empty", body: "  ", wantErr: true},
		{name: "html", body: "no xml at all", wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r, err := Parse([]byte(tc.body))
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v", err)
			}
			if tc.check != nil && r != nil {
				tc.check(t, r)
			}
		})
	}
}

func TestParseFixtures(t *testing.T) {
	tests := []struct {
		file   string
		object string
		count  int
	}{
		{"devmgr_statusmgr.xml", "OBJ_DEVINFO_ID", 1},
		{"devmgr_statusmgr.xml", "OBJ_CPUMEMUSAGE_ID", 1},
		{"wan_internetstatus.xml", "ID_WAN_COMFIG", 2},
		{"status_lan_info.xml", "OBJ_PON_PORT_BASIC_STATUS_ID", 5},
		{"accessdev_landevs.xml", "OBJ_ACCESSDEV_ID", 2},
		{"wlan_client_stat.xml", "OBJ_WLANAP_ID", 8},
		{"wlan_client_stat.xml", "OBJ_WLAN_AD_ID", 0},
		{"wlan_client_stat_synthetic.xml", "OBJ_WLAN_AD_ID", 2},
		{"wlan_wlanstatus.xml", "OBJ_WLANCONFIGDRV_ID", 8},
		{"wlan_wlanbasicadconf.xml", "OBJ_WLANSETTING_ID", 2},
		{"wlan_wlansssidconf.xml", "OBJ_WLANAP_ID", 8},
		{"topo.json", "controllerData", 1},
		{"web_login_timeout.xml", "OBJ_USERIF_ID", 1},
		{"optical_info_synthetic.xml", "OBJ_PON_OPTICALPARA_ID", 1},
	}
	for _, tc := range tests {
		t.Run(tc.file+"/"+tc.object, func(t *testing.T) {
			r := fixture(t, tc.file)
			if got := len(r.Object(tc.object)); got != tc.count {
				t.Errorf("%s: %d instances, want %d (objects %v)", tc.object, got, tc.count, r.ObjectNames())
			}
		})
	}
	if r := fixture(t, "session_timeout.xml"); r.ErrorStr() != "SessionTimeout" {
		t.Errorf("session timeout: %q", r.ErrorStr())
	}
}

func TestLeadingFloat(t *testing.T) {
	for in, want := range map[string]float64{"-21.3 dBm": -21.3, "62%": 62, " 45 ℃": 45, "+3": 3, "1.5e3": 1.5} {
		if got, ok := leadingFloat(in); !ok || got != want {
			t.Errorf("%q = %v,%v want %v", in, got, ok, want)
		}
	}
	for _, in := range []string{"", "N/A", "-", "dBm"} {
		if _, ok := leadingFloat(in); ok {
			t.Errorf("%q parsed", in)
		}
	}
}
