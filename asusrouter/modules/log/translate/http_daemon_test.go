package translate

import (
	"testing"

	"github.com/eduardpaul/routerctl/asusrouter/modules/common"
	"github.com/eduardpaul/routerctl/asusrouter/modules/connection"
	"github.com/eduardpaul/routerctl/asusrouter/modules/log"
)

const httpdIP = "192.168.1.100"

func TestCertificate(t *testing.T) {
	info := "S:RT-AX88U-6D90 Server Certificate, " +
		"I:GT-BE19000AI-53A0 Root Certificate 20240101000111, " +
		"2018/5/5 ~ 2045/12/29"
	cases := []tcase{
		{name: "generate", content: "Generating SSL certificate...8443", want: ev{
			log.AREventKeyEventType: AREventHttpDaemonCertificateGenerate,
			log.AREventKeyRaw:       "Generating SSL certificate...8443",
			log.AREventKeyPort:      8443,
		}},
		{name: "info", content: info, want: ev{
			log.AREventKeyEventType: AREventHttpDaemonCertificateInfo,
			log.AREventKeyRaw:       info,
			log.AREventKeySubject:   "RT-AX88U-6D90",
			log.AREventKeyIssuer:    "GT-BE19000AI-53A0",
			log.AREventKeySerial:    "20240101000111",
			log.AREventKeyValidFrom: Date{2018, 5, 5},
			log.AREventKeyValidTo:   Date{2045, 12, 29},
		}},
	}
	for _, bad := range []string{"2018/13/40", "2018/12"} {
		cases = append(cases, tcase{name: "info_bad_date_omitted/" + bad,
			content: "S:X Server Certificate, I:Y Root Certificate 1, " + bad + " ~ 2045/12/29",
			check: func(t *testing.T, e ev) {
				absent(t, e, log.AREventKeyValidFrom)
				eq(t, e, log.AREventKeyValidTo, Date{2045, 12, 29})
			}})
	}
	cases = append(cases,
		tcase{name: "init_success", content: "Succeed to init SSL certificate...8443", want: ev{
			log.AREventKeyEventType: AREventHttpDaemonCertificateInit,
			log.AREventKeyRaw:       "Succeed to init SSL certificate...8443",
			log.AREventKeyStatus:    common.ARStatusCodeSuccess,
			log.AREventKeyPort:      8443,
		}},
		tcase{name: "reload", content: "reload cert and clean all files", want: ev{
			log.AREventKeyEventType: AREventHttpDaemonCertificateReload,
			log.AREventKeyRaw:       "reload cert and clean all files",
		}},
		tcase{name: "restore", content: "Restore saved SSL certificate...8443", want: ev{
			log.AREventKeyEventType: AREventHttpDaemonCertificateRestore,
			log.AREventKeyRaw:       "Restore saved SSL certificate...8443",
			log.AREventKeyPort:      8443,
		}},
	)
	runCases(t, HttpDaemonPatterns, cases)
}

func TestLogin(t *testing.T) {
	success := "[LOGIN][http][APP] successed (" + httpdIP + ")"
	runCases(t, HttpDaemonPatterns, []tcase{
		{name: "success_app_http", content: success, want: ev{
			log.AREventKeyEventType:  AREventHttpDaemonLogin,
			log.AREventKeyRaw:        classified(success, IDENTIFIER),
			log.AREventKeyConnection: connection.ARConnectionHttp,
			log.AREventKeyApiClient:  common.ARApiClientApp,
			log.AREventKeyStatus:     common.ARStatusCodeSuccess,
			log.AREventKeyClientIp:   ip(t, httpdIP),
		}},
		{name: "failed_web_https", content: "[LOGIN][https][Web] failed (" + httpdIP + ")", check: func(t *testing.T, e ev) {
			eq(t, e, log.AREventKeyConnection, connection.ARConnectionHttps)
			eq(t, e, log.AREventKeyApiClient, common.ARApiClientWeb)
			eq(t, e, log.AREventKeyStatus, common.ARStatusCodeFail)
		}},
		{name: "unknown_status", content: "[LOGIN][http][APP] pending (" + httpdIP + ")", check: func(t *testing.T, e ev) {
			eq(t, e, log.AREventKeyStatus, common.ARStatusCodeUnknown)
		}},
	})
}

func TestTranslateHttpDaemon(t *testing.T) {
	runCases(t, HttpDaemonPatterns, []tcase{
		{name: "unrecognized", content: "some future httpd message", want: ev{
			log.AREventKeyEventType: AREventHttpDaemonUnknown,
			log.AREventKeyRaw:       unread("some future httpd message"),
		}},
	})
}
