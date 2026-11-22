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
	"strings"
	"time"
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
}

// SyncStatus is the outcome of the last push.
type SyncStatus struct {
	At    int64  `json:"at"`
	Count int    `json:"count"`
	Error string `json:"error,omitempty"`
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
	body, err := json.Marshal(map[string]any{"results": own})
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
	st.Count = out.Imported
	return st
}

// autoPush pushes in the background when auto sync is on.
func (m *Manager) autoPush() {
	if s := m.Settings().Sync; s.Auto && s.URL != "" && s.Key != "" {
		go m.Push(context.Background())
	}
}

// ErrSyncKey is returned for a missing or wrong sync key.
var ErrSyncKey = errors.New("invalid sync key")
