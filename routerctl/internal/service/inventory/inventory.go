// Package inventory is the persistent device inventory: every MAC address
// ever seen, with first/last seen times, the reported name/vendor/IP, a
// known flag and a user label. It lives in a JSON state file (written
// atomically) shared with the event engine state.
//
// The CLI (`routerctl devices ...`) and a running daemon may both edit
// the file: Save merges user edits (labels, approvals, forgets) made on
// disk since the last load.
package inventory

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/eduardpaul/routerctl/routerkit"
)

// Entry is one device of the inventory.
type Entry struct {
	MAC       string    `json:"mac"`
	FirstSeen time.Time `json:"first_seen"`
	// LastSeen is the last time the device was seen online
	LastSeen time.Time `json:"last_seen"`
	Name     string    `json:"name,omitempty"`
	Vendor   string    `json:"vendor,omitempty"`
	IP       string    `json:"ip,omitempty"`
	// Known: approved (or labelled) by the user, or recorded by the
	// first-run baseline
	Known bool   `json:"known"`
	Label string `json:"label,omitempty"`
	// Router is the configured device that saw it last
	Router string `json:"router,omitempty"`
	// Online is the last raw online state (not debounced)
	Online bool `json:"online"`
}

// DisplayName is the label, else the reported name, else the MAC.
func (e Entry) DisplayName() string {
	switch {
	case e.Label != "":
		return e.Label
	case e.Name != "":
		return e.Name
	}
	return e.MAC
}

// Lookup is the read side used by the event engine.
type Lookup interface {
	Get(mac string) (Entry, bool)
}

// file is the on-disk format.
type file struct {
	Version int                        `json:"version"`
	Devices map[string]Entry           `json:"devices"`
	Engines map[string]json.RawMessage `json:"engines,omitempty"`
}

const fileVersion = 1

// Store is the inventory plus engine states, backed by a file.
type Store struct {
	mu      sync.Mutex
	path    string
	devices map[string]Entry
	engines map[string]json.RawMessage
	// existed: the state file existed when opened
	existed bool
	// disk sync point: mtime/size of the file we last read or wrote
	syncMod  time.Time
	syncSize int64
	// onDisk: the MACs in the file at the sync point
	onDisk map[string]bool
}

// Open loads the state file (a missing file is an empty inventory).
// An empty path gives an in-memory store.
func Open(path string) (*Store, error) {
	s := &Store{path: path, devices: map[string]Entry{}, engines: map[string]json.RawMessage{}}
	if path == "" {
		return s, nil
	}
	f, st, err := readFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return s, nil
		}
		return nil, err
	}
	s.existed = true
	s.devices, s.engines = f.Devices, f.Engines
	s.syncMod, s.syncSize = st.ModTime(), st.Size()
	s.markDisk()
	return s, nil
}

func readFile(path string) (*file, os.FileInfo, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}
	st, err := os.Stat(path)
	if err != nil {
		return nil, nil, err
	}
	var f file
	if len(strings.TrimSpace(string(data))) > 0 {
		if err := json.Unmarshal(data, &f); err != nil {
			return nil, nil, fmt.Errorf("state file %s: %w", path, err)
		}
	}
	if f.Devices == nil {
		f.Devices = map[string]Entry{}
	}
	if f.Engines == nil {
		f.Engines = map[string]json.RawMessage{}
	}
	return &f, st, nil
}

// Path returns the state file path.
func (s *Store) Path() string { return s.path }

// Existed reports whether the state file existed when opened.
func (s *Store) Existed() bool { return s.existed }

// Len returns the number of devices.
func (s *Store) Len() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.devices)
}

// Get implements Lookup.
func (s *Store) Get(mac string) (Entry, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.devices[routerkit.NormalizeMAC(mac)]
	return e, ok
}

// List returns every entry sorted by MAC.
func (s *Store) List() []Entry {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Entry, 0, len(s.devices))
	for _, e := range s.devices {
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].MAC < out[j].MAC })
	return out
}

// Snapshot returns a copy of the entries (a Lookup frozen in time).
func (s *Store) Snapshot() Map {
	s.mu.Lock()
	defer s.mu.Unlock()
	m := make(Map, len(s.devices))
	for k, v := range s.devices {
		m[k] = v
	}
	return m
}

// Map is a plain Lookup.
type Map map[string]Entry

// Get implements Lookup.
func (m Map) Get(mac string) (Entry, bool) {
	e, ok := m[routerkit.NormalizeMAC(mac)]
	return e, ok
}

// Observe records the clients a router reported. New entries are
// marked known when baseline is set. It returns the MACs that were new.
func (s *Store) Observe(router string, clients []routerkit.Client, now time.Time, baseline bool) []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	var added []string
	for _, c := range clients {
		mac := routerkit.NormalizeMAC(c.MAC)
		if mac == "" {
			continue
		}
		e, ok := s.devices[mac]
		if !ok {
			e = Entry{MAC: mac, FirstSeen: now, Known: baseline}
			added = append(added, mac)
		}
		if c.Online || !ok {
			e.LastSeen = now
		}
		if c.Online && c.IP != "" {
			e.IP = c.IP
		}
		if c.Name != "" {
			e.Name = c.Name
		}
		if c.Vendor != "" {
			e.Vendor = c.Vendor
		}
		e.Online = c.Online
		e.Router = router
		s.devices[mac] = e
	}
	sort.Strings(added)
	return added
}

// ErrNotFound is returned for an unknown MAC.
var ErrNotFound = errors.New("no such device in the inventory")

func (s *Store) edit(mac string, fn func(e *Entry)) error {
	m := routerkit.NormalizeMAC(mac)
	if m == "" {
		return fmt.Errorf("%q is not a MAC address", mac)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.devices[m]
	if !ok {
		return fmt.Errorf("%s: %w", m, ErrNotFound)
	}
	fn(&e)
	s.devices[m] = e
	return nil
}

// Label sets the user label (and marks the device known); an empty
// label clears it.
func (s *Store) Label(mac, label string) error {
	return s.edit(mac, func(e *Entry) {
		e.Label = strings.TrimSpace(label)
		if e.Label != "" {
			e.Known = true
		}
	})
}

// Approve marks a device known.
func (s *Store) Approve(mac string) error { return s.SetKnown(mac, true) }

// SetKnown marks a device known or unknown (an unknown device with a
// label stays labelled).
func (s *Store) SetKnown(mac string, known bool) error {
	return s.edit(mac, func(e *Entry) { e.Known = known })
}

// Forget removes a device (it fires device_new again if it comes back).
func (s *Store) Forget(mac string) error {
	m := routerkit.NormalizeMAC(mac)
	if m == "" {
		return fmt.Errorf("%q is not a MAC address", mac)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.devices[m]; !ok {
		return fmt.Errorf("%s: %w", m, ErrNotFound)
	}
	delete(s.devices, m)
	return nil
}

// EngineState returns the stored event engine state of a router.
func (s *Store) EngineState(router string) json.RawMessage {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.engines[router]
}

// SetEngineState stores the event engine state of a router.
func (s *Store) SetEngineState(router string, raw json.RawMessage) {
	s.mu.Lock()
	s.engines[router] = raw
	s.mu.Unlock()
}

// mergeDisk folds user edits made on disk since our last sync into
// memory: labels and approvals win from disk, and devices that existed
// at the sync point but are gone from disk were forgotten.
func (s *Store) mergeDisk() error {
	f, st, err := readFile(s.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	if st.ModTime().Equal(s.syncMod) && st.Size() == s.syncSize {
		return nil
	}
	for mac, e := range s.devices {
		d, ok := f.Devices[mac]
		if !ok {
			if s.onDisk[mac] {
				delete(s.devices, mac)
			}
			continue
		}
		e.Label, e.Known = d.Label, d.Known
		s.devices[mac] = e
	}
	// Entries added on disk by someone else are kept
	for mac, d := range f.Devices {
		if _, ok := s.devices[mac]; !ok && !s.onDisk[mac] {
			s.devices[mac] = d
		}
	}
	return nil
}

func (s *Store) markDisk() {
	s.onDisk = make(map[string]bool, len(s.devices))
	for mac := range s.devices {
		s.onDisk[mac] = true
	}
}

// Save writes the state file atomically (temp file, fsync, rename).
func (s *Store) Save() error {
	if s.path == "" {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.onDisk != nil {
		if err := s.mergeDisk(); err != nil {
			return err
		}
	}
	data, err := json.MarshalIndent(file{Version: fileVersion, Devices: s.devices, Engines: s.engines}, "", "  ")
	if err != nil {
		return err
	}
	if err := WriteFileAtomic(s.path, append(data, '\n'), 0o600); err != nil {
		return err
	}
	if st, err := os.Stat(s.path); err == nil {
		s.syncMod, s.syncSize = st.ModTime(), st.Size()
	}
	s.markDisk()
	s.existed = true
	return nil
}

// WriteFileAtomic writes data to path through a temp file in the same
// directory, fsyncs it and renames it over path.
func WriteFileAtomic(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name) // no-op after a successful rename
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(perm); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(name, path); err != nil {
		return err
	}
	if d, err := os.Open(dir); err == nil {
		_ = d.Sync()
		d.Close()
	}
	return nil
}
