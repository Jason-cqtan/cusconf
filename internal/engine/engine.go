// Package engine turns a profile + nodes into a client config.
package engine

import (
	"fmt"
	"net/netip"
	"net/url"
	"regexp"
	"slices"
	"sort"
	"strings"

	"github.com/cusconf/cusconf/internal/capability"
	"github.com/cusconf/cusconf/internal/node"
	"github.com/cusconf/cusconf/internal/profile"
	"github.com/cusconf/cusconf/internal/render"
	"github.com/cusconf/cusconf/internal/render/clash"
	"github.com/cusconf/cusconf/internal/render/loon"
	"github.com/cusconf/cusconf/internal/render/qx"
	"github.com/cusconf/cusconf/internal/render/singbox"
	"github.com/cusconf/cusconf/internal/render/surge"
)

type Request struct {
	Profile    *profile.Profile
	Nodes      []*node.Node // all nodes from all sources (Source field set)
	Target     capability.Target
	ManagedURL string
	// Subscriptions maps enabled subscription source IDs to their URLs; the hosts of the
	// profile's sources get DIRECT rules too.
	Subscriptions map[string]string
	// Mirror returns this server's rule set mirror URL (see render.Config.Mirror); optional.
	Mirror func(upstream, behavior, format string) string
	// MirrorLocal marks mirror URLs that only work on this machine (the server was
	// reached as localhost and no public base URL is set). Other devices cannot load
	// them, so rule sets keep their original URLs where the client has one.
	MirrorLocal bool
}

// SelectNodes applies the profile's source selection, filter and renames.
func SelectNodes(p *profile.Profile, all []*node.Node) []*node.Node {
	var out []*node.Node
	seen := map[string]bool{}
	renames := make([]*regexp.Regexp, len(p.Rename))
	for i, r := range p.Rename {
		renames[i], _ = regexp.Compile(r.Pattern)
	}
	for _, n := range all {
		if len(p.Sources) > 0 && !slices.Contains(p.Sources, n.Source) {
			continue
		}
		if !p.Filter.Match(n) {
			continue
		}
		c := *n
		for i, re := range renames {
			if re != nil {
				c.Name = re.ReplaceAllString(c.Name, p.Rename[i].Replace)
			}
		}
		if seen[c.Name] {
			base := c.Name
			for i := 2; seen[c.Name]; i++ {
				c.Name = fmt.Sprintf("%s %d", base, i)
			}
		}
		seen[c.Name] = true
		out = append(out, &c)
	}
	return out
}

// ResolveGroups expands member lists and node filters into concrete names.
func ResolveGroups(p *profile.Profile, nodes []*node.Node, w *render.Warnings) []render.Group {
	nodeNames := map[string]bool{}
	for _, n := range nodes {
		nodeNames[n.Name] = true
	}
	groupNames := map[string]bool{}
	for _, g := range p.Groups {
		groupNames[g.Name] = true
	}
	out := make([]render.Group, 0, len(p.Groups))
	for _, g := range p.Groups {
		rg := render.Group{Group: g}
		seen := map[string]bool{}
		add := func(m string) {
			if !seen[m] {
				seen[m] = true
				rg.Members = append(rg.Members, m)
			}
		}
		for _, m := range g.Members {
			switch {
			case m == g.Name:
				w.Add("group", g.Name, "group cannot contain itself")
			case profile.IsBuiltin(m), groupNames[m], nodeNames[m]:
				add(m)
			default:
				w.Add("group", g.Name, "member %q not found; removed", m)
			}
		}
		if g.Nodes != nil {
			for _, n := range nodes {
				if g.Nodes.Match(n) {
					add(n.Name)
				}
			}
		}
		out = append(out, rg)
	}
	breakCycles(out, w)
	flattenFiltered(out, nodes)
	for i := range out {
		out[i].Members = applyOrder(out[i].Members, out[i].Order)
	}
	return out
}

// flattenFiltered expands the member groups of groups with a MemberFilter into the
// nodes they reach that match it. Cycles must already be broken.
func flattenFiltered(groups []render.Group, nodes []*node.Node) {
	byName := map[string]*node.Node{}
	for _, n := range nodes {
		byName[n.Name] = n
	}
	idx := map[string]int{}
	for i, g := range groups {
		idx[g.Name] = i
	}
	done := make([]bool, len(groups))
	var final func(i int) []string
	var reach func(i int, out *[]string, seen map[string]bool)
	// reach lists the nodes reachable from group i, in member order.
	reach = func(i int, out *[]string, seen map[string]bool) {
		for _, m := range final(i) {
			if j, ok := idx[m]; ok {
				reach(j, out, seen)
			} else if byName[m] != nil && !seen[m] {
				seen[m] = true
				*out = append(*out, m)
			}
		}
	}
	final = func(i int) []string {
		g := &groups[i]
		if done[i] || g.MemberFilter == nil {
			return g.Members
		}
		done[i] = true
		var members []string
		seen := map[string]bool{}
		for _, m := range g.Members {
			j, isGroup := idx[m]
			if !isGroup {
				if !seen[m] {
					seen[m] = true
					members = append(members, m)
				}
				continue
			}
			var reached []string
			reach(j, &reached, map[string]bool{})
			for _, n := range reached {
				if !seen[n] && g.MemberFilter.Match(byName[n]) {
					seen[n] = true
					members = append(members, n)
				}
			}
		}
		g.Members = members
		return members
	}
	for i := range groups {
		final(i)
	}
}

// breakCycles removes group->group edges that close a cycle.
func breakCycles(groups []render.Group, w *render.Warnings) {
	idx := map[string]int{}
	for i, g := range groups {
		idx[g.Name] = i
	}
	state := make([]int, len(groups)) // 0 new, 1 visiting, 2 done
	var visit func(i int)
	visit = func(i int) {
		state[i] = 1
		g := &groups[i]
		kept := g.Members[:0]
		for _, m := range g.Members {
			j, isGroup := idx[m]
			if isGroup {
				if state[j] == 1 {
					w.Add("group", g.Name, "member %q creates a cycle; removed", m)
					continue
				}
				if state[j] == 0 {
					visit(j)
				}
			}
			kept = append(kept, m)
		}
		g.Members = kept
		state[i] = 2
	}
	for i := range groups {
		if state[i] == 0 {
			visit(i)
		}
	}
}

// Generate renders the config for the request target.
func Generate(req Request) (render.Result, error) {
	mx := capability.Default
	t := req.Target
	if err := mx.Validate(&t); err != nil {
		return render.Result{}, err
	}
	if err := req.Profile.Validate(); err != nil {
		return render.Result{}, fmt.Errorf("profile: %w", err)
	}
	var w render.Warnings
	nodes := SelectNodes(req.Profile, req.Nodes)
	if len(nodes) == 0 {
		w.Add("general", "nodes", "no nodes selected")
	}
	groups := ResolveGroups(req.Profile, nodes, &w)
	sets := map[string]profile.RuleSet{}
	for _, rs := range req.Profile.RuleSets {
		sets[rs.Name] = rs
	}
	rules := req.Profile.Rules
	general := req.Profile.General
	if req.MirrorLocal && req.Mirror != nil && hasRuleSets(req.Profile) {
		if general.MirrorRules {
			general.MirrorRules = false
			w.Add("general", "规则集镜像", "this page was opened as localhost, so mirror URLs would point to the device itself; original rule set URLs are used instead. Open the admin page via a LAN / public address or set CUSCONF_BASE_URL")
		}
		if t.Client == capability.SingBox || t.Client == capability.QX {
			w.Add("general", "规则集镜像", "this client loads rule sets through the mirror, whose URLs point to localhost here and will not load on other devices; open the admin page via a LAN / public address or set CUSCONF_BASE_URL")
		}
	}
	// Servers of the profile's sources (all their nodes, not only the filtered ones), so
	// connections to those airports are never sent through another proxy.
	if !general.DisableServerDirect {
		servers := sourceNodes(req.Profile.Sources, req.Nodes)
		subs := sourceURLs(req.Profile.Sources, req.Subscriptions)
		direct := ServerDirectRules(servers, subs)
		if h := dohHost(general.ServerDNS); h != "" {
			direct = append([]profile.Rule{{Type: "DOMAIN", Value: h, Policy: "DIRECT"}}, direct...)
		}
		rules = append(direct, rules...)
		general.Hosts = append(slices.Clone(general.Hosts), ServerDNSHosts(general, servers, subs)...)
	}
	rules = adaptProcessRules(rules, t)
	cfg := &render.Config{
		Target: t, Matrix: mx, Nodes: nodes, Groups: groups,
		Rules: rules, RuleSets: sets, General: general, ManagedURL: req.ManagedURL, Mirror: req.Mirror,
	}
	var res render.Result
	switch t.Client {
	case capability.Surge:
		res = surge.Render(cfg)
	case capability.Stash, capability.ClashMeta, capability.FlClash:
		res = clash.Render(cfg)
	case capability.SingBox:
		res = singbox.Render(cfg)
	case capability.Loon:
		res = loon.Render(cfg)
	case capability.QX:
		res = qx.Render(cfg)
	default:
		return res, fmt.Errorf("no renderer for %s", t.Client)
	}
	res.Warnings = append(w, res.Warnings...)
	return res, nil
}

func hasRuleSets(p *profile.Profile) bool {
	for _, r := range p.Rules {
		if r.Type == "RULE-SET" {
			return true
		}
	}
	return false
}

// sourceNodes lists the real nodes of the given sources (all when empty); airport
// info entries (traffic / expiry) are skipped, since their servers are often placeholders.
func sourceNodes(sources []string, all []*node.Node) []*node.Node {
	var out []*node.Node
	for _, n := range all {
		if !node.IsInfo(n) && (len(sources) == 0 || slices.Contains(sources, n.Source)) {
			out = append(out, n)
		}
	}
	return out
}

// sourceURLs lists the subscription URLs of the given sources (all when empty), sorted.
func sourceURLs(sources []string, subs map[string]string) []string {
	var out []string
	for id, u := range subs {
		if len(sources) == 0 || slices.Contains(sources, id) {
			out = append(out, u)
		}
	}
	sort.Strings(out)
	return out
}

// adaptProcessRules drops process rules on mobile clients (iOS has no process
// matching) and, outside Surge, turns PROCESS-NAME rules holding a full path into
// PROCESS-PATH, since mihomo / sing-box match PROCESS-NAME against the bare name only.
func adaptProcessRules(rules []profile.Rule, t capability.Target) []profile.Rule {
	out := make([]profile.Rule, 0, len(rules))
	for _, r := range rules {
		if r.Type == "PROCESS-NAME" || r.Type == "PROCESS-PATH" {
			if t.Platform == "ios" {
				continue
			}
			if r.Type == "PROCESS-NAME" && t.Client != capability.Surge && strings.Contains(r.Value, "/") {
				r.Type = "PROCESS-PATH"
				r.Value = strings.TrimSuffix(r.Value, "/")
			}
		}
		out = append(out, r)
	}
	return out
}

// applyOrder moves names listed in order to the front, in that order; the rest keep their relative order.
func applyOrder(members, order []string) []string {
	if len(order) == 0 {
		return members
	}
	rank := make(map[string]int, len(order))
	for i, n := range order {
		if _, dup := rank[n]; !dup {
			rank[n] = i
		}
	}
	out := slices.Clone(members)
	slices.SortStableFunc(out, func(a, b string) int {
		ra, okA := rank[a]
		rb, okB := rank[b]
		switch {
		case okA && okB:
			return ra - rb
		case okA:
			return -1
		case okB:
			return 1
		}
		return 0
	})
	return out
}

// ServerDirectRules sends connections to node servers and subscription hosts
// DIRECT. Without them, traffic to those hosts from other apps (or a subscription
// refresh) can be routed into another proxy, which for some protocols never
// completes. Domains get DOMAIN rules, IPs IP-CIDR(6) host routes.
// serverDomains lists the domain names (not IPs) of node servers and subscription hosts, sorted.
func serverDomains(nodes []*node.Node, subscriptionURLs []string) []string {
	seen := map[string]bool{}
	var out []string
	add := func(host string) {
		host = strings.Trim(strings.ToLower(strings.TrimSpace(host)), "[].")
		if host == "" || seen[host] {
			return
		}
		seen[host] = true
		if _, err := netip.ParseAddr(host); err != nil {
			out = append(out, host)
		}
	}
	for _, u := range subscriptionURLs {
		if pu, err := url.Parse(u); err == nil {
			add(pu.Hostname())
		}
	}
	for _, n := range nodes {
		add(n.Server)
	}
	sort.Strings(out)
	return out
}

// ServerDNSHosts maps every node server and subscription domain to General.ServerDNS,
// skipping domains the profile's own hosts already map.
func ServerDNSHosts(g profile.General, nodes []*node.Node, subscriptionURLs []string) []profile.Host {
	if g.ServerDNS == "" {
		return nil
	}
	mine := map[string]bool{}
	for _, h := range g.Hosts {
		mine[strings.ToLower(h.Domain)] = true
	}
	var out []profile.Host
	for _, d := range serverDomains(nodes, subscriptionURLs) {
		if !mine[d] {
			out = append(out, profile.Host{Domain: d, Value: "server:" + g.ServerDNS})
		}
	}
	if len(out) > 0 {
		out[0].Comment = "节点与订阅域名（自动生成）：用国内 DNS 解析，避免经代理的加密 DNS 需要先连上节点"
	}
	return out
}

// dohHost returns the host name of a DoH / DoT / DoQ server address, or "" for a plain IP.
func dohHost(s string) string {
	if !strings.Contains(s, "://") {
		return ""
	}
	u, err := url.Parse(s)
	if err != nil {
		return ""
	}
	h := u.Hostname()
	if _, err := netip.ParseAddr(h); err == nil {
		return ""
	}
	return strings.ToLower(h)
}

func ServerDirectRules(nodes []*node.Node, subscriptionURLs []string) []profile.Rule {
	seen := map[string]bool{}
	var domains, ips []profile.Rule
	add := func(host string) {
		host = strings.Trim(strings.ToLower(strings.TrimSpace(host)), "[].")
		if host == "" || seen[host] {
			return
		}
		seen[host] = true
		if ip, err := netip.ParseAddr(host); err == nil {
			if ip = ip.Unmap(); ip.IsLoopback() || ip.IsPrivate() || ip.IsUnspecified() || ip.IsLinkLocalUnicast() {
				return // local addresses are DIRECT anyway; placeholders must not become rules
			}
			if ip.Is4() {
				ips = append(ips, profile.Rule{Type: "IP-CIDR", Value: ip.Unmap().String() + "/32", Policy: "DIRECT", NoResolve: true})
			} else {
				ips = append(ips, profile.Rule{Type: "IP-CIDR6", Value: ip.String() + "/128", Policy: "DIRECT", NoResolve: true})
			}
			return
		}
		domains = append(domains, profile.Rule{Type: "DOMAIN", Value: host, Policy: "DIRECT"})
	}
	for _, u := range subscriptionURLs {
		if pu, err := url.Parse(u); err == nil {
			add(pu.Hostname())
		}
	}
	for _, n := range nodes {
		add(n.Server)
	}
	sort.SliceStable(domains, func(i, j int) bool { return domains[i].Value < domains[j].Value })
	return append(domains, ips...)
}
