package zte

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/eduardpaul/routerctl/routerkit/brands/zte/ztefake"
)

func newFake(t *testing.T) (*ztefake.Server, *Client, *bytes.Buffer) {
	t.Helper()
	fake := ztefake.New("user", "s3cr3t-pass", ztefake.FixtureFS())
	srv := httptest.NewServer(fake)
	t.Cleanup(srv.Close)
	logs := &bytes.Buffer{}
	c, err := New(srv.URL, "user", "s3cr3t-pass", &Options{
		Logger: slog.New(slog.NewTextHandler(logs, &slog.HandlerOptions{Level: slog.LevelDebug})),
	})
	if err != nil {
		t.Fatal(err)
	}
	return fake, c, logs
}

func TestHashPassword(t *testing.T) {
	// sha256("user84054044"), the digest the UI computes for the observed salt
	if got := hashPassword("user", "84054044"); got != "2b404159c16714ec8b11d2a043875a552b1e2061fd4b72a44fdb9cf526e8e8f5" {
		t.Fatalf("digest %q", got)
	}
	if hashPassword("a", "b") != hashPassword("ab", "") {
		t.Error("digest is not sha256(password+token)")
	}
}

func TestTokenAndDecode(t *testing.T) {
	page := []byte(`var _sessionTmpToken = "";` + "\n" + `_sessionTmpToken = "\x4a\x6f\x66\x43";`)
	if tok := extractToken(page); tok != "JofC" {
		t.Fatalf("token %q", tok)
	}
	for _, plain := range []string{"x", "exactly16bytes!!", "pppoe-user@isp.example", ""} {
		enc := encodeParaValue("AbCdEfGhIjKlMnOpQrStUvWx", plain)
		got, err := decodeParaValue("AbCdEfGhIjKlMnOpQrStUvWx", enc)
		if plain == "" {
			got, err = decodeParaValue("tok", "")
		}
		if err != nil || got != plain {
			t.Errorf("round trip %q -> %q (%v)", plain, got, err)
		}
	}
	if _, err := decodeParaValue("tok", "not base64!"); err == nil {
		t.Error("garbage decoded")
	}
}

func TestSecretName(t *testing.T) {
	for _, n := range []string{"Password", "KeyPassphrase", "WEPKey", "Pwd", "UserName", "PreSharedKey", "WPSPin"} {
		if !secretName(n) {
			t.Errorf("%s not secret", n)
		}
	}
	for _, n := range []string{"WEPKeyIndex", "EnablePassThrough", "HostName", "ESSID", "MACAddress"} {
		if secretName(n) {
			t.Errorf("%s secret", n)
		}
	}
}

func TestLoginAndReads(t *testing.T) {
	fake, c, logs := newFake(t)
	ctx := context.Background()
	if err := c.Login(ctx); err != nil {
		t.Fatal(err)
	}
	d, err := c.DeviceInfo(ctx)
	if err != nil || d.Model != "F8648P" {
		t.Fatalf("device info %+v %v", d, err)
	}
	if _, err := c.System(ctx); err != nil {
		t.Fatal(err)
	}
	w, err := c.WANStatus(ctx)
	if err != nil || len(w.Connections) != 2 {
		t.Fatalf("wan %+v %v", w, err)
	}
	hs, err := c.Clients(ctx)
	if err != nil || len(hs) != 2 {
		t.Fatalf("clients %+v %v", hs, err)
	}
	wf, err := c.WiFi(ctx)
	if err != nil || len(wf.Radios) != 2 {
		t.Fatalf("wifi %+v %v", wf, err)
	}
	tr, err := c.Traffic(ctx)
	if err != nil || len(tr.Ports) != 5 || len(tr.WAN) != 1 {
		t.Fatalf("traffic %+v %v", tr, err)
	}
	tp, err := c.Topology(ctx)
	if err != nil || tp.MAC == "" {
		t.Fatalf("topology %+v %v", tp, err)
	}
	if to, err := c.SessionTimeout(ctx); err != nil || to.Minutes() != 5 {
		t.Fatalf("timeout %v %v", to, err)
	}
	p, err := c.PON(ctx)
	if err != nil || p.RxPower == nil {
		t.Fatalf("pon %+v %v", p, err)
	}
	// Get by name, tag and view/tag; secrets blanked
	r, err := c.Get(ctx, "wan_internetstatus")
	if err != nil {
		t.Fatal(err)
	}
	for _, in := range r.Object("ID_WAN_COMFIG") {
		if in["Password"] != "" || in["UserName"] != "" {
			t.Errorf("secret leaked: %v", in)
		}
	}
	if _, err := c.Get(ctx, "devmgr_statusmgr_lua.lua"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Get(ctx, "statusMgr/devmgr_statusmgr_lua.lua"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Get(ctx, "nope"); err == nil {
		t.Error("unknown source accepted")
	}
	if err := c.Logout(ctx); err != nil {
		t.Fatal(err)
	}
	st := fake.Stats()
	if st.Logins != 1 || st.Logouts != 1 || st.FailedLogins != 0 {
		t.Errorf("stats %+v", st)
	}
	for _, p := range fake.Paths() {
		if strings.HasPrefix(p, "POST") && !strings.Contains(p, "loginData") {
			t.Errorf("non-login POST sent: %s", p)
		}
	}
	if strings.Contains(logs.String(), "s3cr3t-pass") {
		t.Error("password in logs")
	}
}

func TestViewCaching(t *testing.T) {
	fake, c, _ := newFake(t)
	ctx := context.Background()
	if _, err := c.DeviceInfo(ctx); err != nil {
		t.Fatal(err)
	}
	n := fake.Stats().Requests
	if _, err := c.System(ctx); err != nil { // same page: data tag only
		t.Fatal(err)
	}
	if got := fake.Stats().Requests - n; got != 1 {
		t.Errorf("second read on the same page took %d requests", got)
	}
}

func TestAutoRelogin(t *testing.T) {
	fake, c, _ := newFake(t)
	ctx := context.Background()
	if _, err := c.DeviceInfo(ctx); err != nil { // implicit login
		t.Fatal(err)
	}
	fake.Expire()
	if _, err := c.DeviceInfo(ctx); err != nil {
		t.Fatalf("after expiry: %v", err)
	}
	fake.Expire()
	if _, err := c.WANStatus(ctx); err != nil { // view load fails first
		t.Fatalf("after expiry (new page): %v", err)
	}
	if st := fake.Stats(); st.Logins != 3 {
		t.Errorf("logins %d, want 3", st.Logins)
	}
}

func TestNotAvailableIsCached(t *testing.T) {
	fake, c, _ := newFake(t)
	fake.HiddenViews["ponopticalinfo"] = true // like the "user" account
	ctx := context.Background()
	if _, err := c.PON(ctx); !errors.Is(err, ErrNotAvailable) {
		t.Fatalf("pon err %v", err)
	}
	n := fake.Stats()
	if _, err := c.PON(ctx); !errors.Is(err, ErrNotAvailable) {
		t.Fatalf("pon err %v", err)
	}
	if fake.Stats().Requests != n.Requests || n.Logins != 1 {
		t.Errorf("unavailable page re-probed: %+v -> %+v", n, fake.Stats())
	}
	// a data tag the page does not carry
	if _, err := c.Get(ctx, "statusMgr/optical_info_lua.lua"); !errors.Is(err, ErrNotAvailable) {
		t.Errorf("tag err %v", err)
	}
	if fake.Stats().Logins != 1 {
		t.Errorf("not-available triggered a re-login")
	}
}

func TestBadCredentialsNoRetry(t *testing.T) {
	fake := ztefake.New("user", "right", ztefake.FixtureFS())
	srv := httptest.NewServer(fake)
	defer srv.Close()
	c, _ := New(srv.URL, "user", "wrong-password", nil)
	ctx := context.Background()
	err := c.Login(ctx)
	var le *LoginError
	if !errors.As(err, &le) || !errors.Is(err, ErrAuth) || le.Message == "" {
		t.Fatalf("err %v", err)
	}
	if strings.Contains(err.Error(), "wrong-password") {
		t.Error("password in error")
	}
	// reads must not try again on their own
	for i := 0; i < 3; i++ {
		if _, err := c.DeviceInfo(ctx); !errors.Is(err, ErrAuth) {
			t.Fatalf("read err %v", err)
		}
	}
	if st := fake.Stats(); st.FailedLogins != 1 {
		t.Errorf("failed logins %d, want 1", st.FailedLogins)
	}
}

func TestLockout(t *testing.T) {
	fake := ztefake.New("user", "right", ztefake.FixtureFS())
	fake.MaxFailures = 1
	srv := httptest.NewServer(fake)
	defer srv.Close()
	c, _ := New(srv.URL, "user", "wrong", nil)
	ctx := context.Background()
	if err := c.Login(ctx); !errors.Is(err, ErrLocked) {
		t.Fatalf("err %v", err)
	}
	before := fake.Stats().Requests
	if err := c.Login(ctx); !errors.Is(err, ErrLocked) {
		t.Fatalf("err %v", err)
	}
	if fake.Stats().Requests != before {
		t.Error("login attempted while locked")
	}
}

func TestConcurrentReads(t *testing.T) {
	fake, c, _ := newFake(t)
	ctx := context.Background()
	var wg sync.WaitGroup
	errs := make(chan error, 40)
	for i := 0; i < 10; i++ {
		wg.Add(4)
		go func() { defer wg.Done(); _, err := c.DeviceInfo(ctx); errs <- err }()
		go func() { defer wg.Done(); _, err := c.WANStatus(ctx); errs <- err }()
		go func() { defer wg.Done(); _, err := c.Clients(ctx); errs <- err }()
		go func() { defer wg.Done(); _, err := c.Traffic(ctx); errs <- err }()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if st := fake.Stats(); st.Logins != 1 {
		t.Errorf("logins %d, want 1 (login must be serialized)", st.Logins)
	}
}

func TestNoRedirectAndBodyCap(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("_tag") == "login_entry" {
			http.Redirect(w, r, "http://elsewhere.example/", http.StatusFound)
			return
		}
		if r.URL.Path == "/big" {
			_, _ = w.Write(bytes.Repeat([]byte("x"), 2048))
			return
		}
		_, _ = w.Write([]byte("<title>login</title>"))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	c, _ := New(srv.URL, "u", "p", &Options{MaxBodyBytes: 1024})
	if err := c.Login(context.Background()); !errors.Is(err, ErrUnexpected) {
		t.Errorf("redirect: %v", err)
	}
	if _, _, err := c.do(context.Background(), http.MethodGet, "/big", nil); !errors.Is(err, ErrTooLarge) {
		t.Errorf("body cap: %v", err)
	}
}

func TestKeepSecretsDecodes(t *testing.T) {
	// a fake that serves one encoded value, encrypted with the page token
	fake := ztefake.New("user", "p", ztefake.FixtureFS())
	var srvTok func() string = fake.PageToken
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("_type") == "menuData" && r.URL.Query().Get("_tag") == "wlan_wlansssidconf_lua.lua" {
			enc := encodeParaValue(srvTok(), "wifi-key-123")
			_, _ = w.Write([]byte(`<ajax_response_xml_root><IF_ERRORSTR>SUCC</IF_ERRORSTR><OBJ_WLANPSK_ID><Instance><ParaName>KeyPassphrase</ParaName><ParaValue>` + enc + `</ParaValue></Instance></OBJ_WLANPSK_ID><encode>KeyPassphrase</encode></ajax_response_xml_root>`))
			return
		}
		fake.ServeHTTP(w, r)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	ctx := context.Background()
	c, _ := New(srv.URL, "user", "p", &Options{KeepSecrets: true})
	r, err := c.Get(ctx, "wlan_wlansssidconf")
	if err != nil {
		t.Fatal(err)
	}
	if got := r.First("OBJ_WLANPSK_ID")["KeyPassphrase"]; got != "wifi-key-123" {
		t.Errorf("decoded %q", got)
	}
	c2, _ := New(srv.URL, "user", "p", nil)
	r, err = c2.Get(ctx, "wlan_wlansssidconf")
	if err != nil {
		t.Fatal(err)
	}
	if got := r.First("OBJ_WLANPSK_ID")["KeyPassphrase"]; got != "" {
		t.Errorf("secret not blanked: %q", got)
	}
}

func TestNewHostForms(t *testing.T) {
	tests := map[string]struct {
		opts *Options
		want string
	}{
		"192.168.1.1":          {nil, "http://192.168.1.1"},
		"192.168.1.1:8080":     {nil, "http://192.168.1.1:8080"},
		"https://192.168.1.1/": {nil, "https://192.168.1.1"},
		"router.lan":           {&Options{UseSSL: true, Port: 8443}, "https://router.lan:8443"},
	}
	for host, tc := range tests {
		c, err := New(host, "u", "p", tc.opts)
		if err != nil || c.BaseURL() != tc.want {
			t.Errorf("%s: %v %v", host, c, err)
		}
	}
	if _, err := New("", "u", "p", nil); err == nil {
		t.Error("empty host accepted")
	}
	if _, err := New("h", "u", "p", &Options{PinSHA256: "zz"}); err == nil {
		t.Error("bad pin accepted")
	}
}
