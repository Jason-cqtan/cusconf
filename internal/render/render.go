// Package render holds the client-independent input for renderers and shared helpers.
package render

import (
	"fmt"

	"github.com/cusconf/cusconf/internal/capability"
	"github.com/cusconf/cusconf/internal/node"
	"github.com/cusconf/cusconf/internal/profile"
)

// Group is a profile group with members resolved to concrete names
// (node names, group names, DIRECT/REJECT).
type Group struct {
	profile.Group
	Members []string
}

type Config struct {
	Target     capability.Target
	Matrix     *capability.Matrix
	Nodes      []*node.Node
	Groups     []Group
	Rules      []profile.Rule
	RuleSets   map[string]profile.RuleSet
	General    profile.General
	ManagedURL string // self URL for managed-config headers
}

func (c *Config) Has(feature string) bool { return c.Matrix.Supports(c.Target, feature) }

func (c *Config) Req(feature string) string { return c.Matrix.Requirement(c.Target, feature) }

type Warning struct {
	Kind    string `json:"kind"` // node | group | rule | general
	Name    string `json:"name"`
	Message string `json:"message"`
}

type Warnings []Warning

func (w *Warnings) Add(kind, name, format string, args ...any) {
	*w = append(*w, Warning{Kind: kind, Name: name, Message: fmt.Sprintf(format, args...)})
}

// Result is a rendered config.
type Result struct {
	Content     string   `json:"content"`
	ContentType string   `json:"contentType"`
	Warnings    Warnings `json:"warnings"`
}

// FinalizeGroups drops members that refer to nodes the renderer could not emit.
// A group left without members falls back to DIRECT so references stay valid.
func FinalizeGroups(groups []Group, kept map[string]bool, w *Warnings) []Group {
	names := map[string]bool{}
	for _, g := range groups {
		names[g.Name] = true
	}
	out := make([]Group, 0, len(groups))
	for _, g := range groups {
		var ms []string
		for _, m := range g.Members {
			if kept[m] || names[m] || profile.IsBuiltin(m) {
				ms = append(ms, m)
			}
		}
		if len(ms) == 0 {
			w.Add("group", g.Name, "no usable members, falling back to DIRECT")
			ms = []string{"DIRECT"}
		}
		g.Members = ms
		out = append(out, g)
	}
	return out
}

// DowngradeGroupType picks the closest supported group type.
func DowngradeGroupType(c *Config, g *Group, w *Warnings) profile.GroupType {
	t := g.Type
	if c.Has("group." + string(t)) {
		return t
	}
	fallback := map[profile.GroupType]profile.GroupType{
		profile.Smart:       profile.URLTest,
		profile.LoadBalance: profile.URLTest,
		profile.Fallback:    profile.URLTest,
		profile.URLTest:     profile.Select,
	}
	for nt := fallback[t]; nt != ""; nt = fallback[nt] {
		if c.Has("group." + string(nt)) {
			w.Add("group", g.Name, "type %s %s; using %s", t, c.Req("group."+string(t)), nt)
			return nt
		}
	}
	return profile.Select
}

// DefaultTestURL is used for automatic groups when none is configured.
const DefaultTestURL = "http://www.gstatic.com/generate_204"
