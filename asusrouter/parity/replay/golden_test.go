package replay

import (
	"os"
	"path/filepath"
	"testing"
)

// TestLocalDevicesFindsFixtures is the guard against silently skipped
// replay tests: every replay test calls t.Skip when LocalDevices finds no
// session, so a wrong RepoRoot (after files move) would leave the whole
// run green without replaying anything. The committed fixtures are found
// here independently (walking up from the test's working directory), and
// every one of them must be listed by LocalDevices.
func TestLocalDevicesFindsFixtures(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	root := findUp(wd, filepath.Join("parity", "fixtures"))
	if root == "" {
		t.Skip("not inside the repository (no parity/fixtures above the package)")
	}
	sessions, _ := filepath.Glob(filepath.Join(root, "parity", "fixtures", "*", "session.jsonl"))
	if len(sessions) == 0 {
		t.Skip("no committed fixtures under parity/fixtures")
	}
	if got := RepoRoot(); got != root {
		t.Fatalf("RepoRoot() = %q, want %q (the directory holding parity/check.sh)", got, root)
	}
	found := map[string]bool{}
	for _, d := range LocalDevices() {
		found[d] = true
	}
	for _, s := range sessions {
		dir := filepath.Dir(s)
		if !found[dir] {
			t.Errorf("fixture %s is not listed by LocalDevices (is py.json missing? run parity/check.sh)", dir)
		}
	}
}

func TestFindUp(t *testing.T) {
	dir := t.TempDir()
	deep := filepath.Join(dir, "a", "b", "c")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "a", "marker"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if got, want := findUp(deep, "marker"), filepath.Join(dir, "a"); got != want {
		t.Fatalf("findUp = %q, want %q", got, want)
	}
	if got := findUp(deep, "no-such-marker-anywhere"); got != "" {
		t.Fatalf("findUp missing = %q, want empty", got)
	}
}
