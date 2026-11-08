package inventory

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/eduardpaul/routerctl/routerkit"
)

var t0 = time.Date(2026, 11, 7, 10, 0, 0, 0, time.UTC)

func TestObserveAndEdit(t *testing.T) {
	s, _ := Open("")
	added := s.Observe("asus", []routerkit.Client{
		{MAC: "AA-BB-CC-00-00-01", IP: "10.0.0.5", Name: "phone", Vendor: "Acme", Online: true},
		{MAC: "aa:bb:cc:00:00:02", Name: "tv", Online: false},
		{MAC: "not-a-mac"},
	}, t0, false)
	if len(added) != 2 || added[0] != "aa:bb:cc:00:00:01" {
		t.Fatalf("added = %v", added)
	}
	e, ok := s.Get("aa:bb:cc:00:00:01")
	if !ok || e.IP != "10.0.0.5" || e.Known || e.Router != "asus" || !e.FirstSeen.Equal(t0) || !e.Online {
		t.Fatalf("entry = %+v", e)
	}

	// Second sight: last_seen moves only while online, IP only when online
	t1 := t0.Add(time.Minute)
	if added := s.Observe("asus", []routerkit.Client{
		{MAC: "aa:bb:cc:00:00:01", IP: "10.0.0.6", Online: true},
		{MAC: "aa:bb:cc:00:00:02", IP: "10.0.0.9", Online: false},
	}, t1, false); len(added) != 0 {
		t.Fatalf("re-observe added %v", added)
	}
	e, _ = s.Get("aa:bb:cc:00:00:01")
	if !e.LastSeen.Equal(t1) || e.IP != "10.0.0.6" || e.Name != "phone" || !e.FirstSeen.Equal(t0) {
		t.Errorf("after update: %+v", e)
	}
	tv, _ := s.Get("aa:bb:cc:00:00:02")
	if !tv.LastSeen.Equal(t0) || tv.IP != "" {
		t.Errorf("offline client: %+v", tv)
	}

	if err := s.Label("AABBCC000002", "Living room TV"); err != nil {
		t.Fatal(err)
	}
	tv, _ = s.Get("aa:bb:cc:00:00:02")
	if tv.Label != "Living room TV" || !tv.Known || tv.DisplayName() != "Living room TV" {
		t.Errorf("label: %+v", tv)
	}
	if err := s.Approve("aa:bb:cc:00:00:01"); err != nil {
		t.Fatal(err)
	}
	if e, _ := s.Get("aa:bb:cc:00:00:01"); !e.Known {
		t.Error("approve did not mark known")
	}
	if err := s.Forget("aa:bb:cc:00:00:01"); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.Get("aa:bb:cc:00:00:01"); ok {
		t.Error("forget did not remove")
	}
	if err := s.Forget("aa:bb:cc:00:00:01"); err == nil {
		t.Error("forgetting twice should fail")
	}
	if err := s.Label("zz", "x"); err == nil {
		t.Error("invalid MAC should fail")
	}
}

func TestBaseline(t *testing.T) {
	s, _ := Open("")
	s.Observe("asus", []routerkit.Client{{MAC: "aa:bb:cc:00:00:01", Online: true}}, t0, true)
	if e, _ := s.Get("aa:bb:cc:00:00:01"); !e.Known {
		t.Error("baseline entries are known")
	}
}

func TestSaveAtomicAndReload(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "sub", "state.json")
	s, err := Open(p)
	if err != nil {
		t.Fatal(err)
	}
	if s.Existed() {
		t.Error("new file should not exist")
	}
	s.Observe("asus", []routerkit.Client{{MAC: "aa:bb:cc:00:00:01", Name: "phone", Online: true}}, t0, false)
	s.SetEngineState("asus", json.RawMessage(`{"wan_ip":"1.2.3.4"}`))
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}
	st, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode().Perm() != 0o600 {
		t.Errorf("mode %v", st.Mode().Perm())
	}
	// No temp files left behind
	ents, _ := os.ReadDir(filepath.Dir(p))
	if len(ents) != 1 {
		t.Errorf("leftover files: %v", ents)
	}
	s2, err := Open(p)
	if err != nil {
		t.Fatal(err)
	}
	if e, ok := s2.Get("aa:bb:cc:00:00:01"); !ok || e.Name != "phone" {
		t.Errorf("reloaded entry: %+v", e)
	}
	var eng map[string]string
	if err := json.Unmarshal(s2.EngineState("asus"), &eng); err != nil || eng["wan_ip"] != "1.2.3.4" {
		t.Errorf("engine state: %s", s2.EngineState("asus"))
	}
}

// A CLI edit on disk while the daemon holds the store in memory survives
// the daemon's next save.
func TestMergeDiskEdits(t *testing.T) {
	p := filepath.Join(t.TempDir(), "state.json")
	daemon, _ := Open(p)
	daemon.Observe("asus", []routerkit.Client{
		{MAC: "aa:bb:cc:00:00:01", Online: true},
		{MAC: "aa:bb:cc:00:00:02", Online: true},
	}, t0, false)
	if err := daemon.Save(); err != nil {
		t.Fatal(err)
	}

	cli, _ := Open(p)
	if err := cli.Label("aa:bb:cc:00:00:01", "Phone"); err != nil {
		t.Fatal(err)
	}
	if err := cli.Forget("aa:bb:cc:00:00:02"); err != nil {
		t.Fatal(err)
	}
	time.Sleep(10 * time.Millisecond) // distinct mtime
	if err := cli.Save(); err != nil {
		t.Fatal(err)
	}

	// The daemon sees a new device and saves
	daemon.Observe("asus", []routerkit.Client{{MAC: "aa:bb:cc:00:00:03", Online: true}}, t0.Add(time.Minute), false)
	if err := daemon.Save(); err != nil {
		t.Fatal(err)
	}
	final, _ := Open(p)
	if e, _ := final.Get("aa:bb:cc:00:00:01"); e.Label != "Phone" || !e.Known {
		t.Errorf("label lost: %+v", e)
	}
	if _, ok := final.Get("aa:bb:cc:00:00:02"); ok {
		t.Error("forgotten device came back")
	}
	if _, ok := final.Get("aa:bb:cc:00:00:03"); !ok {
		t.Error("new device lost")
	}
}

func TestCorruptFile(t *testing.T) {
	p := filepath.Join(t.TempDir(), "state.json")
	os.WriteFile(p, []byte("{not json"), 0o600)
	if _, err := Open(p); err == nil {
		t.Error("want an error for a corrupt state file")
	}
}
