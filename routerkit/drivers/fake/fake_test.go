package fake

import (
	"context"
	"errors"
	"testing"

	"github.com/eduardpaul/routerctl/routerkit"
)

func TestFactoryAndPoll(t *testing.T) {
	f := Factory()
	if f.Info.Name != "fake" || f.New == nil {
		t.Fatalf("Factory = %+v", f)
	}
	d, err := f.New(routerkit.Config{Host: "demo"})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := d.Connect(ctx); err != nil {
		t.Fatal(err)
	}
	s1, _ := d.Poll(ctx, routerkit.Poll(routerkit.PartTraffic))
	s2, _ := d.Poll(ctx, routerkit.Poll(routerkit.PartTraffic, routerkit.PartClients))
	if s1.Clients != nil || s2.Clients == nil {
		t.Error("parts not honoured")
	}
	if *s2.Traffic[0].RxBytes <= *s1.Traffic[0].RxBytes {
		t.Error("counters should grow")
	}
	// Snapshots are copies
	s2.Clients[0].Name = "changed"
	s3, _ := d.Poll(ctx, routerkit.Poll(routerkit.PartClients))
	if s3.Clients[0].Name == "changed" {
		t.Error("snapshot aliases the driver state")
	}
	if _, err := d.Do(ctx, routerkit.Invocation{Name: "wifi", Args: routerkit.RawArgs(map[string]string{"band": "6g1", "state": "on"})}); !errors.Is(err, routerkit.ErrInvalidArgs) {
		t.Errorf("bad choice: %v", err)
	}
	if _, err := d.Get(ctx, "nope"); err == nil {
		t.Error("unknown source accepted")
	}
	if len(f.Matches(routerkit.HomeEvidence("<html>"+Marker))) != 1 || len(f.Matches(routerkit.HomeEvidence("<html>ASUS"))) != 0 {
		t.Error("fingerprint")
	}
}
