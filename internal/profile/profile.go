// Package profile is the user-defined, client-agnostic configuration:
// which nodes, how they are grouped, and how traffic is routed.
package profile

import (
	"fmt"
	"net/netip"
	"regexp"
	"slices"
	"strings"
	"sync"

	"github.com/cusconf/cusconf/internal/node"
)

type Profile struct {
	ID       string    `json:"id"`
	Name     string    `json:"name"`
	Sources  []string  `json:"sources"` // source IDs to pull nodes from; empty = all
	Filter   Filter    `json:"filter"`  // global node filter applied first
	Rename   []Rename  `json:"rename,omitempty"`
	Groups   []Group   `json:"groups"`
	Rules    []Rule    `json:"rules"`
	RuleSets []RuleSet `json:"ruleSets"`
	General  General   `json:"general"`
}

// Filter selects nodes. All non-empty criteria must match.
type Filter struct {
	Include string   `json:"include,omitempty"` // regex on name
	Exclude string   `json:"exclude,omitempty"` // regex on name
	Regions []string `json:"regions,omitempty"` // region codes, e.g. HK, JP
	// ExcludeRegions drops nodes of these regions; nodes without a known region are kept.
	// Region conditions (Regions / ExcludeRegions) never match self-hosted nodes.
	ExcludeRegions []string `json:"excludeRegions,omitempty"`
	// NodeKind limits to airport ("airport") or self-hosted ("selfhosted") nodes.
	NodeKind string   `json:"nodeKind,omitempty"`
	Types    []string `json:"types,omitempty"`   // node types
	Sources  []string `json:"sources,omitempty"` // source IDs
	Names    []string `json:"names,omitempty"`   // explicit node names (OR'ed with the rest when set alone)

	// Conditions on the latest node test. A node that was never tested does not match them.
	Reachable  bool `json:"reachable,omitempty"`  // last test succeeded
	MaxLatency int  `json:"maxLatency,omitempty"` // ms
	// MinScores requires a minimum safety score (0–100, higher is safer) from each named source.
	MinScores   map[string]float64 `json:"minScores,omitempty"`
	ExitRegions []string           `json:"exitRegions,omitempty"` // exit IP country codes
	Unlocks     []string           `json:"unlocks,omitempty"`     // service IDs that must be fully available
	// ExcludeFlags drops nodes whose exit IP carries any of these labels (e.g. 滥用).
	ExcludeFlags []string `json:"excludeFlags,omitempty"`
}

func (f *Filter) hasProbeConditions() bool {
	return f.Reachable || f.MaxLatency > 0 || len(f.MinScores) > 0 || len(f.ExitRegions) > 0 || len(f.Unlocks) > 0 || len(f.ExcludeFlags) > 0
}

func (f *Filter) matchProbe(p *node.Probe) bool {
	if p == nil {
		return false
	}
	if (f.Reachable || f.MaxLatency > 0) && p.Latency <= 0 {
		return false
	}
	if f.MaxLatency > 0 && p.Latency > f.MaxLatency {
		return false
	}
	for src, minimum := range f.MinScores {
		if v, ok := p.Scores[src]; !ok || v < minimum {
			return false
		}
	}
	if len(f.ExitRegions) > 0 && !slices.Contains(f.ExitRegions, p.ExitCountry) {
		return false
	}
	for _, s := range f.Unlocks {
		if !slices.Contains(p.Unlocked, s) {
			return false
		}
	}
	for _, fl := range f.ExcludeFlags {
		if slices.Contains(p.Flags, fl) {
			return false
		}
	}
	return true
}

func (f *Filter) Empty() bool {
	return f == nil || (f.Include == "" && f.Exclude == "" && len(f.Regions) == 0 && len(f.ExcludeRegions) == 0 && f.NodeKind == "" && len(f.Types) == 0 && len(f.Sources) == 0 && len(f.Names) == 0 && !f.hasProbeConditions())
}

// Rename applies a regex replacement to node names.
type Rename struct {
	Pattern string `json:"pattern"`
	Replace string `json:"replace"`
}

type GroupType string

const (
	Select      GroupType = "select"
	URLTest     GroupType = "url-test"
	Fallback    GroupType = "fallback"
	LoadBalance GroupType = "load-balance"
	Smart       GroupType = "smart"
)

type Group struct {
	Name string    `json:"name"`
	Type GroupType `json:"type"`
	Icon string    `json:"icon,omitempty"`
	// Members are other group names, DIRECT/REJECT, or node names, in order.
	Members []string `json:"members,omitempty"`
	// Nodes dynamically appends nodes matching the filter after Members.
	Nodes *Filter `json:"nodes,omitempty"`
	// MemberFilter, when set, replaces each group in Members by the nodes it reaches
	// (through nested groups too) that match the filter, e.g. "新加坡 nodes that unlock
	// Claude". Clients cannot filter inside a nested group, so the nodes are listed directly.
	MemberFilter *Filter `json:"memberFilter,omitempty"`
	// Order optionally fixes the order of the final member list (members + matched nodes).
	// Listed names come first in this order; anything not listed (e.g. new nodes) follows
	// in its default position. Names that no longer exist are ignored.
	Order []string `json:"order,omitempty"`

	URL        string `json:"url,omitempty"`
	Interval   int    `json:"interval,omitempty"`
	Tolerance  int    `json:"tolerance,omitempty"`
	Timeout    int    `json:"timeout,omitempty"`
	Persistent bool   `json:"persistent,omitempty"` // load-balance sticky
	Hidden     bool   `json:"hidden,omitempty"`
	Category   string `json:"category,omitempty"`
}

type Rule struct {
	Type      string `json:"type"`  // RULE-SET, DOMAIN, DOMAIN-SUFFIX, DOMAIN-KEYWORD, IP-CIDR, IP-CIDR6, GEOIP, PROCESS-NAME, FINAL
	Value     string `json:"value"` // RULE-SET: rule set name
	Policy    string `json:"policy"`
	NoResolve bool   `json:"noResolve,omitempty"`
	// Comment is written above the rule in the generated config (one "#" line per line).
	Comment string `json:"comment,omitempty"`
}

// CommentLines splits a rule comment into trimmed, non-empty lines.
func (r Rule) CommentLines() []string {
	var out []string
	for _, l := range strings.Split(r.Comment, "\n") {
		if l = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(l), "#")); l != "" {
			out = append(out, l)
		}
	}
	return out
}

type RuleSet struct {
	Name     string `json:"name"`
	Behavior string `json:"behavior"` // classical | domain | ipcidr
	// URLs per client. Surge expects .list (classical) or domain-set files;
	// Stash expects rule-provider files in StashFormat.
	SurgeURL    string `json:"surgeUrl,omitempty"`
	StashURL    string `json:"stashUrl,omitempty"`
	StashFormat string `json:"stashFormat,omitempty"` // yaml | text
	Interval    int    `json:"interval,omitempty"`    // seconds
}

type General struct {
	TestURL         string            `json:"testUrl,omitempty"`
	InternetTestURL string            `json:"internetTestUrl,omitempty"`
	DNS             []string          `json:"dns,omitempty"`
	IPv6            bool              `json:"ipv6,omitempty"`
	SkipProxy       []string          `json:"skipProxy,omitempty"`
	UpdateInterval  int               `json:"updateInterval,omitempty"` // managed config refresh, seconds
	Extra           map[string]string `json:"extra,omitempty"`          // raw text per client appended to [General] / top-level
	Hosts           []Host            `json:"hosts,omitempty"`
	// DisableServerDirect turns off the DIRECT rules added in front of all rules for
	// node servers and subscription hosts (on by default).
	DisableServerDirect bool `json:"disableServerDirect,omitempty"`
	// ServerDNS, when set (e.g. a domestic DoH URL), resolves every node server and
	// subscription domain with it via generated [Host] entries, and its own host gets a
	// DIRECT rule. This keeps "DoH over the proxy needs a node, which needs DNS" from looping.
	ServerDNS string `json:"serverDns,omitempty"`
}

// Host is one local DNS mapping, in Surge [Host] terms:
//
//	example.com = 1.2.3.4            fixed IP
//	example.com = other.example.com  alias
//	*.corp.com  = server:10.0.0.53   resolve with a specific DNS server
//	example.com = server:system      resolve with the system DNS
type Host struct {
	Domain string `json:"domain"`
	Value  string `json:"value"`
	// Comment is written above the entry in the generated Surge config.
	Comment string `json:"comment,omitempty"`
}

// HostKind classifies a Host value: "ip" (one or more comma-separated addresses),
// "server" or "alias".
func (h Host) Kind() string {
	if strings.HasPrefix(h.Value, "server:") {
		return "server"
	}
	if len(h.IPs()) > 0 {
		return "ip"
	}
	return "alias"
}

// IPs returns the addresses of an "ip" value ("1.2.3.4, 5.6.7.8"), or nil if any part is not an IP.
func (h Host) IPs() []string {
	var out []string
	for _, p := range strings.Split(h.Value, ",") {
		a, err := netip.ParseAddr(strings.TrimSpace(p))
		if err != nil {
			return nil
		}
		out = append(out, a.String())
	}
	return out
}

var builtins = []string{"DIRECT", "REJECT"}

func IsBuiltin(s string) bool { return slices.Contains(builtins, strings.ToUpper(s)) }

// Validate checks names, references and regexes.
func (p *Profile) Validate() error {
	groups := map[string]bool{}
	for _, g := range p.Groups {
		if g.Name == "" || IsBuiltin(g.Name) {
			return fmt.Errorf("invalid group name %q", g.Name)
		}
		if groups[g.Name] {
			return fmt.Errorf("duplicate group %q", g.Name)
		}
		groups[g.Name] = true
		switch g.Type {
		case Select, URLTest, Fallback, LoadBalance, Smart:
		default:
			return fmt.Errorf("group %q: unknown type %q", g.Name, g.Type)
		}
		if err := g.Nodes.compile(); err != nil {
			return fmt.Errorf("group %q: %w", g.Name, err)
		}
		if err := g.MemberFilter.compile(); err != nil {
			return fmt.Errorf("group %q: %w", g.Name, err)
		}
	}
	if err := p.Filter.compile(); err != nil {
		return err
	}
	for _, r := range p.Rename {
		if _, err := regexp.Compile(r.Pattern); err != nil {
			return fmt.Errorf("rename %q: %w", r.Pattern, err)
		}
	}
	sets := map[string]bool{}
	for _, rs := range p.RuleSets {
		sets[rs.Name] = true
	}
	if s := p.General.ServerDNS; s != "" && strings.ContainsAny(s, " ,=") {
		return fmt.Errorf("server DNS %q: must be a single address or DoH URL", s)
	}
	for i, h := range p.General.Hosts {
		if strings.TrimSpace(h.Domain) == "" || strings.TrimSpace(h.Value) == "" {
			return fmt.Errorf("host %d: domain and value are required", i+1)
		}
		if strings.ContainsAny(h.Domain, " ,=") {
			return fmt.Errorf("host %d (%s): domain must not contain spaces, commas or '='", i+1, h.Domain)
		}
		// Several values are allowed as a comma-separated list: IPs, or servers after "server:".
		for _, part := range strings.Split(strings.TrimPrefix(h.Value, "server:"), ",") {
			if p := strings.TrimSpace(part); p == "" || strings.ContainsAny(p, " =") {
				return fmt.Errorf("host %d (%s): bad value %q", i+1, h.Domain, h.Value)
			}
		}
		if strings.Contains(h.Value, ",") && h.Kind() == "alias" {
			return fmt.Errorf("host %d (%s): a list must be IP addresses or server:a,b", i+1, h.Domain)
		}
	}
	for i, r := range p.Rules {
		if r.Policy != "" && !IsBuiltin(r.Policy) && !groups[r.Policy] {
			return fmt.Errorf("rule %d: unknown policy %q", i+1, r.Policy)
		}
		if r.Type == "RULE-SET" && !sets[r.Value] {
			return fmt.Errorf("rule %d: unknown rule set %q", i+1, r.Value)
		}
	}
	return nil
}

func (f *Filter) compile() error {
	if f == nil {
		return nil
	}
	for _, re := range []string{f.Include, f.Exclude} {
		if re != "" {
			if _, err := regexp.Compile(re); err != nil {
				return fmt.Errorf("bad regex %q: %w", re, err)
			}
		}
	}
	return nil
}

// Match reports whether the node matches. An empty filter matches everything.
func (f *Filter) Match(n *node.Node) bool {
	if f.Empty() {
		return true
	}
	onlyNames := f.Include == "" && f.Exclude == "" && len(f.Regions) == 0 && len(f.ExcludeRegions) == 0 && f.NodeKind == "" && len(f.Types) == 0 && len(f.Sources) == 0 && !f.hasProbeConditions()
	if len(f.Names) > 0 && slices.Contains(f.Names, n.Name) {
		return true
	}
	if onlyNames {
		return false
	}
	if f.Include != "" && !re(f.Include).MatchString(n.Name) {
		return false
	}
	if f.Exclude != "" && re(f.Exclude).MatchString(n.Name) {
		return false
	}
	switch f.NodeKind {
	case "airport":
		if n.SelfHosted {
			return false
		}
	case "selfhosted":
		if !n.SelfHosted {
			return false
		}
	}
	if n.SelfHosted && (len(f.Regions) > 0 || len(f.ExcludeRegions) > 0) {
		return false // region groups are for airport nodes
	}
	if len(f.Regions) > 0 && !slices.Contains(f.Regions, node.Region(n)) {
		return false
	}
	if len(f.ExcludeRegions) > 0 && slices.Contains(f.ExcludeRegions, node.Region(n)) {
		return false
	}
	if len(f.Types) > 0 && !slices.Contains(f.Types, string(n.Type)) {
		return false
	}
	if len(f.Sources) > 0 && !slices.Contains(f.Sources, n.Source) {
		return false
	}
	if f.hasProbeConditions() && !f.matchProbe(n.Probe) {
		return false
	}
	return true
}

var reCache sync.Map

// re returns a cached compiled regex; patterns are validated beforehand, an
// invalid one matches nothing.
func re(p string) *regexp.Regexp {
	if v, ok := reCache.Load(p); ok {
		return v.(*regexp.Regexp)
	}
	r, err := regexp.Compile(p)
	if err != nil {
		r = regexp.MustCompile(`$^`)
	}
	reCache.Store(p, r)
	return r
}
