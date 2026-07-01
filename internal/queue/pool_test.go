package queue

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/t0mer/fulcrum/internal/store"
)

func newStore(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

type handlerFunc func(context.Context, store.Job) error

func (h handlerFunc) Process(ctx context.Context, j store.Job) error { return h(ctx, j) }

func TestPoolProcessesEnqueuedJobs(t *testing.T) {
	st := newStore(t)
	var processed int64
	var wg sync.WaitGroup
	wg.Add(3)
	h := handlerFunc(func(_ context.Context, _ store.Job) error {
		atomic.AddInt64(&processed, 1)
		wg.Done()
		return nil
	})
	pool := New(st, h, Options{Workers: 2, PollInterval: 20 * time.Millisecond})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go pool.Run(ctx)

	for i := 0; i < 3; i++ {
		if _, err := st.EnqueueJob("gowa", "g@g.us", "m", "ref"); err != nil {
			t.Fatalf("enqueue: %v", err)
		}
	}
	pool.Notify()

	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatalf("only %d/3 jobs processed", atomic.LoadInt64(&processed))
	}
}

func TestPoolRetriesThenMarksDead(t *testing.T) {
	st := newStore(t)
	h := handlerFunc(func(_ context.Context, _ store.Job) error {
		return errors.New("always fails")
	})
	pool := New(st, h, Options{Workers: 1, MaxAttempts: 2, PollInterval: 10 * time.Millisecond})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go pool.Run(ctx)

	id, _ := st.EnqueueJob("gowa", "g@g.us", "m", "ref")
	pool.Notify()

	// After MaxAttempts failures the job should end up 'dead'.
	deadline := time.After(2 * time.Second)
	for {
		var status string
		if err := st.DB().QueryRow(`SELECT status FROM jobs WHERE id = ?`, id).Scan(&status); err != nil {
			t.Fatalf("query status: %v", err)
		}
		if status == "dead" {
			return
		}
		select {
		case <-deadline:
			t.Fatalf("job never marked dead (last status %q)", status)
		case <-time.After(20 * time.Millisecond):
		}
	}
}

func TestRequeueStuckJobs(t *testing.T) {
	st := newStore(t)
	id, _ := st.EnqueueJob("gowa", "g@g.us", "m", "ref")
	// Simulate a crash mid-processing.
	if _, err := st.DB().Exec(`UPDATE jobs SET status='processing' WHERE id=?`, id); err != nil {
		t.Fatal(err)
	}
	n, err := st.RequeueStuckJobs()
	if err != nil || n != 1 {
		t.Fatalf("RequeueStuckJobs = %d, %v; want 1, nil", n, err)
	}
	pending, _ := st.PendingJobs()
	if pending != 1 {
		t.Errorf("pending = %d, want 1", pending)
	}
}
