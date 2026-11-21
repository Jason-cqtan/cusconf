// Package source fetches subscriptions, parses them and keeps the node cache fresh.
package source

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/cusconf/cusconf/internal/parser"
	"github.com/cusconf/cusconf/internal/store"
)

// DefaultUA makes most airports return a full Clash.Meta YAML, which carries
// the richest node fields (REALITY, hysteria2, tuic...).
const DefaultUA = "clash.meta"

type Manager struct {
	Store  *store.Store
	Client *http.Client
	mu     sync.Mutex // serializes refreshes
}

func NewManager(st *store.Store) *Manager {
	return &Manager{Store: st, Client: &http.Client{Timeout: 30 * time.Second}}
}

// Fetch downloads a subscription body and its subscription-userinfo header.
func (m *Manager) Fetch(ctx context.Context, url, ua string) ([]byte, *store.SubInfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, nil, err
	}
	if ua == "" {
		ua = DefaultUA
	}
	req.Header.Set("User-Agent", ua)
	resp, err := m.Client.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 20<<20))
	if err != nil {
		return nil, nil, err
	}
	return body, ParseUserinfo(resp.Header.Get("Subscription-Userinfo")), nil
}

// ParseUserinfo parses "upload=1; download=2; total=3; expire=4".
func ParseUserinfo(h string) *store.SubInfo {
	if h == "" {
		return nil
	}
	info := &store.SubInfo{}
	for _, part := range strings.Split(h, ";") {
		k, v, ok := strings.Cut(strings.TrimSpace(part), "=")
		if !ok {
			continue
		}
		n, _ := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
		switch strings.ToLower(k) {
		case "upload":
			info.Upload = n
		case "download":
			info.Download = n
		case "total":
			info.Total = n
		case "expire":
			info.Expire = n
		}
	}
	return info
}

// Refresh re-reads a source and updates its node cache and status.
func (m *Manager) Refresh(ctx context.Context, src *store.Source) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	var (
		body []byte
		info *store.SubInfo
		err  error
	)
	switch src.Kind {
	case "manual":
		body = []byte(src.Content)
	default:
		body, info, err = m.Fetch(ctx, src.URL, src.UserAgent)
	}
	src.FetchedAt = time.Now().Unix()
	if err != nil {
		src.LastError = err.Error()
		m.Store.PutSource(src)
		return err
	}
	res := parser.Parse(body)
	src.LastError = ""
	src.Format = string(res.Format)
	src.Info = info
	src.Warnings = nil
	for i, e := range res.Errors {
		if i >= 50 {
			src.Warnings = append(src.Warnings, fmt.Sprintf("... %d more", len(res.Errors)-50))
			break
		}
		src.Warnings = append(src.Warnings, e.Error())
	}
	if len(res.Nodes) == 0 && src.Kind != "manual" {
		// Keep the previous cache: an empty response is more likely an airport hiccup.
		src.LastError = "no nodes parsed"
		m.Store.PutSource(src)
		return fmt.Errorf("no nodes parsed")
	}
	for _, n := range res.Nodes {
		n.Source = src.ID
	}
	src.NodeCount = len(res.Nodes)
	if err := m.Store.PutNodes(src.ID, res.Nodes); err != nil {
		return err
	}
	return m.Store.PutSource(src)
}

// Run refreshes due sources until ctx is done.
func (m *Manager) Run(ctx context.Context) {
	tick := time.NewTicker(time.Minute)
	defer tick.Stop()
	for {
		m.refreshDue(ctx)
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}

func (m *Manager) refreshDue(ctx context.Context) {
	srcs, err := m.Store.ListSources()
	if err != nil {
		slog.Error("list sources", "err", err)
		return
	}
	now := time.Now().Unix()
	for i := range srcs {
		s := &srcs[i]
		if !s.Enabled || s.Kind != "url" || s.RefreshInterval <= 0 || now-s.FetchedAt < int64(s.RefreshInterval) {
			continue
		}
		if err := m.Refresh(ctx, s); err != nil {
			slog.Warn("refresh source", "source", s.Name, "err", err)
		} else {
			slog.Info("refreshed source", "source", s.Name, "nodes", s.NodeCount)
		}
	}
}
