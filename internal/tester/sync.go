package tester

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/cusconf/cusconf/internal/node"
	"github.com/cusconf/cusconf/internal/store"
)

// Some nodes only accept connections from certain networks (e.g. relay entries
// that only admit mainland China IPs), so a server elsewhere cannot test them.
// Sync lets another cusconf that can reach them (e.g. one at home) push its
// results here; they are then used like local results.

// SyncSettings configures pushing this instance's results to another cusconf.
type SyncSettings struct {
	URL  string `json:"url,omitempty"`  // base URL of the receiving cusconf, e.g. https://sub.example.com
	Key  string `json:"key,omitempty"`  // the receiver's sync key
	Auto bool   `json:"auto,omitempty"` // push after every test job and single-node test
	// NoNodes stops pushing the nodes of this instance's sources. By default they are
	// pushed, so a receiver that cannot fetch some subscriptions still has their nodes.
	NoNodes bool `json:"noNodes,omitempty"`
}

// SyncSource is a source and its nodes as pushed to another instance.
type SyncSource struct {
	Name       string         `json:"name"`
	Kind       string         `json:"kind"`
	URL        string         `json:"url,omitempty"`
	UserAgent  string         `json:"userAgent,omitempty"`
	Content    string         `json:"content,omitempty"` // manual sources
	SelfHosted bool           `json:"selfHosted,omitempty"`
	Interval   int            `json:"refreshInterval,omitempty"`
	FetchedAt  int64          `json:"fetchedAt"`
	Format     string         `json:"format,omitempty"`
	Info       *store.SubInfo `json:"info,omitempty"`
	Nodes      []*node.Node   `json:"nodes"`
}

// SyncStatus is the outcome of the last push.
type SyncStatus struct {
	At      int64  `json:"at"`
	Count   int    `json:"count"`             // results the receiver stored
	Sources int    `json:"sources,omitempty"` // sources whose nodes the receiver updated
	Error   string `json:"error,omitempty"`
}

const syncKeySetting = "sync_key"

// SyncKey returns the key other instances use to push results here, creating it on first use.
func (m *Manager) SyncKey() (string, error) {
	k, err := m.st.Setting(syncKeySetting)
	if err != nil || k != "" {
		return k, err
	}
	return m.RegenerateSyncKey()
}

// RegenerateSyncKey replaces the sync key; instances using the old one stop syncing.
func (m *Manager) RegenerateSyncKey() (string, error) {
	b := make([]byte, 24)
	rand.Read(b)
	k := base64.RawURLEncoding.EncodeToString(b)
	return k, m.st.SetSetting(syncKeySetting, k)
}

// Import stores results pushed by another instance. A result replaces the stored
// one only when it is newer. Returns how many were stored.
func (m *Manager) Import(results []Result) int {
	cur := m.Results()
	n := 0
	for _, r := range results {
		if r.Key == "" || r.TestedAt <= 0 {
			continue
		}
		if prev, ok := cur[r.Key]; ok && prev.TestedAt >= r.TestedAt {
			continue
		}
		r.Synced = true
		m.store(r)
		n++
	}
	return n
}

// Push sends this instance's own results (not ones it received) to the configured receiver.
func (m *Manager) Push(ctx context.Context) SyncStatus {
	st := SyncStatus{At: time.Now().Unix()}
	defer func() {
		m.mu.Lock()
		m.lastSync = &st
		m.mu.Unlock()
		if st.Error != "" {
			slog.Warn("push test results", "err", st.Error)
		}
	}()
	cfg := m.Settings().Sync
	if cfg.URL == "" || cfg.Key == "" {
		st.Error = "未设置同步地址或密钥"
		return st
	}
	var own []Result
	for _, r := range m.Results() {
		if !r.Synced {
			own = append(own, r)
		}
	}
	payload := map[string]any{"results": own}
	if !cfg.NoNodes {
		srcs, err := m.localSources()
		if err != nil {
			st.Error = err.Error()
			return st
		}
		payload["sources"] = srcs
	}
	body, err := json.Marshal(payload)
	if err != nil {
		st.Error = err.Error()
		return st
	}
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(cfg.URL, "/")+"/api/sync/results", bytes.NewReader(body))
	if err != nil {
		st.Error = err.Error()
		return st
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+cfg.Key)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		st.Error = shortErr(err)
		return st
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	var out struct {
		Imported int    `json:"imported"`
		Sources  int    `json:"sources"`
		Error    string `json:"error"`
	}
	json.Unmarshal(raw, &out)
	if resp.StatusCode != http.StatusOK {
		st.Error = fmt.Sprintf("HTTP %d %s", resp.StatusCode, out.Error)
		if resp.StatusCode == http.StatusUnauthorized {
			st.Error = "同步密钥不正确"
		}
		return st
	}
	st.Count, st.Sources = out.Imported, out.Sources
	return st
}

// localSources lists the enabled sources that have nodes, for pushing. Sources that
// were themselves received from elsewhere are not pushed on.
func (m *Manager) localSources() ([]SyncSource, error) {
	srcs, err := m.st.ListSources()
	if err != nil {
		return nil, err
	}
	var out []SyncSource
	for _, s := range srcs {
		if !s.Enabled || s.SyncedAt > 0 && s.SyncedAt >= s.FetchedAt {
			continue
		}
		nodes, err := m.st.GetNodes(s.ID)
		if err != nil {
			return nil, err
		}
		if len(nodes) == 0 {
			continue
		}
		for _, n := range nodes {
			n.Source, n.Probe = "", nil
		}
		out = append(out, SyncSource{Name: s.Name, Kind: s.Kind, URL: s.URL, UserAgent: s.UserAgent, Content: s.Content,
			SelfHosted: s.SelfHosted, Interval: s.RefreshInterval, FetchedAt: s.FetchedAt, Format: s.Format, Info: s.Info, Nodes: nodes})
	}
	return out, nil
}

// ImportSources stores pushed sources' nodes. A source is matched as described in
// matchSource and created when missing. Its nodes are replaced
// unless this instance fetched the source itself more recently and successfully.
// Returns how many sources were updated.
func (m *Manager) ImportSources(in []SyncSource) (int, error) {
	srcs, err := m.st.ListSources()
	if err != nil {
		return 0, err
	}
	n := 0
	for _, p := range in {
		if p.Name == "" || len(p.Nodes) == 0 || (p.Kind == "url" && p.URL == "") {
			continue
		}
		s := m.matchSource(srcs, p)
		if s == nil {
			s = &store.Source{ID: store.NewID(), Name: p.Name, Kind: p.Kind, URL: p.URL, UserAgent: p.UserAgent,
				Content: p.Content, SelfHosted: p.SelfHosted, RefreshInterval: p.Interval, Enabled: true}
			srcs = append(srcs, *s)
		} else if s.LastError == "" && s.FetchedAt > p.FetchedAt && s.SyncedAt < s.FetchedAt {
			continue // our own fetch is newer and worked
		}
		for _, nd := range p.Nodes {
			nd.Source = s.ID
		}
		if err := m.st.PutNodes(s.ID, p.Nodes); err != nil {
			return n, err
		}
		s.NodeCount, s.Format, s.SyncedAt = len(p.Nodes), p.Format, time.Now().Unix()
		if p.Info != nil {
			s.Info = p.Info
		}
		if s.Kind == "manual" && s.Content == "" {
			s.Content = p.Content
		}
		if err := m.st.PutSource(s); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

// matchSource finds the local source a pushed one stands for. The same subscription
// is often added with different query parameters (output format, flags) or names on
// each instance, so after an exact URL match it tries, in order: the same host and
// path (when only one source has them), most of the pushed nodes already being in a
// source, and the same name ignoring case.
func (m *Manager) matchSource(srcs []store.Source, p SyncSource) *store.Source {
	for i := range srcs {
		if p.Kind == "url" && srcs[i].Kind == "url" && srcs[i].URL == p.URL {
			return &srcs[i]
		}
	}
	if p.Kind == "url" {
		if hp := hostPath(p.URL); hp != "" {
			var found *store.Source
			n := 0
			for i := range srcs {
				if srcs[i].Kind == "url" && hostPath(srcs[i].URL) == hp {
					found = &srcs[i]
					n++
				}
			}
			if n == 1 {
				return found
			}
		}
	}
	keys := map[string]bool{}
	for _, nd := range p.Nodes {
		if !node.IsInfo(nd) {
			keys[nd.Key()] = true
		}
	}
	for i := range srcs {
		if srcs[i].Kind != p.Kind || len(keys) == 0 {
			continue
		}
		have, _ := m.st.GetNodes(srcs[i].ID)
		common := 0
		for _, nd := range have {
			if keys[nd.Key()] {
				common++
			}
		}
		if common*2 >= len(keys) {
			return &srcs[i]
		}
	}
	for i := range srcs {
		if srcs[i].Kind == p.Kind && strings.EqualFold(srcs[i].Name, p.Name) {
			return &srcs[i]
		}
	}
	return nil
}

// hostPath is the lower-case host and path of a URL, without query or fragment.
func hostPath(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return ""
	}
	return strings.ToLower(u.Host) + strings.TrimRight(u.Path, "/")
}

// autoPush pushes in the background when auto sync is on.
func (m *Manager) autoPush() {
	if s := m.Settings().Sync; s.Auto && s.URL != "" && s.Key != "" {
		go m.Push(context.Background())
	}
}

// ErrSyncKey is returned for a missing or wrong sync key.
var ErrSyncKey = errors.New("invalid sync key")
