package consts

import (
	"testing"

	"github.com/eduardpaul/routerctl/asusrouter/tools/converters"
)

func TestVersion(t *testing.T) {
	if UserAgent != "asusrouter-library-DUTUtil-2.0.0.dev0" {
		t.Fatalf("UserAgent = %q", UserAgent)
	}
}

func TestNaiveLocation(t *testing.T) {
	if NaiveLocation != converters.NaiveLocation {
		t.Fatal("NaiveLocation differs from converters.NaiveLocation")
	}
}

func TestHTTPStatusFromValue(t *testing.T) {
	cases := []struct {
		value any
		want  HTTPStatus
	}{
		{200, HTTPStatusOk}, {"404", HTTPStatusNotFound}, {"json_bad_format", HTTPStatusJsonBadFormat},
		{HTTPStatusConflict, HTTPStatusConflict}, {999, HTTPStatusUnknown}, {nil, HTTPStatusUnknown},
	}
	for _, c := range cases {
		if got := HTTPStatusFromValue(c.value); got != c.want {
			t.Errorf("HTTPStatusFromValue(%#v) = %d, want %d", c.value, got, c.want)
		}
	}
	if HTTPStatusJsonBadRequest.Name() != "JSON_BAD_REQUEST" || !HTTPStatusOk.Valid() {
		t.Fatal("bad names")
	}
}
