package api

import (
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/t0mer/fulcrum/internal/enroll"
	"github.com/t0mer/fulcrum/internal/store"
)

func newMatchesAPI(t *testing.T) (http.Handler, *store.Store, string) {
	t.Helper()
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "t.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	faces := filepath.Join(dir, "faces")
	svc := enroll.New(st, nil, faces)
	return New(Deps{Store: st, Enroll: svc}).Routes(), st, dir
}

func seedMatch(t *testing.T, st *store.Store, dir string, emb []float32, stored bool) int64 {
	t.Helper()
	sub, err := st.CreateSubject("Yael", "yael", nil)
	if err != nil {
		// subject may already exist across calls; fetch it
		subs, _ := st.ListSubjects()
		sub = &subs[0]
	}
	path := ""
	if stored {
		path = filepath.Join(dir, "match.jpg")
		if err := os.WriteFile(path, []byte("JPEGDATA"), 0o640); err != nil {
			t.Fatal(err)
		}
	}
	id, err := st.CreateMatch(store.Match{
		MessageID: "M" + sub.Slug, ImageSHA256: "sha", SubjectID: sub.ID,
		Similarity: 0.6, SourceGroupID: "g@g.us", StoredPath: path, Embedding: emb,
	})
	if err != nil {
		t.Fatalf("CreateMatch: %v", err)
	}
	return id
}

func TestConfirmReinforcesReferences(t *testing.T) {
	h, st, dir := newMatchesAPI(t)
	id := seedMatch(t, st, dir, []float32{0.1, 0.2, 0.3}, true)

	rec := do(t, h, http.MethodPost, "/matches/"+itoa(id)+"/review", `{"decision":"confirm"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("review = %d (%s), want 200", rec.Code, rec.Body)
	}
	// A new reference face should now exist for the subject.
	subs, _ := st.ListSubjects()
	faces, _ := st.ListFaces(subs[0].ID)
	if len(faces) != 1 {
		t.Fatalf("reinforced faces = %d, want 1", len(faces))
	}
	// And the reinforced file should live under the faces tree.
	if _, err := os.Stat(faces[0].SourcePath); err != nil {
		t.Errorf("reinforced file missing: %v", err)
	}
}

func TestConfirmWithoutReinforce(t *testing.T) {
	h, st, dir := newMatchesAPI(t)
	id := seedMatch(t, st, dir, []float32{0.1, 0.2, 0.3}, true)

	rec := do(t, h, http.MethodPost, "/matches/"+itoa(id)+"/review", `{"decision":"confirm","reinforce":false}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("review = %d, want 200", rec.Code)
	}
	subs, _ := st.ListSubjects()
	if faces, _ := st.ListFaces(subs[0].ID); len(faces) != 0 {
		t.Errorf("faces = %d, want 0 (reinforce disabled)", len(faces))
	}
}

func TestRejectRecordsHardNegativeAndDeletesFile(t *testing.T) {
	h, st, dir := newMatchesAPI(t)
	id := seedMatch(t, st, dir, []float32{0.4, 0.5, 0.6}, true)
	m, _ := st.GetMatch(id)

	rec := do(t, h, http.MethodPost, "/matches/"+itoa(id)+"/review", `{"decision":"reject"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("review = %d, want 200", rec.Code)
	}
	// File deleted.
	if _, err := os.Stat(m.StoredPath); !os.IsNotExist(err) {
		t.Errorf("rejected file should be deleted")
	}
	// Hard negative recorded.
	var n int
	st.DB().QueryRow("SELECT COUNT(1) FROM hard_negatives").Scan(&n)
	if n != 1 {
		t.Errorf("hard_negatives = %d, want 1", n)
	}
}

func itoa(i int64) string {
	return strconv.FormatInt(i, 10)
}

func TestDeleteSubjectRemovesMatchDir(t *testing.T) {
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "t.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	matchesRoot := filepath.Join(dir, "matches")
	svc := enroll.New(st, nil, filepath.Join(dir, "faces"))
	h := New(Deps{Store: st, Enroll: svc, MatchesPath: matchesRoot}).Routes()

	sub, _ := st.CreateSubject("Yael", "yael", nil)
	matchDir := filepath.Join(matchesRoot, "yael", "2026", "07")
	if err := os.MkdirAll(matchDir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(matchDir, "m.jpg"), []byte("x"), 0o640); err != nil {
		t.Fatal(err)
	}

	rec := do(t, h, http.MethodDelete, "/subjects/"+itoa(sub.ID), "")
	if rec.Code != http.StatusOK {
		t.Fatalf("delete = %d, want 200", rec.Code)
	}
	if _, err := os.Stat(filepath.Join(matchesRoot, "yael")); !os.IsNotExist(err) {
		t.Errorf("matches/yael should be removed after subject delete")
	}
}
