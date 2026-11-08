// Command parity is the Go side of the Python <-> Go parity harness
// (see parity/py_harness.py and asusrouter/PARITY.md).
//
//	parity replay <session.jsonl>   canonical JSON of every source
//	parity capture <session.jsonl>  record a live session (AR_HOST, AR_USER, AR_PASS)
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"time"

	asusrouter "github.com/eduardpaul/routerctl/asusrouter"
	"github.com/eduardpaul/routerctl/asusrouter/arerror"
	"github.com/eduardpaul/routerctl/asusrouter/parity/canonical"
	_ "github.com/eduardpaul/routerctl/asusrouter/register"
)

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "usage: parity replay|capture <session.jsonl>")
		os.Exit(2)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	var err error
	switch os.Args[1] {
	case "replay":
		err = replayCmd(ctx, os.Args[2])
	case "capture":
		err = captureCmd(ctx, os.Args[2])
	default:
		err = fmt.Errorf("unknown command %q", os.Args[1])
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

// runPipeline mirrors `_run_pipeline`: connect, then fetch every default
// source once, in name order.
func runPipeline(ctx context.Context, r *asusrouter.AsusRouter) (map[string]any, error) {
	results := map[string]any{}
	if _, err := r.Connect(ctx); err != nil {
		return nil, err
	}
	results["_identity"] = canonical.Of(r.Description())
	srcs := r.DefaultSources()
	sort.Slice(srcs, func(i, j int) bool { return srcs[i].SourceName() < srcs[j].SourceName() })
	for _, src := range srcs {
		content, err := r.FetchData(ctx, src, true, nil)
		if err != nil {
			results[src.SourceName()] = map[string]any{"_error": arerror.Name(err)}
			continue
		}
		results[src.SourceName()] = canonical.Of(content)
	}
	_, _ = r.Disconnect(ctx)
	return results, nil
}

func replayCmd(ctx context.Context, path string) error {
	t, err := asusrouter.NewReplayTransport(path)
	if err != nil {
		return err
	}
	r, err := asusrouter.New("replay.invalid", "admin", "replay", asusrouter.Options{Transport: t})
	if err != nil {
		return err
	}
	results, err := runPipeline(ctx, r)
	if err != nil {
		return err
	}
	if os.Getenv("PARITY_DEBUG") != "" {
		for _, m := range t.Session.Misses {
			fmt.Fprintln(os.Stderr, "replay miss:", m)
		}
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", " ")
	enc.SetEscapeHTML(false)
	return enc.Encode(results)
}

func captureCmd(ctx context.Context, path string) error {
	conn, err := asusrouter.NewHTTPTransport(os.Getenv("AR_HOST"), os.Getenv("AR_USER"), os.Getenv("AR_PASS"),
		asusrouter.ConnectionOptions{UseSSL: os.Getenv("AR_SSL") != ""})
	if err != nil {
		return err
	}
	rec := asusrouter.NewRecordingTransport(conn)
	r, err := asusrouter.New(os.Getenv("AR_HOST"), os.Getenv("AR_USER"), "", asusrouter.Options{Transport: rec})
	if err != nil {
		return err
	}
	if _, err := runPipeline(ctx, r); err != nil {
		return err
	}
	_ = r.Close(ctx)
	if err := rec.Save(path); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "captured %d queries -> %s\n", len(rec.Records()), path)
	return nil
}
