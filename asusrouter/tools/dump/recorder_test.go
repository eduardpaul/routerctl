package dump

import (
	"context"
	"reflect"
	"testing"

	"github.com/eduardpaul/routerctl/asusrouter/consts"
)

type fakeEndpoint string

func (e fakeEndpoint) Name() string { return string(e) }

const (
	fetchData      fakeEndpoint = "FETCH_DATA"
	fetchVpnStatus fakeEndpoint = "FETCH_VPN_STATUS"
)

func ptr(s string) *string { return &s }

func TestEmpty(t *testing.T) {
	recorder := NewARDumpRecorder()
	if len(recorder.Requests()) != 0 || recorder.Len() != 0 || recorder.Bool() {
		t.Fatal("not empty")
	}
}

func TestRecordAppendsOrdered(t *testing.T) {
	recorder := NewARDumpRecorder()
	recorder.Record(fetchData, consts.RequestTypePost, ptr("p0"), ptr("b0"))
	recorder.Record(fetchVpnStatus, consts.RequestTypeGet, nil, ptr("b1"))

	if recorder.Len() != 2 || !recorder.Bool() {
		t.Fatal("not recorded")
	}
	requests := recorder.Requests()
	want := ARDumpRequest{Order: 0, Endpoint: fetchData, RequestType: consts.RequestTypePost, Payload: ptr("p0"), Content: ptr("b0")}
	if !reflect.DeepEqual(requests[0], want) {
		t.Fatal(requests[0])
	}
	second := requests[1]
	if second.Order != 1 || second.Endpoint != fetchVpnStatus || second.Payload != nil {
		t.Fatal(second)
	}
}

func TestBindRecorder(t *testing.T) {
	parent := context.Background()
	if ActiveRecorder(parent) != nil {
		t.Fatal("unexpected recorder")
	}
	recorder := NewARDumpRecorder()
	ctx := BindRecorder(parent, recorder)
	if ActiveRecorder(ctx) != recorder || ActiveRecorder(parent) != nil {
		t.Fatal("bad binding")
	}
}
