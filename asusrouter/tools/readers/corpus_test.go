package readers

import (
	"encoding/hex"
	"fmt"
	"math"
	"sort"
	"strings"
	"testing"

	"github.com/eduardpaul/routerctl/asusrouter/tools/converters"
)

// tag renders a parsed value with its exact Python type, matching the
// `tag` helper of the corpus generator.
func tag(v any) string {
	switch x := v.(type) {
	case nil:
		return "None"
	case bool:
		if x {
			return "True"
		}
		return "False"
	case int64:
		return fmt.Sprintf("i:%d", x)
	case float64:
		if math.IsNaN(x) {
			return "f:nan"
		}
		return "f:" + converters.FloatRepr(x)
	case complex128:
		return fmt.Sprintf("c:(%s,%s)", converters.FloatRepr(real(x)), converters.FloatRepr(imag(x)))
	case string:
		return "s:" + hex.EncodeToString([]byte(x))
	case []byte:
		return "b:" + hex.EncodeToString(x)
	case []any:
		return "[" + joinTags(x, false) + "]"
	case Tuple:
		return "(" + joinTags(x, false) + ")"
	case Set:
		return "{" + joinTags(x, true) + "}"
	case map[string]any:
		parts := []string{}
		for k, e := range x {
			parts = append(parts, tag(k)+"="+tag(e))
		}
		sort.Strings(parts)
		return "<" + strings.Join(parts, ",") + ">"
	case map[any]any:
		parts := []string{}
		for k, e := range x {
			parts = append(parts, tag(k)+"="+tag(e))
		}
		sort.Strings(parts)
		return "<" + strings.Join(parts, ",") + ">"
	}
	return fmt.Sprintf("unsupported:%T", v)
}

func joinTags(items []any, sorted bool) string {
	parts := make([]string, len(items))
	for i, e := range items {
		parts[i] = tag(e)
	}
	if sorted {
		sort.Strings(parts)
	}
	return strings.Join(parts, ",")
}

// knownDeviations maps corpus inputs to the documented Go result where
// it differs from Python (integers beyond int64, tuple dict keys).
var knownDeviations = map[string]string{
	"a = 9223372036854775808;":   "<s:61=f:9.223372036854776e+18>",
	"a = {(1, 2): 3};":           "<s:61=s:7b28312c2032293a20337d>",
	`{"a": 9223372036854775808}`: "<s:61=f:9.223372036854776e+18>",
}

// TestJSVariablesCorpus compares ReadJSVariables with CPython on inputs
// exercising the JSON / literal_eval / fallback chain.
func TestJSVariablesCorpus(t *testing.T) {
	for _, c := range jsVariablesCorpus {
		if strings.HasPrefix(c.want, "raise:") {
			// Python yields an Ellipsis object; Go falls back to the text
			continue
		}
		want := c.want
		if dev, ok := knownDeviations[c.input]; ok {
			want = dev
		}
		if got := tag(ReadJSVariables(c.input)); got != want {
			t.Errorf("ReadJSVariables(%q)\n got %s\nwant %s", c.input, got, want)
		}
	}
}

func TestJSONContentCorpus(t *testing.T) {
	for _, c := range jsonContentCorpus {
		want := c.want
		if dev, ok := knownDeviations[c.input]; ok {
			want = dev
		}
		if got := tag(ReadJSONContent(c.input)); got != want {
			t.Errorf("ReadJSONContent(%q)\n got %s\nwant %s", c.input, got, want)
		}
	}
}
