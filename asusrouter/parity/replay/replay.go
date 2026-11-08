// Package replay mirrors the session capture/replay of
// parity/py_harness.py: recorded wire queries keyed by (endpoint,
// normalized payload), answered in order.
package replay

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
	"sync"
)

// Record is one recorded wire query (one JSON line of a session file).
type Record struct {
	Endpoint string  `json:"endpoint"`
	Payload  *string `json:"payload"`
	Status   int     `json:"status"`
	Content  string  `json:"content"`
}

var tsRe = regexp.MustCompile(`(^|&)ts=\d+`)

// NormalizePayload mirrors `normalize_payload`: `ts=<n>` -> `ts=*`, and
// `hook=` items are sorted.
func NormalizePayload(payload *string) string {
	if payload == nil {
		return "\x00none"
	}
	p := tsRe.ReplaceAllString(*payload, "${1}ts=*")
	if strings.HasPrefix(p, "hook=") {
		items := strings.Split(p[len("hook="):], ";")
		sort.Strings(items)
		p = "hook=" + strings.Join(items, ";")
	}
	return p
}

// Session answers queries from recorded records.
type Session struct {
	mu     sync.Mutex
	queues map[string][]Record
	cursor map[string]int
	// Misses collects unmatched queries (for debugging)
	Misses []string
}

func sessionKey(endpoint string, payload *string) string {
	return endpoint + "\x01" + NormalizePayload(payload)
}

// NewSession builds a session from records.
func NewSession(records []Record) *Session {
	s := &Session{queues: map[string][]Record{}, cursor: map[string]int{}}
	for _, r := range records {
		k := sessionKey(r.Endpoint, r.Payload)
		s.queues[k] = append(s.queues[k], r)
	}
	return s
}

// Load reads a session file (JSON lines).
func Load(path string) (*Session, error) {
	records, err := ReadRecords(path)
	if err != nil {
		return nil, err
	}
	return NewSession(records), nil
}

// ReadRecords reads the records of a session file.
func ReadRecords(path string) ([]Record, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var records []Record
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1024*1024), 64*1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var r Record
		if err := json.Unmarshal([]byte(line), &r); err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		records = append(records, r)
	}
	return records, sc.Err()
}

// WriteRecords writes records as a session file.
func WriteRecords(path string, records []Record) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	w := bufio.NewWriter(f)
	for _, r := range records {
		b, err := json.Marshal(r)
		if err != nil {
			return err
		}
		w.Write(b)
		w.WriteByte('\n')
	}
	return w.Flush()
}

// Answer returns the recorded reply for a query; unknown queries get a
// 404 like the Python replay.
func (s *Session) Answer(endpoint string, payload *string) Record {
	s.mu.Lock()
	defer s.mu.Unlock()
	k := sessionKey(endpoint, payload)
	q := s.queues[k]
	if len(q) == 0 {
		p := "<nil>"
		if payload != nil {
			p = *payload
		}
		s.Misses = append(s.Misses, endpoint+" "+p)
		return Record{Endpoint: endpoint, Payload: payload, Status: 404}
	}
	i := s.cursor[k]
	s.cursor[k] = i + 1
	if i >= len(q) {
		i = len(q) - 1
	}
	return q[i]
}

// Find returns the first recorded reply for an endpoint whose payload
// contains the given substring (handy in module unit tests).
func (s *Session) Find(endpoint, payloadContains string) (Record, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, q := range s.queues {
		for _, r := range q {
			p := ""
			if r.Payload != nil {
				p = *r.Payload
			}
			if r.Endpoint == endpoint && strings.Contains(p, payloadContains) {
				return r, true
			}
		}
	}
	return Record{}, false
}
