// Package pipeline implements the per-image processing a queue worker runs:
// download → dedup → detect → match → sink. Non-matching media is never
// persisted (CLAUDE.md §5, §17).
package pipeline

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/t0mer/fulcrum/internal/match"
	"github.com/t0mer/fulcrum/internal/metrics"
	"github.com/t0mer/fulcrum/internal/ml"
	"github.com/t0mer/fulcrum/internal/sink"
	"github.com/t0mer/fulcrum/internal/store"
	"github.com/t0mer/fulcrum/internal/whatsapp"
)

// Downloader resolves a message's media to bytes.
type Downloader interface {
	DownloadMedia(ctx context.Context, m whatsapp.InboundMessage) ([]byte, string, error)
}

// Detector runs face detection + embedding.
type Detector interface {
	Detect(ctx context.Context, image []byte, filename string) ([]ml.Face, error)
}

// Config holds the matching/sink knobs the pipeline reads.
type Config struct {
	DefaultThreshold float64
	SinkMode         string // storage-only | forward-only | both
	StoragePath      string
}

// Processor is the queue Handler for image jobs.
type Processor struct {
	store      *store.Store
	downloader Downloader
	detector   Detector
	forward    *sink.Forward
	fs         *sink.FS
	cfg        Config
	metrics    *metrics.Metrics
	log        *slog.Logger
}

// New builds a Processor.
func New(st *store.Store, dl Downloader, det Detector, fwd *sink.Forward, cfg Config, m *metrics.Metrics, log *slog.Logger) *Processor {
	if log == nil {
		log = slog.Default()
	}
	return &Processor{
		store: st, downloader: dl, detector: det, forward: fwd,
		fs:  &sink.FS{Root: cfg.StoragePath},
		cfg: cfg, metrics: m, log: log,
	}
}

// effectiveSinkMode / effectiveThreshold prefer the runtime setting (adjustable
// from the Settings page) and fall back to the startup config.
func (p *Processor) effectiveSinkMode() string {
	if v, ok, _ := p.store.GetSetting(store.SettingSinkMode); ok {
		switch v {
		case "storage-only", "forward-only", "both":
			return v
		}
	}
	return p.cfg.SinkMode
}

func (p *Processor) effectiveThreshold() float64 {
	if v, ok, _ := p.store.GetSetting(store.SettingGlobalThreshold); ok {
		if f, err := strconv.ParseFloat(v, 64); err == nil && f > 0 && f < 1 {
			return f
		}
	}
	return p.cfg.DefaultThreshold
}

func storeEnabled(mode string) bool   { return mode != "forward-only" }
func forwardEnabled(mode string) bool { return mode != "storage-only" }

// Process runs the pipeline for one job.
func (p *Processor) Process(ctx context.Context, job store.Job) error {
	msg := whatsapp.InboundMessage{
		ProviderGroupID: job.ProviderGroupID,
		MessageID:       job.MessageID,
		IsImage:         true,
		MediaRef:        job.MediaRef,
	}

	data, mime, err := p.downloader.DownloadMedia(ctx, msg)
	if err != nil {
		return fmt.Errorf("download: %w", err)
	}

	// Content dedup: skip images already processed.
	sum := sha256.Sum256(data)
	sha := hex.EncodeToString(sum[:])
	fresh, err := p.store.MarkSeen(sha)
	if err != nil {
		return err
	}
	if !fresh {
		p.log.Debug("duplicate media, skipping", "message_id", job.MessageID)
		return nil
	}

	p.incImagesProcessed()

	faces, err := p.detector.Detect(ctx, data, "job"+extForMime(mime))
	if err != nil {
		return fmt.Errorf("detect: %w", err)
	}
	p.addFacesDetected(len(faces))
	if len(faces) == 0 {
		return nil // nothing to match; media discarded
	}

	refs, subjects, err := p.loadReferences()
	if err != nil {
		return err
	}
	if len(refs) == 0 {
		return nil // no enrolled subjects yet
	}

	// Best-scoring face per subject across all detected faces (one image can
	// match several kids, but each kid at most once). We keep the winning
	// face's embedding so a confirmed match can reinforce the references.
	gt := p.effectiveThreshold()
	type hit struct {
		sim float64
		emb []float32
	}
	bestPerSubject := map[int64]hit{}
	for _, f := range faces {
		res, ok := match.Best(f.Embedding, refs, thresholdFor(subjects, gt))
		if !ok {
			continue
		}
		if cur, seen := bestPerSubject[res.SubjectID]; !seen || res.Similarity > cur.sim {
			bestPerSubject[res.SubjectID] = hit{sim: res.Similarity, emb: f.Embedding}
		}
	}
	if len(bestPerSubject) == 0 {
		return nil // no match; media discarded
	}

	mode := p.effectiveSinkMode()
	for subjectID, h := range bestPerSubject {
		sub := subjects[subjectID]
		if err := p.deliver(ctx, job, sub, h.sim, h.emb, data, mime, mode); err != nil {
			p.log.Error("deliver match", "subject", sub.Slug, "err", err)
		}
	}
	return nil
}

func (p *Processor) deliver(ctx context.Context, job store.Job, sub store.Subject, sim float64, emb []float32, data []byte, mime, mode string) error {
	var storedPath string
	if storeEnabled(mode) {
		path, err := p.fs.Save(sub.Slug, job.MessageID, data, mime, time.Time{})
		if err != nil {
			p.sinkError("fs")
			return err
		}
		storedPath = path
	}

	matchID, err := p.store.CreateMatch(store.Match{
		MessageID:     job.MessageID,
		ImageSHA256:   shaOf(data),
		SubjectID:     sub.ID,
		Similarity:    sim,
		SourceGroupID: job.ProviderGroupID,
		StoredPath:    storedPath,
		Embedding:     emb,
	})
	if err != nil {
		return err
	}
	p.incMatch(sub.Slug)

	if forwardEnabled(mode) && p.forward != nil {
		caption := p.caption(sub.Name, sim, job.ProviderGroupID)
		if err := p.forward.Send(ctx, data, mime, caption); err != nil {
			p.sinkError("whatsapp")
			return err
		}
		if matchID != 0 {
			if err := p.store.SetForwarded(matchID); err != nil {
				return err
			}
		}
	}
	return nil
}

func (p *Processor) caption(name string, sim float64, groupID string) string {
	return fmt.Sprintf("%s · %.2f · from %s · %s",
		name, sim, p.store.GroupName(groupID), time.Now().Format("2006-01-02 15:04"))
}

// loadReferences builds the matcher inputs and a subject lookup.
func (p *Processor) loadReferences() ([]match.Reference, map[int64]store.Subject, error) {
	faces, err := p.store.AllFaces()
	if err != nil {
		return nil, nil, err
	}
	refs := make([]match.Reference, 0, len(faces))
	for _, f := range faces {
		refs = append(refs, match.Reference{SubjectID: f.SubjectID, Embedding: f.Embedding})
	}
	subs, err := p.store.ListSubjects()
	if err != nil {
		return nil, nil, err
	}
	byID := make(map[int64]store.Subject, len(subs))
	for _, s := range subs {
		byID[s.ID] = s
	}
	return refs, byID, nil
}

func thresholdFor(subjects map[int64]store.Subject, global float64) func(int64) float64 {
	return func(id int64) float64 {
		if s, ok := subjects[id]; ok && s.Threshold != nil {
			return *s.Threshold
		}
		return global
	}
}

// --- nil-safe metrics helpers ---

func (p *Processor) incImagesProcessed() {
	if p.metrics != nil {
		p.metrics.ImagesProcessed.Inc()
	}
}
func (p *Processor) addFacesDetected(n int) {
	if p.metrics != nil {
		p.metrics.FacesDetected.Add(float64(n))
	}
}
func (p *Processor) incMatch(subject string) {
	if p.metrics != nil {
		p.metrics.Matches.WithLabelValues(subject).Inc()
	}
}
func (p *Processor) sinkError(sinkName string) {
	if p.metrics != nil {
		p.metrics.SinkErrors.WithLabelValues(sinkName).Inc()
	}
}

func shaOf(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func extForMime(mime string) string {
	switch mime {
	case "image/png":
		return ".png"
	case "image/webp":
		return ".webp"
	default:
		return ".jpg"
	}
}
