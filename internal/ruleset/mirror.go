package ruleset

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/cusconf/cusconf/internal/store"
)

// Mirror keeps cached copies of remote rule lists and serves them converted.
// Lists are registered when a config that uses them is generated, so the mirror
// only ever fetches URLs the admin configured.
type Mirror struct {
	st     *store.Store
	Client *http.Client
	TTL    time.Duration // refresh a copy older than this when it is requested

	mu       sync.Mutex
	known    map[string]bool
	inflight map[string]*sync.WaitGroup
}

func NewMirror(st *store.Store) *Mirror {
	return &Mirror{st: st, Client: &http.Client{Timeout: 30 * time.Second}, TTL: 12 * time.Hour,
		known: map[string]bool{}, inflight: map[string]*sync.WaitGroup{}}
}

// ID is the stable mirror id of a list.
func ID(url, behavior string) string {
	h := sha256.Sum256([]byte(behavior + "|" + url))
	return hex.EncodeToString(h[:8])
}

// Register records a list and returns its id.
func (m *Mirror) Register(url, behavior string) string {
	if behavior == "" {
		behavior = "classical"
	}
	id := ID(url, behavior)
	m.mu.Lock()
	known := m.known[id]
	m.known[id] = true
	m.mu.Unlock()
	if !known {
		if err := m.st.RegisterRule(id, url, behavior); err != nil {
			slog.Warn("rule mirror register", "url", url, "err", err)
		}
	}
	return id
}

// Get returns the list, fetching it when there is no copy yet or the copy is older
// than TTL. When the upstream fails, the previous copy is served.
func (m *Mirror) Get(ctx context.Context, id string) (*store.RuleCache, error) {
	r, err := m.st.GetRule(id)
	if err != nil {
		return nil, err
	}
	m.st.TouchRule(id, time.Now().Unix())
	if r.Body != nil && time.Since(time.Unix(r.FetchedAt, 0)) < m.TTL {
		return r, nil
	}
	if ferr := m.Refresh(ctx, id); ferr != nil && r.Body == nil {
		return nil, ferr
	}
	return m.st.GetRule(id)
}

// Refresh fetches the upstream copy now. Concurrent calls for one id share a fetch.
func (m *Mirror) Refresh(ctx context.Context, id string) error {
	m.mu.Lock()
	if wg, ok := m.inflight[id]; ok {
		m.mu.Unlock()
		wg.Wait()
		r, err := m.st.GetRule(id)
		if err == nil && r.Error != "" {
			return errors.New(r.Error)
		}
		return err
	}
	wg := &sync.WaitGroup{}
	wg.Add(1)
	m.inflight[id] = wg
	m.mu.Unlock()
	defer func() {
		m.mu.Lock()
		delete(m.inflight, id)
		m.mu.Unlock()
		wg.Done()
	}()

	r, err := m.st.GetRule(id)
	if err != nil {
		return err
	}
	body, err := m.fetch(ctx, r.URL)
	if err != nil {
		m.st.SetRuleError(id, err.Error())
		return err
	}
	return m.st.PutRuleBody(id, body, time.Now().Unix())
}

func (m *Mirror) fetch(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "cusconf-rule-mirror")
	resp, err := m.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("upstream HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return nil, err
	}
	if len(body) == 0 {
		return nil, errors.New("upstream returned an empty list")
	}
	return body, nil
}

// Serve converts a list to format.
func Serve(r *store.RuleCache, format string) ([]byte, string) {
	out, _ := Format(Parse(r.Body, r.Behavior), r.Behavior, format)
	ct := "text/plain; charset=utf-8"
	switch format {
	case JSON:
		ct = "application/json"
	case YAML:
		ct = "text/yaml; charset=utf-8"
	}
	return out, ct
}

// RefreshLoop refreshes lists that clients used in the last week, every TTL.
func (m *Mirror) RefreshLoop(ctx context.Context) {
	t := time.NewTicker(time.Hour)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		rules, err := m.st.ListRules()
		if err != nil {
			continue
		}
		for _, r := range rules {
			if time.Since(time.Unix(r.UsedAt, 0)) > 7*24*time.Hour || time.Since(time.Unix(r.FetchedAt, 0)) < m.TTL {
				continue
			}
			if err := m.Refresh(ctx, r.ID); err != nil {
				slog.Warn("rule mirror refresh", "url", r.URL, "err", err)
			}
		}
	}
}

// Forget drops an id from the registration cache, so a later use registers it again.
func (m *Mirror) Forget(id string) {
	m.mu.Lock()
	delete(m.known, id)
	m.mu.Unlock()
}
