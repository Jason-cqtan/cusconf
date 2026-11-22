package tester

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cusconf/cusconf/internal/node"
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

func TestImportSources(t *testing.T) {
	m := newManager(t)
	ss := func(at int64, names ...string) []*node.Node {
		var out []*node.Node
		for _, n := range names {
			out = append(out, &node.Node{Name: n, Type: node.SS, Server: n + ".example.com", Port: 443, Cipher: "aes-128-gcm", Password: "x"})
		}
		return out
	}
	// A server-side source whose own fetch fails, and one that works and is newer.
	m.st.PutSource(&store.Source{ID: "blocked", Name: "B", Kind: "url", URL: "https://b/sub", Enabled: true, FetchedAt: 200, LastError: "HTTP 403"})
	m.st.PutSource(&store.Source{ID: "fresh", Name: "F", Kind: "url", URL: "https://f/sub", Enabled: true, FetchedAt: 300, NodeCount: 1})
	m.st.PutNodes("fresh", ss(300, "own"))

	n, err := m.ImportSources([]SyncSource{
		{Name: "B", Kind: "url", URL: "https://b/sub", FetchedAt: 100, Nodes: ss(100, "b1", "b2"), Info: &store.SubInfo{Total: 9}},
		{Name: "F", Kind: "url", URL: "https://f/sub", FetchedAt: 250, Nodes: ss(250, "old")},
		{Name: "New", Kind: "url", URL: "https://n/sub", FetchedAt: 100, Nodes: ss(100, "n1")},
		{Name: "Mine", Kind: "manual", Content: "ss://x", SelfHosted: true, FetchedAt: 100, Nodes: ss(100, "m1")},
	})
	if err != nil || n != 3 {
		t.Fatalf("updated %d, err %v", n, err)
	}
	if b, _ := m.st.GetNodes("blocked"); len(b) != 2 || b[0].Source != "blocked" {
		t.Errorf("blocked source should get pushed nodes: %+v", b)
	}
	if bs, _ := m.st.GetSource("blocked"); bs.SyncedAt == 0 || bs.Info == nil || bs.Info.Total != 9 || bs.NodeCount != 2 {
		t.Errorf("blocked source status: %+v", bs)
	}
	if f, _ := m.st.GetNodes("fresh"); len(f) != 1 || f[0].Name != "own" {
		t.Error("own newer successful fetch must not be replaced")
	}
	srcs, _ := m.st.ListSources()
	byName := map[string]store.Source{}
	for _, s := range srcs {
		byName[s.Name] = s
	}
	if s, ok := byName["New"]; !ok || !s.Enabled || s.URL != "https://n/sub" {
		t.Errorf("missing source should be created: %+v", s)
	}
	if s := byName["Mine"]; s.Kind != "manual" || !s.SelfHosted || s.Content != "ss://x" {
		t.Errorf("manual source: %+v", s)
	}
	// Received sources are not pushed on.
	local, _ := m.localSources()
	for _, s := range local {
		if s.Name == "B" || s.Name == "New" {
			t.Errorf("received source %s would be pushed on", s.Name)
		}
	}
}

func TestSyncPushNodes(t *testing.T) {
	recv := newManager(t)
	key, _ := recv.SyncKey()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Results []Result
			Sources []SyncSource
		}
		json.NewDecoder(r.Body).Decode(&in)
		ns, _ := recv.ImportSources(in.Sources)
		json.NewEncoder(w).Encode(map[string]int{"imported": recv.Import(in.Results), "sources": ns})
	}))
	defer srv.Close()
	send := newManager(t)
	send.st.PutSource(&store.Source{ID: "s", Name: "A", Kind: "url", URL: "https://a/sub", Enabled: true, FetchedAt: 10})
	send.st.PutNodes("s", []*node.Node{{Name: "a1", Type: node.SS, Server: "a1.example.com", Port: 1, Cipher: "aes-128-gcm", Password: "x"}})
	s := send.Settings()
	s.Sync = SyncSettings{URL: srv.URL, Key: key}
	send.SaveSettings(s)
	if st := send.Push(context.Background()); st.Error != "" || st.Sources != 1 {
		t.Fatalf("push: %+v", st)
	}
	all, _ := recv.st.AllNodes()
	if len(all) != 1 || all[0].Name != "a1" {
		t.Errorf("receiver nodes: %+v", all)
	}
	s.Sync.NoNodes = true
	send.SaveSettings(s)
	if st := send.Push(context.Background()); st.Sources != 0 {
		t.Errorf("nodes pushed although turned off: %+v", st)
	}
}

func TestMatchSource(t *testing.T) {
	m := newManager(t)
	nd := func(names ...string) []*node.Node {
		var out []*node.Node
		for _, n := range names {
			out = append(out, &node.Node{Name: n, Type: node.SS, Server: n + ".example.com", Port: 443, Cipher: "aes-128-gcm", Password: "x"})
		}
		return out
	}
	srcs := []store.Source{
		{ID: "a", Name: "Airport B", Kind: "url", URL: "https://203.0.113.10/sub?token=1&target=surge"},
		{ID: "y1", Name: "Y1", Kind: "url", URL: "https://panel.example/sub?token=1"},
		{ID: "y2", Name: "Y2", Kind: "url", URL: "https://panel.example/sub?token=2"},
		{ID: "m", Name: "My VPS", Kind: "manual"},
	}
	m.st.PutNodes("y2", nd("p", "q", "r"))
	cases := []struct {
		p    SyncSource
		want string
	}{
		{SyncSource{Name: "airport-b", Kind: "url", URL: "https://203.0.113.10/sub?token=1&target=clash"}, "a"},                 // host + path
		{SyncSource{Name: "x", Kind: "url", URL: "https://panel.example/sub?token=2&flag=meta", Nodes: nd("p", "q", "z")}, "y2"}, // ambiguous host: node overlap
		{SyncSource{Name: "my vps", Kind: "manual"}, "m"},                                                                        // name, any case
		{SyncSource{Name: "new", Kind: "url", URL: "https://other.example/sub"}, ""},
	}
	for _, c := range cases {
		got := m.matchSource(srcs, c.p)
		if (got == nil && c.want != "") || (got != nil && got.ID != c.want) {
			t.Errorf("%s: got %v, want %q", c.p.Name, got, c.want)
		}
	}
}
