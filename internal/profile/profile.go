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
	Types   []string `json:"types,omitempty"`   // node types
	Sources []string `json:"sources,omitempty"` // source IDs
	Names   []string `json:"names,omitempty"`   // explicit node names (OR'ed with the rest when set alone)
}

func (f *Filter) Empty() bool {
	return f == nil || (f.Include == "" && f.Exclude == "" && len(f.Regions) == 0 && len(f.Types) == 0 && len(f.Sources) == 0 && len(f.Names) == 0)
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
}

// HostKind classifies a Host value: "ip", "server" or "alias".
func (h Host) Kind() string {
	if strings.HasPrefix(h.Value, "server:") {
		return "server"
	}
	if _, err := netip.ParseAddr(h.Value); err == nil {
		return "ip"
	}
	return "alias"
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
	for i, h := range p.General.Hosts {
		if strings.TrimSpace(h.Domain) == "" || strings.TrimSpace(h.Value) == "" {
			return fmt.Errorf("host %d: domain and value are required", i+1)
		}
		if strings.ContainsAny(h.Domain+h.Value, " ,=") {
			return fmt.Errorf("host %d (%s): must not contain spaces, commas or '='", i+1, h.Domain)
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
	onlyNames := f.Include == "" && f.Exclude == "" && len(f.Regions) == 0 && len(f.Types) == 0 && len(f.Sources) == 0
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
	if len(f.Regions) > 0 && !slices.Contains(f.Regions, node.Region(n)) {
		return false
	}
	if len(f.Types) > 0 && !slices.Contains(f.Types, string(n.Type)) {
		return false
	}
	if len(f.Sources) > 0 && !slices.Contains(f.Sources, n.Source) {
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
