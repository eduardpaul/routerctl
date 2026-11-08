package routerkit

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"testing"
)

func TestClassify(t *testing.T) {
	base := errors.New("zte: login rejected: bad password")
	err := Classify(base, ErrAuth)
	if err.Error() != base.Error() || !errors.Is(err, ErrAuth) || !errors.Is(err, base) {
		t.Errorf("Classify = %v", err)
	}
	if Classify(nil, ErrAuth) != nil {
		t.Error("Classify(nil) != nil")
	}
	if e := fmt.Errorf("x: %w", ErrAuth); Classify(e, ErrAuth) != e {
		t.Error("already classified errors are returned as is")
	}
}

func TestCodeOf(t *testing.T) {
	opErr := &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("connection refused")}
	for err, want := range map[error]ErrCode{
		nil:                                     "",
		errors.New("boom"):                      CodeDevice,
		fmt.Errorf("x: %w", ErrUnsupported):     CodeUnsupported,
		Classify(errors.New("x"), ErrNotFound):  CodeUnsupported,
		Classify(errors.New("x"), ErrLockedOut): CodeAuth,
		fmt.Errorf("x: %w", ErrBusy):            CodeBusy,
		fmt.Errorf("get: %w", opErr):            CodeUnreachable,
		context.DeadlineExceeded:                CodeTimeout,
	} {
		if got := CodeOf(err); got != want {
			t.Errorf("CodeOf(%v) = %q, want %q", err, got, want)
		}
	}
	pe := NewPartError(errors.New("boom"))
	if pe.Code != CodeDevice || pe.Error() != "boom" {
		t.Errorf("NewPartError = %+v", pe)
	}
}

func TestCapabilities(t *testing.T) {
	c := Capabilities{Parts: AllSupported()}
	c.Parts[PartNodes] = PartSupport{}
	c.Parts[PartFirmware] = PartialSupport("available")
	if !c.Supports(PartWAN) || c.Supports(PartNodes) || !c.Supports(PartFirmware) {
		t.Error("Supports")
	}
	if got := c.Filter(AllParts); len(got) != len(AllParts)-1 {
		t.Errorf("Filter = %v", got)
	}
	if got := (Capabilities{}).Filter(AllParts); len(got) != len(AllParts) {
		t.Errorf("an undeclared Capabilities filters %v", got)
	}
	b, err := json.Marshal(c.Parts[PartFirmware])
	if err != nil || string(b) != `{"level":"partial","missing":["available"]}` {
		t.Errorf("JSON %s %v", b, err)
	}
	var ps PartSupport
	if err := json.Unmarshal(b, &ps); err != nil || ps.Level != Partial {
		t.Errorf("round trip %+v %v", ps, err)
	}
	if PartialSupport().Level != Supported {
		t.Error("PartialSupport() without fields")
	}
}

func TestBaseDefaults(t *testing.T) {
	var b Base
	if c, err := b.Capabilities(context.Background()); err != nil || len(c.Parts) != 0 {
		t.Errorf("Capabilities = %+v, %v", c, err)
	}
	if _, err := b.Get(context.Background(), "x"); !errors.Is(err, ErrNotFound) {
		t.Errorf("Get: %v", err)
	}
	if _, err := b.Do(context.Background(), Invocation{Name: "x"}); !errors.Is(err, ErrUnsupported) {
		t.Errorf("Do: %v", err)
	}
}
