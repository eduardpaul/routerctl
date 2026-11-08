package httprec

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
)

func TestRecordReplay(t *testing.T) {
	n := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n++
		http.SetCookie(w, &http.Cookie{Name: "SID", Value: "secret-session", Path: "/", HttpOnly: true})
		w.Header().Set("Content-Type", "text/plain")
		w.Header().Set("X-Private", "do-not-keep")
		_ = r.ParseForm()
		io.WriteString(w, r.URL.Query().Get("_tag")+":"+r.PostForm.Get("Username")+":"+string(rune('0'+n)))
	}))
	defer srv.Close()
	red := Redact{Query: []string{"token"}, Form: []string{"Password", "_sessionTOKEN"}}
	rec := NewRecorder(nil, red)
	c := &http.Client{Transport: rec}
	get := func(c *http.Client, path string) string {
		t.Helper()
		resp, err := c.Get(srv.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return string(b)
	}
	if got := get(c, "/?_tag=a&token=t1"); got != "a::1" {
		t.Fatalf("live answer %q", got)
	}
	get(c, "/?token=t2&_tag=a")
	resp, err := c.PostForm(srv.URL+"/login", url.Values{"Username": {"admin"}, "Password": {"hunter2"}, "_sessionTOKEN": {"abc"}})
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if rec.Len() != 3 {
		t.Fatalf("recorded %d", rec.Len())
	}
	path := filepath.Join(t.TempDir(), "session.jsonl")
	if err := rec.Save(path); err != nil {
		t.Fatal(err)
	}
	ex, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range ex {
		all := e.URL + e.Body + strings.Join(e.Header["Set-Cookie"], ";")
		for _, secret := range []string{"t1", "t2", "hunter2", "abc", "secret-session"} {
			if strings.Contains(all, secret) {
				t.Errorf("%q leaked into %+v", secret, e)
			}
		}
		if _, ok := e.Header["X-Private"]; ok {
			t.Error("unlisted header kept")
		}
	}
	if !strings.Contains(ex[2].Body, "Username=admin") {
		t.Errorf("form %q", ex[2].Body)
	}

	// Replay: same key (query order and masked token ignored), in order,
	// the last answer repeats
	rp := NewReplayer(ex, red)
	rc := &http.Client{Transport: rp}
	for i, want := range []string{"a::1", "a::2", "a::2"} {
		if got := get(rc, "/?_tag=a&token=other"); got != want {
			t.Errorf("replay %d = %q, want %q", i, got, want)
		}
	}
	resp, err = rc.Get(srv.URL + "/unknown")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound || len(rp.Misses()) != 1 || rp.Requests() != 4 {
		t.Errorf("miss: %d %v %d", resp.StatusCode, rp.Misses(), rp.Requests())
	}
	if len(rp.Keys()) != 2 {
		t.Errorf("keys %v", rp.Keys())
	}
	rp.Rewind()
	if got := get(rc, "/?_tag=a&token=x"); got != "a::1" {
		t.Errorf("after rewind %q", got)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/?_tag=a", nil)
	if _, err := rc.Do(req); err == nil {
		t.Error("cancelled request answered")
	}
}

func TestReadErrors(t *testing.T) {
	if _, err := Read(strings.NewReader(`{"method":"GET"}`)); err == nil {
		t.Error("incomplete exchange accepted")
	}
	if _, err := Read(strings.NewReader(`{bad`)); err == nil {
		t.Error("bad JSON accepted")
	}
	if ex, err := Read(strings.NewReader("")); err != nil || len(ex) != 0 {
		t.Errorf("empty: %v %v", ex, err)
	}
}
