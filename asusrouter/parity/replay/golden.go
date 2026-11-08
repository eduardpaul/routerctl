package replay

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"sort"

	"github.com/eduardpaul/routerctl/asusrouter/modules/endpoint"
	"github.com/eduardpaul/routerctl/asusrouter/parity/canonical"
	"github.com/eduardpaul/routerctl/asusrouter/tools/converters"
)

// Read returns an `async_read` that answers from the session and parses
// with the endpoint reader, like AsusRouter.async_read (redirect pages and
// 404s give an empty result).
func (s *Session) Read() func(ctx context.Context, ep endpoint.AREndpoint, request string) (any, error) {
	return func(ctx context.Context, ep endpoint.AREndpoint, request string) (any, error) {
		raw, err := s.FetchRaw()(ctx, ep, request)
		if err != nil || raw == nil {
			return map[string]any{}, err
		}
		normalized := converters.RawToStr(*raw)
		if normalized == nil || *normalized == "" {
			return map[string]any{}, nil
		}
		return endpoint.GetEndpointReader(ep)(*normalized), nil
	}
}

// FetchRaw returns an `async_fetch` answering from the session.
func (s *Session) FetchRaw() func(ctx context.Context, ep endpoint.AREndpoint, request string) (*string, error) {
	return func(ctx context.Context, ep endpoint.AREndpoint, request string) (*string, error) {
		var payload *string
		if request != "" {
			payload = &request
		}
		rec := s.Answer(string(ep), payload)
		if rec.Status == 404 {
			return nil, nil
		}
		content := rec.Content
		return &content, nil
	}
}

// RepoRoot returns the repository root: the nearest directory above this
// source file that holds parity/check.sh. It returns "" when there is none
// (for example when the module is used from the module cache), so that
// LocalDevices finds nothing instead of looking in the wrong place.
func RepoRoot() string {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return ""
	}
	return findUp(filepath.Dir(file), filepath.Join("parity", "check.sh"))
}

// findUp returns the first directory at or above dir that contains rel,
// or "" when no such directory exists.
func findUp(dir, rel string) string {
	for d := filepath.Clean(dir); ; {
		if _, err := os.Stat(filepath.Join(d, rel)); err == nil {
			return d
		}
		parent := filepath.Dir(d)
		if parent == d {
			return ""
		}
		d = parent
	}
}

// LocalDevices lists the device directories under parity/local and
// parity/fixtures that hold both a session.jsonl and a py.json golden.
func LocalDevices() []string {
	root := RepoRoot()
	if root == "" {
		return nil
	}
	var out []string
	for _, base := range []string{"parity/local", "parity/fixtures"} {
		matches, _ := filepath.Glob(filepath.Join(root, base, "*", "session.jsonl"))
		for _, m := range matches {
			dir := filepath.Dir(m)
			if _, err := os.Stat(filepath.Join(dir, "py.json")); err == nil {
				out = append(out, dir)
			}
		}
	}
	sort.Strings(out)
	return out
}

// LoadGolden reads a py.json golden (source name -> canonical value).
func LoadGolden(path string) (map[string]any, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var out map[string]any
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// Diff compares a Go value (canonicalized here) with a golden JSON value
// and returns human-readable differences (empty when equal).
func Diff(golden any, got any) []string {
	var g any
	b, _ := json.Marshal(canonical.Of(got))
	_ = json.Unmarshal(b, &g)
	var diffs []string
	diff("$", golden, g, &diffs)
	return diffs
}

func diff(path string, want, got any, out *[]string) {
	if len(*out) > 50 {
		return
	}
	switch w := want.(type) {
	case map[string]any:
		g, ok := got.(map[string]any)
		if !ok {
			*out = append(*out, fmt.Sprintf("%s: want object, got %s", path, short(got)))
			return
		}
		keys := map[string]bool{}
		for k := range w {
			keys[k] = true
		}
		for k := range g {
			keys[k] = true
		}
		sorted := make([]string, 0, len(keys))
		for k := range keys {
			sorted = append(sorted, k)
		}
		sort.Strings(sorted)
		for _, k := range sorted {
			wv, wok := w[k]
			gv, gok := g[k]
			switch {
			case !gok:
				*out = append(*out, fmt.Sprintf("%s.%s: missing in Go (want %s)", path, k, short(wv)))
			case !wok:
				*out = append(*out, fmt.Sprintf("%s.%s: extra in Go (%s)", path, k, short(gv)))
			default:
				diff(path+"."+k, wv, gv, out)
			}
		}
	case []any:
		g, ok := got.([]any)
		if !ok {
			*out = append(*out, fmt.Sprintf("%s: want list, got %s", path, short(got)))
			return
		}
		if len(w) != len(g) {
			*out = append(*out, fmt.Sprintf("%s: want %d items, got %d", path, len(w), len(g)))
		}
		for i := 0; i < len(w) && i < len(g); i++ {
			diff(fmt.Sprintf("%s[%d]", path, i), w[i], g[i], out)
		}
	case float64:
		g, ok := got.(float64)
		if !ok || math.Abs(w-g) > 1e-6*math.Max(1, math.Abs(w)) {
			*out = append(*out, fmt.Sprintf("%s: want %v, got %s", path, w, short(got)))
		}
	default:
		if fmt.Sprintf("%T:%v", want, want) != fmt.Sprintf("%T:%v", got, got) {
			*out = append(*out, fmt.Sprintf("%s: want %s, got %s", path, short(want), short(got)))
		}
	}
}

func short(v any) string {
	b, _ := json.Marshal(v)
	s := string(b)
	if len(s) > 120 {
		s = s[:120] + "..."
	}
	return s
}
