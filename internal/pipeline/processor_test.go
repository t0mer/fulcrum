package pipeline

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"path/filepath"
	"testing"

	"github.com/t0mer/fulcrum/internal/ml"
	"github.com/t0mer/fulcrum/internal/sink"
	"github.com/t0mer/fulcrum/internal/store"
	"github.com/t0mer/fulcrum/internal/whatsapp"
)

type fakeDownloader struct {
	data []byte
	mime string
}

func (f *fakeDownloader) DownloadMedia(context.Context, whatsapp.InboundMessage) ([]byte, string, error) {
	return f.data, f.mime, nil
}

type fakeDetector struct{ faces []ml.Face }

func (f *fakeDetector) Detect(context.Context, []byte, string) ([]ml.Face, error) {
	return f.faces, nil
}

type fakeSender struct{ calls int }

func (f *fakeSender) SendImage(context.Context, string, []byte, string, string) error {
	f.calls++
	return nil
}

func setup(t *testing.T, faces []ml.Face, mode string) (*Processor, *store.Store, *fakeSender) {
	t.Helper()
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "t.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { st.Close() })

	sender := &fakeSender{}
	proc := New(st,
		&fakeDownloader{data: []byte("IMG"), mime: "image/jpeg"},
		&fakeDetector{faces: faces},
		&sink.Forward{Sender: sender, DestinationGroupID: "dest@g.us"},
		Config{DefaultThreshold: 0.5, SinkMode: mode, StoragePath: filepath.Join(dir, "matches")},
		nil, nil)
	return proc, st, sender
}

func enrolledFace(t *testing.T, st *store.Store, embedding []float32) *store.Subject {
	t.Helper()
	sub, err := st.CreateSubject("Yael", "yael", nil)
	if err != nil {
		t.Fatalf("CreateSubject: %v", err)
	}
	if _, err := st.AddFace(sub.ID, embedding, "/faces/yael/a.jpg"); err != nil {
		t.Fatalf("AddFace: %v", err)
	}
	return sub
}

func job() store.Job {
	return store.Job{ID: 1, Provider: "gowa", ProviderGroupID: "src@g.us", MessageID: "MSG1", MediaRef: "b64:image/jpeg:SUdN"}
}

func TestPipelineMatchStoresAndForwards(t *testing.T) {
	emb := []float32{1, 0, 0}
	proc, st, sender := setup(t, []ml.Face{{Embedding: emb, DetScore: 0.99}}, "both")
	enrolledFace(t, st, emb)

	if err := proc.Process(context.Background(), job()); err != nil {
		t.Fatalf("Process: %v", err)
	}
	matches, _ := st.ListMatches(store.MatchFilter{})
	if len(matches) != 1 {
		t.Fatalf("matches = %d, want 1", len(matches))
	}
	if !matches[0].Forwarded {
		t.Error("match should be marked forwarded")
	}
	if matches[0].StoredPath == "" {
		t.Error("match should have a stored path")
	}
	if sender.calls != 1 {
		t.Errorf("forward calls = %d, want 1", sender.calls)
	}
}

func TestPipelineNoFaceDiscards(t *testing.T) {
	proc, st, sender := setup(t, nil, "both")
	enrolledFace(t, st, []float32{1, 0, 0})

	if err := proc.Process(context.Background(), job()); err != nil {
		t.Fatalf("Process: %v", err)
	}
	if matches, _ := st.ListMatches(store.MatchFilter{}); len(matches) != 0 {
		t.Errorf("matches = %d, want 0 (no face)", len(matches))
	}
	if sender.calls != 0 {
		t.Errorf("forward calls = %d, want 0", sender.calls)
	}
}

func TestPipelineBelowThresholdDiscards(t *testing.T) {
	// Detected face is orthogonal to the enrolled one -> similarity 0.
	proc, st, _ := setup(t, []ml.Face{{Embedding: []float32{0, 1, 0}, DetScore: 0.99}}, "both")
	enrolledFace(t, st, []float32{1, 0, 0})

	if err := proc.Process(context.Background(), job()); err != nil {
		t.Fatalf("Process: %v", err)
	}
	if matches, _ := st.ListMatches(store.MatchFilter{}); len(matches) != 0 {
		t.Errorf("matches = %d, want 0 (below threshold)", len(matches))
	}
}

func TestPipelineDedupSkipsSecondTime(t *testing.T) {
	emb := []float32{1, 0, 0}
	proc, st, _ := setup(t, []ml.Face{{Embedding: emb, DetScore: 0.99}}, "storage-only")
	enrolledFace(t, st, emb)

	_ = proc.Process(context.Background(), job())
	// Same bytes again -> dedup, no second match.
	_ = proc.Process(context.Background(), job())
	if matches, _ := st.ListMatches(store.MatchFilter{}); len(matches) != 1 {
		t.Errorf("matches = %d, want 1 (dedup)", len(matches))
	}
}

// seqDownloader returns a different payload on each call.
type seqDownloader struct {
	payloads [][]byte
	i        int
}

func (d *seqDownloader) DownloadMedia(context.Context, whatsapp.InboundMessage) ([]byte, string, error) {
	p := d.payloads[d.i%len(d.payloads)]
	d.i++
	return p, "image/jpeg", nil
}

func testImage() image.Image {
	img := image.NewRGBA(image.Rect(0, 0, 48, 48))
	for y := 0; y < 48; y++ {
		for x := 0; x < 48; x++ {
			img.Set(x, y, color.RGBA{uint8(x * 5), uint8(y * 5), 90, 255})
		}
	}
	return img
}

func TestPipelineSkipsPerceptualNearDuplicate(t *testing.T) {
	img := testImage()
	var pngBuf, jpgBuf bytes.Buffer
	_ = png.Encode(&pngBuf, img)
	_ = jpeg.Encode(&jpgBuf, img, &jpeg.Options{Quality: 88})

	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "t.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { st.Close() })

	emb := []float32{1, 0, 0}
	sub, _ := st.CreateSubject("Yael", "yael", nil)
	st.AddFace(sub.ID, emb, "/f.jpg")

	proc := New(st,
		&seqDownloader{payloads: [][]byte{pngBuf.Bytes(), jpgBuf.Bytes()}},
		&fakeDetector{faces: []ml.Face{{Embedding: emb, DetScore: 0.99}}},
		&sink.Forward{Sender: &fakeSender{}},
		Config{DefaultThreshold: 0.5, SinkMode: "storage-only", StoragePath: filepath.Join(dir, "m"), NearDupDistance: 4},
		nil, nil)

	// Two jobs, different message ids and different encodings of the same image.
	j1 := store.Job{ID: 1, ProviderGroupID: "g@g.us", MessageID: "M1", MediaRef: "r1"}
	j2 := store.Job{ID: 2, ProviderGroupID: "g@g.us", MessageID: "M2", MediaRef: "r2"}
	if err := proc.Process(context.Background(), j1); err != nil {
		t.Fatalf("job1: %v", err)
	}
	if err := proc.Process(context.Background(), j2); err != nil {
		t.Fatalf("job2: %v", err)
	}

	matches, _ := st.ListMatches(store.MatchFilter{})
	if len(matches) != 1 {
		t.Fatalf("matches = %d, want 1 (near-duplicate skipped)", len(matches))
	}
}

func TestPipelineForwardOnlySkipsStorage(t *testing.T) {
	emb := []float32{1, 0, 0}
	proc, st, sender := setup(t, []ml.Face{{Embedding: emb, DetScore: 0.99}}, "forward-only")
	enrolledFace(t, st, emb)

	if err := proc.Process(context.Background(), job()); err != nil {
		t.Fatalf("Process: %v", err)
	}
	matches, _ := st.ListMatches(store.MatchFilter{})
	if len(matches) != 1 || matches[0].StoredPath != "" {
		t.Errorf("forward-only should not store a path, got %+v", matches)
	}
	if sender.calls != 1 {
		t.Errorf("forward calls = %d, want 1", sender.calls)
	}
}
