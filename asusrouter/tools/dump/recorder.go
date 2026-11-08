// Raw request recorder for AsusRouter data dumps (recorder.py).

package dump

import (
	"context"
	"slices"
	"sync"

	"github.com/eduardpaul/routerctl/asusrouter/consts"
)

// Endpoint is the recorded endpoint (`AREndpoint` in Python, only
// type-checked there). `endpoint.AREndpoint` satisfies it.
type Endpoint interface {
	Name() string
}

// ARDumpRequest mirrors `ARDumpRequest`: a single recorded wire request
// and its raw reply.
type ARDumpRequest struct {
	Order       int                `json:"order"`
	Endpoint    Endpoint           `json:"endpoint"`
	RequestType consts.RequestType `json:"request_type"`
	Payload     *string            `json:"payload"`
	Content     *string            `json:"content"`
}

// ARDumpRecorder mirrors `ARDumpRecorder`: collect raw wire requests
// issued while resolving a source. Safe for concurrent use.
type ARDumpRecorder struct {
	mu       sync.Mutex
	requests []ARDumpRequest
}

// NewARDumpRecorder creates an empty recorder.
func NewARDumpRecorder() *ARDumpRecorder {
	return &ARDumpRecorder{requests: []ARDumpRequest{}}
}

// Record mirrors `record`: record a single wire request and its raw
// reply.
func (r *ARDumpRecorder) Record(
	endpoint Endpoint,
	requestType consts.RequestType,
	payload *string,
	content *string,
) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.requests = append(r.requests, ARDumpRequest{
		Order:       len(r.requests),
		Endpoint:    endpoint,
		RequestType: requestType,
		Payload:     payload,
		Content:     content,
	})
}

// Requests mirrors the `requests` property: the recorded requests in
// capture order (a copy).
func (r *ARDumpRecorder) Requests() []ARDumpRequest {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.requests == nil {
		return []ARDumpRequest{}
	}
	return slices.Clone(r.requests)
}

// Len mirrors `__len__`: the number of recorded requests.
func (r *ARDumpRecorder) Len() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.requests)
}

// Bool mirrors `__bool__`: whether anything was recorded.
func (r *ARDumpRecorder) Bool() bool {
	return r.Len() > 0
}

// Recorder bound to the current context; only the dump's own fetches
// (and the work they start with that context) see it, so concurrent
// unrelated traffic on the same router is never swept into a dump.
// Python uses a ContextVar; Go carries it in a context.Context.
type recorderKey struct{}

// ActiveRecorder mirrors `active_recorder`: return the recorder bound to
// the context, if any.
func ActiveRecorder(ctx context.Context) *ARDumpRecorder {
	if ctx == nil {
		return nil
	}
	recorder, _ := ctx.Value(recorderKey{}).(*ARDumpRecorder)
	return recorder
}

// BindRecorder mirrors `bind_recorder`: bind a recorder to a derived
// context. Python's `unbind_recorder(token)` is going back to the parent
// context.
func BindRecorder(ctx context.Context, recorder *ARDumpRecorder) context.Context {
	return context.WithValue(ctx, recorderKey{}, recorder)
}
