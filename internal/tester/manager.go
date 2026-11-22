package tester

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"slices"
	"sync"
	"time"

	"github.com/cusconf/cusconf/internal/node"
	"github.com/cusconf/cusconf/internal/store"
)

const settingsKey = "test_settings"

// Job is the progress of a test run. Only one runs at a time.
type Job struct {
	Running    bool   `json:"running"`
	Total      int    `json:"total"`
	Done       int    `json:"done"`
	Reachable  int    `json:"reachable"`
	Checks     Checks `json:"checks"`
	StartedAt  int64  `json:"startedAt,omitempty"`
	FinishedAt int64  `json:"finishedAt,omitempty"`
	Scheduled  bool   `json:"scheduled,omitempty"`
	Error      string `json:"error,omitempty"` // the job could not run (e.g. mihomo missing)
	// Sync is the outcome of the last push to another cusconf, if any.
	Sync *SyncStatus `json:"sync,omitempty"`
}

type Manager struct {
	st *store.Store

	mu       sync.Mutex
	job      Job
	cancel   context.CancelFunc
	results  map[string]Result // cache of stored results
	lastSync *SyncStatus
}

func NewManager(st *store.Store) *Manager { return &Manager{st: st} }

var (
	ErrBusy    = errors.New("已有测试在运行")
	ErrNoNodes = errors.New("没有可测试的节点（机场信息节点不测试；请检查测试来源设置）")
)

// Testable drops airport info pseudo-nodes and nodes outside the configured sources,
// and keeps one node per endpoint key.
func Testable(nodes []*node.Node, s Settings) []*node.Node {
	seen := map[string]bool{}
	var out []*node.Node
	for _, n := range nodes {
		if node.IsInfo(n) || (len(s.Sources) > 0 && !slices.Contains(s.Sources, n.Source)) {
			continue
		}
		if k := n.Key(); !seen[k] {
			seen[k] = true
			out = append(out, n)
		}
	}
	return out
}

func (m *Manager) Settings() Settings {
	raw, err := m.st.Setting(settingsKey)
	if err != nil || raw == "" {
		return DefaultSettings()
	}
	// Decode into an empty value: missing fields (e.g. knownServices in older
	// settings) must stay empty so normalize can tell what is new.
	var s Settings
	if json.Unmarshal([]byte(raw), &s) != nil {
		return DefaultSettings()
	}
	s.normalize()
	return s
}

func (m *Manager) SaveSettings(s Settings) error {
	s.normalize()
	b, err := json.Marshal(s)
	if err != nil {
		return err
	}
	return m.st.SetSetting(settingsKey, string(b))
}

// Results returns the latest result per node key.
func (m *Manager) Results() map[string]Result {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.results == nil {
		m.results = map[string]Result{}
		raw, err := m.st.Tests()
		if err != nil {
			slog.Error("load test results", "err", err)
		}
		for k, b := range raw {
			var r Result
			if json.Unmarshal(b, &r) == nil {
				r.upgrade()
				m.results[k] = r
			}
		}
	}
	out := make(map[string]Result, len(m.results))
	for k, v := range m.results {
		out[k] = v
	}
	return out
}

// Attach sets Probe on nodes that have a test result.
func (m *Manager) Attach(nodes []*node.Node) {
	res := m.Results()
	for _, n := range nodes {
		if r, ok := res[n.Key()]; ok {
			n.Probe = r.Probe()
		}
	}
}

func (m *Manager) Status() Job {
	m.mu.Lock()
	defer m.mu.Unlock()
	j := m.job
	j.Sync = m.lastSync
	return j
}

// Stop cancels the running job; finished nodes keep their results.
func (m *Manager) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.cancel != nil {
		m.cancel()
	}
}

// Start tests the given nodes in the background.
func (m *Manager) Start(nodes []*node.Node, c Checks, scheduled bool) (Job, error) {
	m.Results() // make sure the cache is loaded before workers write to it
	m.mu.Lock()
	if m.job.Running {
		m.mu.Unlock()
		return Job{}, ErrBusy
	}
	todo := Testable(nodes, m.Settings())
	if len(todo) == 0 {
		m.mu.Unlock()
		return Job{}, ErrNoNodes
	}
	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	m.job = Job{Running: true, Total: len(todo), Checks: c, StartedAt: time.Now().Unix(), Scheduled: scheduled}
	job := m.job
	m.mu.Unlock()

	go m.run(ctx, todo, c)
	return job, nil
}

// TestOne tests a single node right away (independently of any running job),
// stores and returns the result.
func (m *Manager) TestOne(ctx context.Context, n *node.Node, c Checks) Result {
	m.Results() // load the cache
	core, err := StartCore(ctx, []*node.Node{n})
	if err != nil {
		// Not stored: the failure is about this machine, not the node.
		return Result{Key: n.Key(), Name: n.Name, TestedAt: time.Now().Unix(), Error: err.Error()}
	}
	defer core.Close()
	r := Test(ctx, core, n, m.Settings(), c)
	r = m.save(r, c)
	m.autoPush()
	return r
}

// save merges a result with the previous one, caches and persists it.
// A failed local test does not replace a working result synced from another
// instance: that instance can reach the node, this one cannot (see sync.go).
func (m *Manager) save(r Result, c Checks) Result {
	m.mu.Lock()
	if prev, had := m.results[r.Key]; had {
		if prev.Synced && prev.Latency > 0 && r.Latency == 0 {
			m.mu.Unlock()
			return prev
		}
		r.Merge(&prev, c)
	}
	m.mu.Unlock()
	m.store(r)
	return r
}

// store caches and persists a result as is.
func (m *Manager) store(r Result) {
	m.Results() // make sure the cache is loaded
	m.mu.Lock()
	m.results[r.Key] = r
	m.mu.Unlock()
	if b, err := json.Marshal(r); err == nil {
		if err := m.st.PutTest(r.Key, b, r.TestedAt); err != nil {
			slog.Error("store test result", "err", err)
		}
	}
}

func (m *Manager) run(ctx context.Context, nodes []*node.Node, c Checks) {
	s := m.Settings()
	core, err := StartCore(ctx, nodes)
	if err != nil {
		slog.Error("node test: start mihomo", "err", err)
		m.mu.Lock()
		m.job.Running, m.job.Error = false, err.Error()
		m.job.FinishedAt = time.Now().Unix()
		m.cancel = nil
		m.mu.Unlock()
		return
	}
	defer core.Close()
	work := make(chan *node.Node)
	var wg sync.WaitGroup
	for range s.Concurrency {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for n := range work {
				r := Test(ctx, core, n, s, c)
				if ctx.Err() != nil {
					return // cancelled mid-test: do not store a bogus failure
				}
				r = m.save(r, c)
				m.mu.Lock()
				m.job.Done++
				if r.Latency > 0 {
					m.job.Reachable++
				}
				m.mu.Unlock()
			}
		}()
	}
	for _, n := range nodes {
		select {
		case work <- n:
		case <-ctx.Done():
		}
		if ctx.Err() != nil {
			break
		}
	}
	close(work)
	wg.Wait()
	m.mu.Lock()
	m.job.Running = false
	m.job.FinishedAt = time.Now().Unix()
	m.cancel = nil
	slog.Info("node test finished", "total", m.job.Total, "done", m.job.Done, "reachable", m.job.Reachable)
	m.mu.Unlock()
	m.autoPush()
}

const lastScheduledKey = "test_last_scheduled"

// RunSchedule starts a full test every ScheduleHours. nodes supplies the current node list.
func (m *Manager) RunSchedule(ctx context.Context, nodes func() ([]*node.Node, error)) {
	tick := time.NewTicker(time.Minute)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			m.Stop()
			return
		case <-tick.C:
		}
		s := m.Settings()
		if s.ScheduleHours <= 0 {
			continue
		}
		last, _ := m.st.Setting(lastScheduledKey)
		var lastAt int64
		_ = json.Unmarshal([]byte(last), &lastAt)
		if time.Since(time.Unix(lastAt, 0)) < time.Duration(s.ScheduleHours)*time.Hour {
			continue
		}
		ns, err := nodes()
		if err != nil || len(ns) == 0 {
			continue
		}
		if _, err := m.Start(ns, Checks{IP: true, Services: true}, true); err == nil {
			b, _ := json.Marshal(time.Now().Unix())
			_ = m.st.SetSetting(lastScheduledKey, string(b))
			slog.Info("scheduled node test started", "nodes", len(ns))
		}
	}
}
