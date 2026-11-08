// Package ztefake is an httptest-friendly fake of the ZTE F86xx web UI:
// the login handshake (login_entry / login_token / SHA-256 digest), the
// single session cookie, the "page must be loaded before its data tag"
// rule, SessionTimeout answers, the login lockout and logout. Data tags are
// served from fixture files.
package ztefake

import (
	"crypto/rand"
	"crypto/sha256"
	"embed"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io/fs"
	"net/http"
	"strings"
	"sync"
)

//go:embed testdata
var testdata embed.FS

// FixtureFS returns the default fixture files (an F8648P's data tags and
// login page, embedded from testdata/), for New and for tests in any
// package.
func FixtureFS() fs.FS {
	sub, err := fs.Sub(testdata, "testdata")
	if err != nil {
		panic(err) // the embedded directory always exists
	}
	return sub
}

// SessionPath is the sanitized recorded session of an F8648P (firmware
// V2.0.12P7N17, "user" account) inside FixtureFS, in netkit/httprec
// format: the data tags as the device answered them (MACs, serials, SSIDs,
// host names, public addresses, credentials and tokens replaced), the UI
// pages reduced to their page token, and the login answers the capture
// lacked synthesized.
const SessionPath = "F8648P/V2.0.12P7N17/session.jsonl"

// Session returns the bytes of the recorded session (SessionPath).
func Session() []byte {
	b, err := fs.ReadFile(FixtureFS(), SessionPath)
	if err != nil {
		panic(err) // embedded
	}
	return b
}

// Views maps the menu views the fake knows to the data tags (without
// query parameters) each one unlocks.
var Views = map[string][]string{
	"statusMgr":      {"devmgr_statusmgr_lua.lua"},
	"ethWanStatus":   {"wan_internetstatus_lua.lua"},
	"localNetStatus": {"status_lan_info_lua.lua", "wlan_wlanstatus_lua.lua", "wlan_client_stat_lua.lua", "accessdev_landevs_lua.lua", "usb_usbdevs_lua.lua"},
	"wlanBasic":      {"wlan_wlanbasiconoff_lua.lua", "wlan_wlanbasicadconf_lua.lua", "wlan_wlansssidconf_lua.lua"},
	"mmTopology":     {"topo_lua.lua"},
	"accountMgr":     {"web_login_timeout_lua.lua"},
	"ponopticalinfo": {"optical_info_lua.lua"},
}

// Fixtures maps data tags to fixture file names.
var Fixtures = map[string]string{
	"devmgr_statusmgr_lua.lua":     "devmgr_statusmgr.xml",
	"wan_internetstatus_lua.lua":   "wan_internetstatus.xml",
	"status_lan_info_lua.lua":      "status_lan_info.xml",
	"wlan_wlanstatus_lua.lua":      "wlan_wlanstatus.xml",
	"wlan_client_stat_lua.lua":     "wlan_client_stat.xml",
	"accessdev_landevs_lua.lua":    "accessdev_landevs.xml",
	"wlan_wlanbasiconoff_lua.lua":  "wlan_wlanbasiconoff.xml",
	"wlan_wlanbasicadconf_lua.lua": "wlan_wlanbasicadconf.xml",
	"wlan_wlansssidconf_lua.lua":   "wlan_wlansssidconf.xml",
	"topo_lua.lua":                 "topo.json",
	"web_login_timeout_lua.lua":    "web_login_timeout.xml",
	"optical_info_lua.lua":         "optical_info_synthetic.xml",
}

const sessionTimeoutXML = `<?xml version="1.0"?>
<ajax_response_xml_root>
    <IF_ERRORSTR>SessionTimeout</IF_ERRORSTR>
    <IF_ERRORPARAM>SUCC</IF_ERRORPARAM>
    <IF_ERRORTYPE>SUCC</IF_ERRORTYPE>
</ajax_response_xml_root>`

const notFoundHTML = "<html>\n    <head>\n        <title>404 Not Found</title>\n    </head>\n<body>404</body></html>"

// Server is the fake device. Use it as an http.Handler.
type Server struct {
	User, Pass string
	// Data holds the fixture files (Fixtures names)
	Data fs.FS
	// HiddenViews are views that answer 404 (e.g. "ponopticalinfo" to
	// mimic the "user" account)
	HiddenViews map[string]bool
	// MaxFailures before the login locks (default 3)
	MaxFailures int
	// Files overrides Fixtures per tag (e.g. a synthetic client list)
	Files map[string]string
	// LockSeconds is the lockingTime reported while locked (default 60)
	LockSeconds int
	// FailView makes the next N loads of a view answer 404 (transient)
	FailView map[string]int
	// FailTag makes a data tag answer HTTP 500 (a device-side failure)
	FailTag map[string]bool

	mu        sync.Mutex
	sid       string // the single active session
	salt      string
	sessToken string
	pageToken string
	curView   string
	failures  int
	locked    bool
	lockNext  bool

	// counters (read with Stats)
	logins, failed, logouts, requests int
	paths                             []string
}

// Stats are request counters.
type Stats struct {
	Logins, FailedLogins, Logouts, Requests int
}

// New returns a fake device serving fixtures from data.
func New(user, pass string, data fs.FS) *Server {
	return &Server{User: user, Pass: pass, Data: data, HiddenViews: map[string]bool{}, MaxFailures: 3,
		Files: map[string]string{}, LockSeconds: 60, FailView: map[string]int{}, FailTag: map[string]bool{}}
}

// LockNextLogin makes the next credential POST answer with a temporary
// lock (lockingTime = LockSeconds), as a device does on a re-login storm.
func (s *Server) LockNextLogin() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lockNext = true
}

// Stats returns the counters.
func (s *Server) Stats() Stats {
	s.mu.Lock()
	defer s.mu.Unlock()
	return Stats{s.logins, s.failed, s.logouts, s.requests}
}

// Paths returns the request log ("GET /?_type=menuView&_tag=statusMgr").
func (s *Server) Paths() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.paths...)
}

// Expire drops the session (idle timeout).
func (s *Server) Expire() {
	s.mu.Lock()
	s.sid, s.curView = "", ""
	s.mu.Unlock()
}

// PageToken returns the token of the last page served (for decoding).
func (s *Server) PageToken() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.pageToken
}

func randHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func jsEscape(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		fmt.Fprintf(&b, `\x%02x`, s[i])
	}
	return b.String()
}

func (s *Server) authed(r *http.Request) bool {
	c, err := r.Cookie("SID")
	return err == nil && s.sid != "" && c.Value == s.sid
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.requests++
	q := r.URL.Query()
	typ, tag := q.Get("_type"), q.Get("_tag")
	s.paths = append(s.paths, r.Method+" "+r.URL.Path+"?_type="+typ+"&_tag="+tag)
	switch {
	case r.URL.Path == "/" && typ == "":
		s.home(w, r)
	case typ == "loginData" && tag == "login_entry" && r.Method == http.MethodGet:
		s.sessToken = randHex(12)
		writeJSON(w, fmt.Sprintf(`{"lockingTime":%d,"loginErrMsg":"","promptMsg":"%s","sess_token":"%s"}`,
			map[bool]int{true: 60}[s.locked], map[bool]string{true: "locked"}[s.locked], s.sessToken))
	case typ == "loginData" && tag == "login_token":
		var b [4]byte
		_, _ = rand.Read(b[:])
		s.salt = fmt.Sprintf("%08d", binary.BigEndian.Uint32(b[:])%100000000)
		w.Header().Set("Content-Type", "text/xml; charset=utf-8")
		fmt.Fprintf(w, "<ajax_response_xml_root>%s</ajax_response_xml_root>", s.salt)
	case typ == "loginData" && tag == "login_entry" && r.Method == http.MethodPost:
		s.login(w, r)
	case typ == "loginData" && tag == "logout_entry" && r.Method == http.MethodPost:
		_ = r.ParseForm()
		if s.authed(r) && r.PostForm.Get("IF_LogOff") == "1" && r.PostForm.Get("_sessionTOKEN") == s.pageToken {
			s.sid, s.curView = "", ""
			s.logouts++
		}
		http.SetCookie(w, &http.Cookie{Name: "SID", Value: "", Path: "/", MaxAge: -1})
		writeJSON(w, `{"need_refresh":true}`)
	case typ == "menuView":
		if s.FailView[tag] > 0 {
			s.FailView[tag]--
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprint(w, notFoundHTML)
			return
		}
		if !s.authed(r) || s.HiddenViews[tag] {
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprint(w, notFoundHTML)
			return
		}
		if _, ok := Views[tag]; !ok {
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprint(w, notFoundHTML)
			return
		}
		s.curView = tag
		s.pageToken = randHex(12)
		fmt.Fprintf(w, "<div id=\"%s\"><script>\n_sessionTmpToken = \"%s\";\n</script></div>", tag, jsEscape(s.pageToken))
	case typ == "menuData" && s.FailTag[tag]:
		http.Error(w, "internal error", http.StatusInternalServerError)
	case typ == "menuData":
		w.Header().Set("Content-Type", "text/xml; charset=utf-8")
		if !s.authed(r) || !allowed(s.curView, tag) {
			fmt.Fprint(w, sessionTimeoutXML)
			return
		}
		name, ok := s.Files[tag]
		if !ok {
			name, ok = Fixtures[tag]
		}
		if !ok {
			fmt.Fprint(w, sessionTimeoutXML)
			return
		}
		b, err := fs.ReadFile(s.Data, name)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		_, _ = w.Write(b)
	default:
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, notFoundHTML)
	}
}

func allowed(view, tag string) bool {
	for _, t := range Views[view] {
		if t == tag {
			return true
		}
	}
	return false
}

func writeJSON(w http.ResponseWriter, body string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	fmt.Fprint(w, body)
}

func (s *Server) home(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if !s.authed(r) {
		http.SetCookie(w, &http.Cookie{Name: "SID", Value: randHex(32), Path: "/", HttpOnly: true})
		b, err := fs.ReadFile(s.Data, "login_page.html")
		if err != nil {
			b = []byte(`<title>&#70;&#56;&#54;&#52;&#56;&#80;</title><input id="Frm_Username">`)
		}
		_, _ = w.Write(b)
		return
	}
	s.pageToken = s.sessToken
	fmt.Fprintf(w, "<html><title>&#70;&#56;&#54;&#52;&#56;&#80;</title><script>var _sessionTmpToken = \"\";\n_sessionTmpToken = \"%s\";\nfunction LogOff(){}</script></html>", jsEscape(s.pageToken))
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	f := r.PostForm
	if s.locked {
		writeJSON(w, `{"lockingTime":60,"loginErrMsg":"","promptMsg":"locked","sess_token":"x"}`)
		return
	}
	if s.lockNext {
		s.lockNext = false
		s.salt = ""
		writeJSON(w, fmt.Sprintf(`{"lockingTime":%d,"loginErrMsg":"","promptMsg":"locked","sess_token":"x"}`, s.LockSeconds))
		return
	}
	sum := sha256.Sum256([]byte(s.Pass + s.salt))
	ok := s.salt != "" && f.Get("action") == "login" && f.Get("Username") == s.User &&
		f.Get("Password") == hex.EncodeToString(sum[:]) && f.Get("_sessionTOKEN") == s.sessToken
	s.salt = "" // one use
	if !ok {
		s.failed++
		s.failures++
		if s.failures >= s.MaxFailures {
			s.locked = true
			writeJSON(w, `{"lockingTime":60,"loginErrMsg":"Usuario o contraseña incorrectos.","promptMsg":"","sess_token":"y"}`)
			return
		}
		writeJSON(w, `{"lockingTime":0,"loginErrMsg":"Usuario o contraseña incorrectos.","promptMsg":"","sess_token":"y"}`)
		return
	}
	s.failures = 0
	s.logins++
	s.sid = randHex(32)
	s.curView = ""
	s.sessToken = randHex(12)
	http.SetCookie(w, &http.Cookie{Name: "SID", Value: s.sid, Path: "/", HttpOnly: true})
	writeJSON(w, fmt.Sprintf(`{"sess_token":"%s","login_need_refresh":true}`, s.sessToken))
}
