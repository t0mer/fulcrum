// Package queue runs a pool of workers over the durable SQLite job queue.
// Intake never blocks on processing: the webhook enqueues a job and returns,
// and workers drain the queue. See CLAUDE.md §5.
package queue

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/t0mer/fulcrum/internal/store"
)

// Handler processes a single claimed job.
type Handler interface {
	Process(ctx context.Context, job store.Job) error
}

// DepthReporter is an optional hook to publish queue depth after each drain.
type DepthReporter func(depth int)

// Pool coordinates worker goroutines.
type Pool struct {
	store       *store.Store
	handler     Handler
	workers     int
	maxAttempts int
	log         *slog.Logger
	onDepth     DepthReporter

	notify chan struct{}
	// pollInterval is the fallback wake when no Notify arrives.
	pollInterval time.Duration
}

// Options configure a Pool.
type Options struct {
	Workers      int
	MaxAttempts  int
	Logger       *slog.Logger
	OnDepth      DepthReporter
	PollInterval time.Duration
}

// New builds a pool.
func New(st *store.Store, h Handler, opts Options) *Pool {
	if opts.Workers < 1 {
		opts.Workers = 1
	}
	if opts.MaxAttempts < 1 {
		opts.MaxAttempts = 5
	}
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	if opts.PollInterval <= 0 {
		opts.PollInterval = 5 * time.Second
	}
	return &Pool{
		store: st, handler: h,
		workers: opts.Workers, maxAttempts: opts.MaxAttempts,
		log: opts.Logger, onDepth: opts.OnDepth,
		notify: make(chan struct{}, 1), pollInterval: opts.PollInterval,
	}
}

// Notify wakes a worker to check for newly enqueued work (non-blocking).
func (p *Pool) Notify() {
	select {
	case p.notify <- struct{}{}:
	default:
	}
}

// Run starts the workers and blocks until ctx is cancelled. Any job left
// 'processing' from a prior crash is requeued first.
func (p *Pool) Run(ctx context.Context) {
	if n, err := p.store.RequeueStuckJobs(); err != nil {
		p.log.Warn("requeue stuck jobs", "err", err)
	} else if n > 0 {
		p.log.Info("requeued stuck jobs", "count", n)
	}

	var wg sync.WaitGroup
	for i := 0; i < p.workers; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			p.worker(ctx, id)
		}(i)
	}
	wg.Wait()
}

func (p *Pool) worker(ctx context.Context, id int) {
	ticker := time.NewTicker(p.pollInterval)
	defer ticker.Stop()

	for {
		drained := p.drain(ctx)
		if ctx.Err() != nil {
			return
		}
		if drained {
			continue // keep draining while work remains
		}
		select {
		case <-ctx.Done():
			return
		case <-p.notify:
		case <-ticker.C:
		}
	}
}

// drain processes jobs until the queue is empty or ctx is cancelled. Returns
// true if it processed at least one job.
func (p *Pool) drain(ctx context.Context) bool {
	processed := false
	for {
		if ctx.Err() != nil {
			return processed
		}
		job, err := p.store.ClaimJob()
		if errors.Is(err, store.ErrNoJob) {
			p.reportDepth()
			return processed
		}
		if err != nil {
			p.log.Error("claim job", "err", err)
			return processed
		}
		processed = true
		p.handle(ctx, *job)
		p.reportDepth()
	}
}

func (p *Pool) handle(ctx context.Context, job store.Job) {
	if err := p.handler.Process(ctx, job); err != nil {
		dead, ferr := p.store.FailJob(job.ID, p.maxAttempts)
		if ferr != nil {
			p.log.Error("fail job", "id", job.ID, "err", ferr)
			return
		}
		lvl := slog.LevelWarn
		if dead {
			lvl = slog.LevelError
		}
		p.log.Log(ctx, lvl, "job failed", "id", job.ID, "attempts", job.Attempts, "dead", dead, "err", err)
		return
	}
	if err := p.store.CompleteJob(job.ID); err != nil {
		p.log.Error("complete job", "id", job.ID, "err", err)
	}
}

func (p *Pool) reportDepth() {
	if p.onDepth == nil {
		return
	}
	if n, err := p.store.PendingJobs(); err == nil {
		p.onDepth(n)
	}
}
