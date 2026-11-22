package tester

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cusconf/cusconf/internal/store"
)

func newManager(t *testing.T) *Manager {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return NewManager(st)
}

func TestSyncMergeRules(t *testing.T) {
	m := newManager(t)
	ok := Result{Key: "k1", Name: "n", TestedAt: 100, Latency: 50}
	if n := m.Import([]Result{ok, {Key: "k2", TestedAt: 0}}); n != 1 {
		t.Fatalf("imported %d", n)
	}
	if !m.Results()["k1"].Synced {
		t.Error("imported result should be marked synced")
	}
	// Older pushes do not replace newer data.
	if n := m.Import([]Result{{Key: "k1", TestedAt: 90, Latency: 10}}); n != 0 {
		t.Error("older result imported")
	}
	// A local failure keeps the synced success…
	if r := m.save(Result{Key: "k1", TestedAt: 200, Error: "EOF"}, Checks{}); r.Latency != 50 || !r.Synced {
		t.Errorf("synced success overwritten: %+v", r)
	}
	// …but a local success replaces it.
	if r := m.save(Result{Key: "k1", TestedAt: 300, Latency: 70}, Checks{}); r.Latency != 70 || r.Synced {
		t.Errorf("local success not stored: %+v", r)
	}
	// Persisted, not only cached.
	m2 := NewManager(m.st)
	if m2.Results()["k1"].Latency != 70 {
		t.Error("result not persisted")
	}
}

func TestSyncPush(t *testing.T) {
	recv := newManager(t)
	key, _ := recv.SyncKey()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/sync/results" || r.Header.Get("Authorization") != "Bearer "+key {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		var in struct{ Results []Result }
		json.NewDecoder(r.Body).Decode(&in)
		json.NewEncoder(w).Encode(map[string]int{"imported": recv.Import(in.Results)})
	}))
	defer srv.Close()

	send := newManager(t)
	send.store(Result{Key: "a", TestedAt: 10, Latency: 30})
	send.store(Result{Key: "b", TestedAt: 10, Latency: 40, Synced: true}) // received elsewhere: not pushed on
	s := send.Settings()
	s.Sync = SyncSettings{URL: srv.URL + "/", Key: key}
	send.SaveSettings(s)

	st := send.Push(context.Background())
	if st.Error != "" || st.Count != 1 {
		t.Fatalf("push: %+v", st)
	}
	if r := recv.Results(); r["a"].Latency != 30 || !r["a"].Synced || r["b"].Key != "" {
		t.Errorf("receiver got %+v", r)
	}
	if send.Status().Sync == nil {
		t.Error("status should report the last push")
	}

	s.Sync.Key = "wrong"
	send.SaveSettings(s)
	if st := send.Push(context.Background()); !strings.Contains(st.Error, "密钥") {
		t.Errorf("wrong key: %+v", st)
	}
}
