// Package engine turns a profile + nodes into a client config.
package engine

import (
	"fmt"
	"regexp"
	"slices"

	"github.com/cusconf/cusconf/internal/capability"
	"github.com/cusconf/cusconf/internal/node"
	"github.com/cusconf/cusconf/internal/profile"
	"github.com/cusconf/cusconf/internal/render"
	"github.com/cusconf/cusconf/internal/render/stash"
	"github.com/cusconf/cusconf/internal/render/surge"
)

type Request struct {
	Profile    *profile.Profile
	Nodes      []*node.Node // all nodes from all sources (Source field set)
	Target     capability.Target
	ManagedURL string
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
		rg.Members = applyOrder(rg.Members, g.Order)
		out = append(out, rg)
	}
	breakCycles(out, w)
	return out
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
	cfg := &render.Config{
		Target: t, Matrix: mx, Nodes: nodes, Groups: groups,
		Rules: req.Profile.Rules, RuleSets: sets, General: req.Profile.General, ManagedURL: req.ManagedURL,
	}
	var res render.Result
	switch t.Client {
	case capability.Surge:
		res = surge.Render(cfg)
	case capability.Stash:
		res = stash.Render(cfg)
	default:
		return res, fmt.Errorf("no renderer for %s", t.Client)
	}
	res.Warnings = append(w, res.Warnings...)
	return res, nil
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
