package stdout

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/eduardpaul/routerctl/routerctl/internal/service/events"
	"github.com/eduardpaul/routerctl/routerctl/internal/service/poller"
)

func TestJSONLines(t *testing.T) {
	var b strings.Builder
	s := New(&b, true, events.Selector{})
	s.Event(events.Event{Type: events.WANDown, Device: "asus", Time: time.Unix(0, 0).UTC(), Data: map[string]any{}})
	s.State(poller.DeviceState{Name: "asus", Online: true, LastPoll: time.Unix(1, 0).UTC()})
	lines := strings.Split(strings.TrimSpace(b.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("lines: %q", lines)
	}
	var e map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &e); err != nil || e["type"] != "wan_down" || e["device"] != "asus" || e["time"] != "1970-01-01T00:00:00Z" {
		t.Errorf("event line: %s", lines[0])
	}
	var snap map[string]any
	if err := json.Unmarshal([]byte(lines[1]), &snap); err != nil || snap["type"] != "snapshot" || snap["online"] != true {
		t.Errorf("snapshot line: %s", lines[1])
	}
	b.Reset()
	New(&b, true, events.NewSelector([]string{"wan_up"}, nil, nil, false)).Event(events.Event{Type: events.WANDown})
	if b.Len() != 0 {
		t.Error("selector ignored")
	}
	New(&b, false, events.Selector{}).State(poller.DeviceState{Name: "asus"})
	if b.Len() != 0 {
		t.Error("snapshots off still printed")
	}
}
