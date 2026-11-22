// Package tester probes nodes through an embedded mihomo core: latency, exit IP
// (with configurable risk-score sources) and availability of streaming / AI services.
package tester

import (
	"context"
	"net"
	"net/netip"
	"slices"
	"sync"
	"time"

	"github.com/cusconf/cusconf/internal/node"
)

type Settings struct {
	Concurrency   int        `json:"concurrency"`   // nodes tested in parallel
	TimeoutSec    int        `json:"timeoutSec"`    // per request
	LatencyURL    string     `json:"latencyUrl"`    // url-test style endpoint
	IPSources     []IPSource `json:"ipSources"`     // exit IP / safety score sources
	Services      []string   `json:"services"`      // enabled service check IDs
	ScheduleHours int        `json:"scheduleHours"` // automatic full test interval; 0 = off
	Sources       []string   `json:"sources"`       // source IDs to test (manual and scheduled); empty = all
	// KnownServices are the service IDs that existed when Services was last saved, so
	// checks added later are switched on by default instead of looking disabled.
	KnownServices []string `json:"knownServices"`
	// KnownIPSources are the default IP source names that existed when settings were
	// saved, so defaults added later get appended (a removed default stays removed).
	KnownIPSources []string `json:"knownIpSources"`
}

// ipSourcesBeforeKnown are the default IP sources that shipped before KnownIPSources was tracked.
var ipSourcesBeforeKnown = []string{"ip-api.com", "proxycheck.io"}

// servicesBeforeKnown are the checks that shipped before KnownServices was tracked.
var servicesBeforeKnown = []string{"netflix", "disney", "youtube", "chatgpt", "claude", "gemini", "tiktok"}

func DefaultSettings() Settings {
	ids := make([]string, len(Services))
	for i, s := range Services {
		ids[i] = s.ID
	}
	var srcNames []string
	for _, src := range DefaultIPSources() {
		srcNames = append(srcNames, src.Name)
	}
	return Settings{
		KnownServices:  slices.Clone(ids),
		KnownIPSources: srcNames,
		Concurrency:    8, TimeoutSec: 8,
		LatencyURL: "https://cp.cloudflare.com/generate_204",
		IPSources:  DefaultIPSources(),
		Services:   ids,
	}
}

func (s *Settings) normalize() {
	d := DefaultSettings()
	if s.Concurrency <= 0 {
		s.Concurrency = d.Concurrency
	}
	s.Concurrency = min(s.Concurrency, 64)
	if s.TimeoutSec <= 0 {
		s.TimeoutSec = d.TimeoutSec
	}
	// Empty, or the previous built-in default: use the current default.
	if s.LatencyURL == "" || s.LatencyURL == "http://www.gstatic.com/generate_204" {
		s.LatencyURL = d.LatencyURL
	}
	known := s.KnownServices
	if known == nil {
		known = servicesBeforeKnown
	}
	for _, svc := range Services {
		if !slices.Contains(known, svc.ID) && !slices.Contains(s.Services, svc.ID) {
			s.Services = append(s.Services, svc.ID)
		}
	}
	s.KnownServices = s.KnownServices[:0]
	for _, svc := range Services {
		s.KnownServices = append(s.KnownServices, svc.ID)
	}
	knownSrc := s.KnownIPSources
	if knownSrc == nil {
		knownSrc = ipSourcesBeforeKnown
	}
	s.KnownIPSources = nil
	for _, def := range DefaultIPSources() {
		s.KnownIPSources = append(s.KnownIPSources, def.Name)
		if slices.Contains(knownSrc, def.Name) || slices.ContainsFunc(s.IPSources, func(x IPSource) bool { return x.Name == def.Name }) {
			continue
		}
		s.IPSources = append(s.IPSources, def)
	}
	defaults := DefaultIPSources()
	for i := range s.IPSources {
		src := &s.IPSources[i]
		// What a built-in source's number means is fixed by that site, not configurable.
		for _, d := range defaults {
			if d.Name == src.Name {
				src.HigherIsSafer = d.HigherIsSafer
			}
		}
		if src.URL == oldProxycheckURL { // old v2 default: its risk model differs from the website's
			enabled := src.Enabled
			*src = proxycheckV3()
			src.Enabled = enabled
			continue
		}
		if src.Score == "" && src.LegacyRisk != "" { // risk-based settings from before
			src.Score, src.ScoreScale = src.LegacyRisk, src.LegacyRiskScale
		}
		src.LegacyRisk, src.LegacyRiskScale = "", 0
	}
}

// Checks selects what to test.
type Checks struct {
	IP       bool `json:"ip"`
	Services bool `json:"services"`
}

// Exit summarizes the node's exit IP across all sources.
type Exit struct {
	IP      string `json:"ip,omitempty"`
	Country string `json:"country,omitempty"`
	City    string `json:"city,omitempty"`
	ISP     string `json:"isp,omitempty"`
	ASN     string `json:"asn,omitempty"`
	// Scores holds each source's safety score (0–100, higher is safer), by source name.
	// Sources are not combined: each site rates differently.
	Scores map[string]float64 `json:"scores,omitempty"`
	Flags  []string           `json:"flags,omitempty"`

	// Fields of results stored by earlier versions; dropped by upgrade.
	LegacyRisk  *float64 `json:"risk,omitempty"`
	LegacyScore *float64 `json:"score,omitempty"`
}

// upgrade converts results stored by earlier versions: risk scores (higher = worse)
// become safety scores, and the per-source scores are rebuilt from the reports.
func (r *Result) upgrade() {
	for i := range r.Reports {
		if rep := &r.Reports[i]; rep.Score == nil && rep.LegacyRisk != nil {
			v := max(0, min(100, 100-*rep.LegacyRisk))
			rep.Score, rep.LegacyRisk = &v, nil
		}
	}
	if e := r.Exit; e != nil {
		e.LegacyRisk, e.LegacyScore = nil, nil
		if e.Scores == nil {
			e.Scores = scoresOf(r.Reports)
		}
	}
}

func scoresOf(reports []IPReport) map[string]float64 {
	var m map[string]float64
	for _, rep := range reports {
		if rep.Score != nil && rep.Error == "" {
			if m == nil {
				m = map[string]float64{}
			}
			m[rep.Source] = *rep.Score
		}
	}
	return m
}

type Result struct {
	Key      string                   `json:"key"`
	Name     string                   `json:"name"`
	TestedAt int64                    `json:"testedAt"`
	Latency  int                      `json:"latency"` // ms; 0 when unreachable
	Error    string                   `json:"error,omitempty"`
	Warning  string                   `json:"warning,omitempty"` // e.g. traffic intercepted by a local proxy
	Exit     *Exit                    `json:"exit,omitempty"`
	Reports  []IPReport               `json:"reports,omitempty"`
	Services map[string]ServiceResult `json:"services,omitempty"`
}

// Merge keeps earlier IP / service data when a newer run skipped those checks.
func (r *Result) Merge(prev *Result, c Checks) {
	if prev == nil || r.Latency == 0 {
		return
	}
	if !c.IP {
		r.Exit, r.Reports = prev.Exit, prev.Reports
	}
	if !c.Services {
		r.Services = prev.Services
	}
}

// Probe converts a result into the summary attached to nodes for filtering.
func (r *Result) Probe() *node.Probe {
	p := &node.Probe{Latency: r.Latency, TestedAt: r.TestedAt}
	if r.Exit != nil {
		p.ExitCountry, p.Scores, p.Flags = r.Exit.Country, r.Exit.Scores, r.Exit.Flags
	}
	for id, s := range r.Services {
		switch s.Status {
		case Available:
			p.Unlocked = append(p.Unlocked, id)
		case Partial:
			p.Partial = append(p.Partial, id)
		}
	}
	slices.Sort(p.Unlocked)
	slices.Sort(p.Partial)
	return p
}

// Test probes one node.
func Test(ctx context.Context, n *node.Node, s Settings, c Checks) Result {
	s.normalize()
	r := Result{Key: n.Key(), Name: n.Name, TestedAt: time.Now().Unix()}
	timeout := time.Duration(s.TimeoutSec) * time.Second
	r.Warning = interceptWarning(ctx, n.Server)
	p, err := NewProxy(n)
	if err != nil {
		r.Error = err.Error()
		return r
	}
	defer p.Close()

	// One retry: a single dropped handshake (EOF / reset) is common and not worth flagging a node for.
	var lat time.Duration
	for attempt := 0; attempt < 2; attempt++ {
		lctx, cancel := context.WithTimeout(ctx, 2*timeout)
		lat, err = p.Latency(lctx, s.LatencyURL, timeout)
		cancel()
		if err == nil || ctx.Err() != nil {
			break
		}
	}
	if err != nil {
		r.Error = shortErr(err)
		return r
	}
	r.Latency = max(int(lat.Milliseconds()), 1)

	var wg sync.WaitGroup
	if c.IP {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r.Reports, r.Exit = queryIP(ctx, p, s, timeout)
		}()
	}
	if c.Services && len(s.Services) > 0 {
		r.Services = map[string]ServiceResult{}
		var mu sync.Mutex
		e := &env{c: p.Client(timeout, true)}
		for _, id := range s.Services {
			svc, ok := ServiceByID(id)
			if !ok {
				continue
			}
			wg.Add(1)
			go func() {
				defer wg.Done()
				sctx, cancel := context.WithTimeout(ctx, 3*timeout)
				defer cancel()
				res := svc.check(sctx, e)
				mu.Lock()
				r.Services[svc.ID] = res
				mu.Unlock()
			}()
		}
	}
	wg.Wait()
	return r
}

func queryIP(ctx context.Context, p *Proxy, s Settings, timeout time.Duration) ([]IPReport, *Exit) {
	client := p.Client(timeout, true)
	defer client.CloseIdleConnections()
	var reports []IPReport
	var direct []IPSource
	// Sources that go through the node run first and tell us the exit IP.
	for _, src := range s.IPSources {
		if !src.Enabled {
			continue
		}
		if containsIPPlaceholder(src.URL) {
			direct = append(direct, src)
			continue
		}
		qctx, cancel := context.WithTimeout(ctx, 2*timeout)
		reports = append(reports, src.Query(qctx, client, "", timeout))
		cancel()
	}
	exit := &Exit{}
	merge := func(rep IPReport) {
		if rep.Error != "" {
			return
		}
		exit.IP = or(exit.IP, rep.IP)
		exit.Country = or(exit.Country, rep.Country)
		exit.City = or(exit.City, rep.City)
		exit.ISP = or(exit.ISP, rep.ISP)
		exit.ASN = or(exit.ASN, rep.ASN)
		if rep.Score != nil {
			if exit.Scores == nil {
				exit.Scores = map[string]float64{}
			}
			exit.Scores[rep.Source] = *rep.Score
		}
		for _, f := range rep.Flags {
			if !slices.Contains(exit.Flags, f) {
				exit.Flags = append(exit.Flags, f)
			}
		}
	}
	for _, rep := range reports {
		merge(rep)
	}
	for _, src := range direct {
		qctx, cancel := context.WithTimeout(ctx, 2*timeout)
		rep := src.Query(qctx, nil, exit.IP, timeout)
		cancel()
		reports = append(reports, rep)
		merge(rep)
	}
	slices.Sort(exit.Flags)
	if exit.IP == "" && exit.Country == "" {
		return reports, nil
	}
	return reports, exit
}

func containsIPPlaceholder(u string) bool {
	for i := 0; i+4 <= len(u); i++ {
		if u[i:i+4] == "{ip}" {
			return true
		}
	}
	return false
}

// fakeIPRange is used by Surge / Clash / Stash "fake IP" DNS in enhanced / TUN mode.
var fakeIPRange = netip.MustParsePrefix("198.18.0.0/15")

// interceptWarning detects when cusconf itself runs behind a local proxy app that
// resolves names to fake IPs: connections to the node then follow that app's rules
// (possibly through another proxy), so results do not reflect the node alone.
func interceptWarning(ctx context.Context, host string) string {
	if _, err := netip.ParseAddr(host); err == nil || host == "" {
		return ""
	}
	lctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	addrs, err := net.DefaultResolver.LookupNetIP(lctx, "ip4", host)
	if err != nil {
		return ""
	}
	for _, a := range addrs {
		if fakeIPRange.Contains(a.Unmap()) {
			return "节点服务器被解析为 " + a.String() + "（本机代理软件的 Fake IP），连接会按该软件的规则转发，可能被套进其它代理导致超时或结果不准。请在 Surge / Clash 中让 cusconf 进程或该域名直连。"
		}
	}
	return ""
}
