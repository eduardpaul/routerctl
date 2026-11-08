package asusrouter

import (
	"context"
	"testing"
	"time"

	"github.com/eduardpaul/routerctl/asusrouter/modules/source"
	"github.com/eduardpaul/routerctl/asusrouter/registry"
)

type staleTestSource struct{ source.Base }

func (*staleTestSource) SourceName() string { return "staleTestSource" }

// A forced fetch whose fetcher commits nothing must return nil, not the
// previous (stale) content - like Python, which returns only fresh states.
func TestForcedFetchDoesNotReturnStale(t *testing.T) {
	commit := true
	registry.RegisterSource(registry.SourceEntry{
		Name: "staleTestSource",
		FetchMulti: func(_ context.Context, _ *registry.Context, srcs []source.DataSource) (map[source.ID]any, error) {
			if !commit {
				return map[source.ID]any{}, nil
			}
			return map[source.ID]any{source.IDOf(srcs[0]): "v1"}, nil
		},
	})
	tr := &ReplayTransport{}
	tr.connected = true
	r, err := New("x", "u", "p", Options{Transport: tr})
	if err != nil {
		t.Fatal(err)
	}
	r.SetCacheThreshold(50 * time.Millisecond)
	src := &staleTestSource{}
	ctx := context.Background()
	got, err := r.FetchData(ctx, src, true, nil)
	if err != nil || got != "v1" {
		t.Fatalf("first fetch: %v %v", got, err)
	}
	time.Sleep(80 * time.Millisecond)
	commit = false
	got, err = r.FetchData(ctx, src, true, nil)
	if err != nil || got != nil {
		t.Fatalf("forced fetch without commit returned %v (err %v), want nil", got, err)
	}
}
