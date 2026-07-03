// Package intake holds the single per-message admission path shared by every
// inbound source: the HTTP webhook route (gowa) and the polling receive loop
// (greenapi). It applies the "image from a monitored group" filter, enqueues a
// durable job, and wakes the worker pool. See CLAUDE.md §5.
package intake

import (
	"context"
	"log/slog"

	"github.com/t0mer/fulcrum/internal/metrics"
	"github.com/t0mer/fulcrum/internal/whatsapp"
)

// Store is the subset of the persistence layer intake needs.
type Store interface {
	IsMonitored(providerGroupID string) (bool, error)
	EnqueueJob(provider, providerGroupID, messageID, mediaRef string) (int64, error)
	GroupName(providerGroupID string) string
}

// Notifier is woken when new work is enqueued.
type Notifier interface{ Notify() }

// Service admits inbound messages into the job queue.
type Service struct {
	store    Store
	metrics  *metrics.Metrics
	notifier Notifier
	provName string
	log      *slog.Logger
}

// New builds an intake service. metrics and notifier may be nil.
func New(st Store, m *metrics.Metrics, n Notifier, provName string, log *slog.Logger) *Service {
	if log == nil {
		log = slog.Default()
	}
	return &Service{store: st, metrics: m, notifier: n, provName: provName, log: log}
}

// Accept admits a single inbound message. It records the inbound metric, drops
// anything that is not an image from a monitored group, enqueues the rest, and
// wakes the worker pool. It returns true when a job was enqueued. Accept is
// best-effort: store errors are logged and swallowed, never returned, so a
// misbehaving row can't stall the caller's hot path.
func (s *Service) Accept(_ context.Context, m whatsapp.InboundMessage) bool {
	s.incInbound(m.ProviderGroupID)
	if !m.IsImage {
		return false
	}
	monitored, err := s.store.IsMonitored(m.ProviderGroupID)
	if err != nil {
		s.log.Error("check monitored", "err", err)
		return false
	}
	if !monitored {
		return false
	}
	if _, err := s.store.EnqueueJob(s.provName, m.ProviderGroupID, m.MessageID, m.MediaRef); err != nil {
		s.log.Error("enqueue job", "err", err)
		return false
	}
	if s.notifier != nil {
		s.notifier.Notify()
	}
	return true
}

func (s *Service) incInbound(groupID string) {
	if s.metrics != nil {
		s.metrics.InboundMessages.WithLabelValues(s.provName, s.store.GroupName(groupID)).Inc()
	}
}
