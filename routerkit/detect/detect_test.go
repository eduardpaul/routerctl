package detect

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/eduardpaul/routerctl/routerkit"
	"github.com/eduardpaul/routerctl/routerkit/drivers/asus"
	"github.com/eduardpaul/routerctl/routerkit/drivers/fake"
	ztedriver "github.com/eduardpaul/routerctl/routerkit/drivers/zte"
)

func allBrands(t *testing.T) *routerkit.Registry {
	t.Helper()
	r, err := routerkit.NewRegistry(fake.Factory(), asus.Factory(), ztedriver.Factory())
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// TestMatrix runs every fingerprint against every page of the corpus
// (testdata/homepages/<driver>-*.html): the page's own driver wins, and
// no other driver scores above a weak marker.
func TestMatrix(t *testing.T) {
	r := allBrands(t)
	files, err := filepath.Glob("testdata/homepages/*.html")
	if err != nil || len(files) == 0 {
		t.Fatalf("no corpus: %v", err)
	}
	brands := map[string]bool{}
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		want := strings.SplitN(filepath.Base(f), "-", 2)[0]
		brands[want] = true
		cands := Score(r, routerkit.HomeEvidence(string(b)))
		best, err := Pick(r, cands)
		if err != nil || best.Driver != want {
			t.Errorf("%s: picked %+v, %v (candidates %v)", f, best, err, cands)
		}
		for _, c := range cands {
			if c.Driver != want && c.Score > routerkit.ScoreWeak {
				t.Errorf("%s: %s scores %d (> weak) on another brand's page", f, c.Driver, c.Score)
			}
		}
	}
	for _, n := range r.Names() {
		if !brands[n] {
			t.Errorf("driver %s has no page in the corpus", n)
		}
	}
}

func TestPick(t *testing.T) {
	r := allBrands(t)
	if _, err := Pick(r, nil); !errors.Is(err, ErrUnrecognized) || !strings.Contains(err.Error(), "known drivers: [asus fake zte]") {
		t.Errorf("no candidate: %v", err)
	}
	tie := []routerkit.Match{{Driver: "asus", Score: 40}, {Driver: "zte", Score: 40}, {Driver: "fake", Score: 10}}
	if _, err := Pick(r, tie); !errors.Is(err, ErrAmbiguous) || !strings.Contains(err.Error(), "asus, zte") {
		t.Errorf("tie: %v", err)
	}
	if m, err := Pick(r, []routerkit.Match{{Driver: "zte", Score: 90}, {Driver: "asus", Score: 10}}); err != nil || m.Driver != "zte" {
		t.Errorf("best: %+v %v", m, err)
	}
}

func TestProbe(t *testing.T) {
	probe := routerkit.Fingerprint{Probes: []string{"/extra.xml"}, Match: func(ev *routerkit.Evidence) (routerkit.Match, bool) {
		if ev.Page("/extra.xml").Status == 200 {
			return routerkit.Match{Score: routerkit.ScoreStrong, Reason: "extra probe"}, true
		}
		return routerkit.Match{}, false
	}}
	reg, err := routerkit.NewRegistry(fake.Factory(), routerkit.Factory{Info: routerkit.DriverInfo{Name: "zz"}, New: fake.Factory().New, Fingerprints: []routerkit.Fingerprint{probe}})
	if err != nil {
		t.Fatal(err)
	}
	if got := Probes(reg); strings.Join(got, ",") != "/,/extra.xml" {
		t.Errorf("Probes = %v", got)
	}
	var hits []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" || r.Method != http.MethodGet {
			t.Error("detection must send unauthenticated GETs only")
		}
		hits = append(hits, r.URL.Path)
		switch r.URL.Path {
		case "/":
			http.Redirect(w, r, "/Main_Login.asp", http.StatusFound)
		case "/extra.xml":
			w.Write([]byte("<root/>"))
		default:
			w.Header().Set("Server", "httpd/2.0")
			w.Write([]byte("<html><head><title>Fake Router</title></head><body>" + fake.Marker + "</body></html>"))
		}
	}))
	defer srv.Close()
	host := strings.TrimPrefix(srv.URL, "http://")
	res, err := Probe(context.Background(), reg, host, false, 0, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if res.Driver != "fake" || res.Title != "Fake Router" || res.Status != 200 || res.Server != "httpd/2.0" || len(res.Candidates) != 2 {
		t.Errorf("result: %+v", res)
	}
	if strings.Join(hits, ",") != "/,/Main_Login.asp,/extra.xml" {
		t.Errorf("requests %v: each probe once", hits)
	}
	ev, err := Gather(context.Background(), Target{Host: host, Timeout: 5 * time.Second}, []string{"/", "/extra.xml"})
	if err != nil || !strings.Contains(ev.Home().Text(), "Main_Login.asp") {
		t.Errorf("redirect trail: %v %v", ev.Home().Redirects, err)
	}

	unknown := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte("<title>Something else</title>"))
	}))
	defer unknown.Close()
	res, err = Probe(context.Background(), reg, strings.TrimPrefix(unknown.URL, "http://"), false, 0, 5*time.Second)
	if !errors.Is(err, ErrUnrecognized) || res.Driver != "" || res.Title != "Something else" {
		t.Errorf("unknown device: %+v %v", res, err)
	}
	if _, err := Probe(context.Background(), reg, "127.0.0.1:1", false, 0, time.Second); err == nil {
		t.Error("closed port detected")
	}
}

func TestProber(t *testing.T) {
	r := allBrands(t)
	page, err := os.ReadFile("testdata/homepages/zte-F8648P-V2.0.12P7N17.html")
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.Write(page) }))
	defer srv.Close()
	m, err := Prober(context.Background(), r, routerkit.Config{Host: strings.TrimPrefix(srv.URL, "http://"), Timeout: 5 * time.Second})
	if err != nil || m.Driver != "zte" || m.Model != "F8648P" || m.Variant != "lua" {
		t.Errorf("Prober = %+v, %v", m, err)
	}
	name, err := r.WithProber(Prober).Detect(context.Background(), routerkit.Config{Host: strings.TrimPrefix(srv.URL, "http://")})
	if err != nil || name != "zte" {
		t.Errorf("Registry.Detect = %q, %v", name, err)
	}
}

func TestBaseURL(t *testing.T) {
	if got := BaseURL("192.168.50.1", true, 8443); got != "https://192.168.50.1:8443/" {
		t.Error(got)
	}
	if got := BaseURL("router.lan", false, 0); got != "http://router.lan/" {
		t.Error(got)
	}
}
