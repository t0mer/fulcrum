package intake

import (
	"context"
	"errors"
	"testing"

	"github.com/t0mer/fulcrum/internal/whatsapp"
)

type fakeStore struct {
	monitored  map[string]bool
	monitorErr error
	enqueued   []string // "provider|group|msg|ref"
	enqueueErr error
	groupNames map[string]string
}

func (f *fakeStore) IsMonitored(g string) (bool, error) {
	if f.monitorErr != nil {
		return false, f.monitorErr
	}
	return f.monitored[g], nil
}

func (f *fakeStore) EnqueueJob(provider, group, msg, ref string) (int64, error) {
	if f.enqueueErr != nil {
		return 0, f.enqueueErr
	}
	f.enqueued = append(f.enqueued, provider+"|"+group+"|"+msg+"|"+ref)
	return int64(len(f.enqueued)), nil
}

func (f *fakeStore) GroupName(g string) string { return f.groupNames[g] }

type fakeNotifier struct{ pings int }

func (f *fakeNotifier) Notify() { f.pings++ }

func img(group string) whatsapp.InboundMessage {
	return whatsapp.InboundMessage{ProviderGroupID: group, MessageID: "m1", IsImage: true, MediaRef: "url:x"}
}

func TestAccept_EnqueuesMonitoredImage(t *testing.T) {
	st := &fakeStore{monitored: map[string]bool{"g@g.us": true}}
	n := &fakeNotifier{}
	s := New(st, nil, n, "greenapi", nil)

	if !s.Accept(context.Background(), img("g@g.us")) {
		t.Fatal("expected Accept to enqueue a monitored image")
	}
	if len(st.enqueued) != 1 || st.enqueued[0] != "greenapi|g@g.us|m1|url:x" {
		t.Fatalf("unexpected enqueue: %v", st.enqueued)
	}
	if n.pings != 1 {
		t.Fatalf("expected 1 notify, got %d", n.pings)
	}
}

func TestAccept_DropsNonImage(t *testing.T) {
	st := &fakeStore{monitored: map[string]bool{"g@g.us": true}}
	n := &fakeNotifier{}
	s := New(st, nil, n, "greenapi", nil)

	m := img("g@g.us")
	m.IsImage = false
	if s.Accept(context.Background(), m) {
		t.Fatal("non-image should not enqueue")
	}
	if len(st.enqueued) != 0 || n.pings != 0 {
		t.Fatalf("non-image leaked: enq=%v pings=%d", st.enqueued, n.pings)
	}
}

func TestAccept_DropsUnmonitored(t *testing.T) {
	st := &fakeStore{monitored: map[string]bool{}} // group not monitored
	n := &fakeNotifier{}
	s := New(st, nil, n, "greenapi", nil)

	if s.Accept(context.Background(), img("other@g.us")) {
		t.Fatal("unmonitored group should not enqueue")
	}
	if len(st.enqueued) != 0 || n.pings != 0 {
		t.Fatalf("unmonitored leaked: enq=%v pings=%d", st.enqueued, n.pings)
	}
}

func TestAccept_SwallowsStoreErrors(t *testing.T) {
	st := &fakeStore{monitorErr: errors.New("db down")}
	n := &fakeNotifier{}
	s := New(st, nil, n, "greenapi", nil)

	if s.Accept(context.Background(), img("g@g.us")) {
		t.Fatal("store error must not report success")
	}

	st = &fakeStore{monitored: map[string]bool{"g@g.us": true}, enqueueErr: errors.New("insert failed")}
	s = New(st, nil, n, "greenapi", nil)
	if s.Accept(context.Background(), img("g@g.us")) {
		t.Fatal("enqueue error must not report success")
	}
	if n.pings != 0 {
		t.Fatalf("no notify expected on failure, got %d", n.pings)
	}
}
