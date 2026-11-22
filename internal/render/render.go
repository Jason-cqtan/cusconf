// Package render holds the client-independent input for renderers and shared helpers.
package render

import (
	"fmt"
	"strings"

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
	// Mirror, when set, returns this server's mirror URL for a rule list, converted
	// to format (see RuleSetURL). Nil when mirroring is not available.
	Mirror func(upstream, behavior, format string) string
}

// RuleSetURL picks the URL a client loads a rule set from. native is the URL
// configured for this client (may be empty); format is the mirror format the
// client reads: "list" (Surge), "loon" (Loon), "txt" (Clash text), "json" (sing-box
// source), "qx". The mirror is used when the profile turns it on for all rule
// sets, or when the client has no native URL and another one can be converted.
func (c *Config) RuleSetURL(rs profile.RuleSet, native, format string) (url string, mirrored bool) {
	if c.Mirror == nil || (native != "" && !c.General.MirrorRules) {
		return native, false
	}
	up := native
	for _, u := range []string{rs.SurgeURL, rs.StashURL} {
		if up == "" {
			up = u
		}
	}
	if up == "" {
		return "", false
	}
	return c.Mirror(up, rs.Behavior, format), true
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
const DefaultTestURL = "https://cp.cloudflare.com/generate_204"

// KV is one "key = value" line of an INI-style general section.
type KV struct{ K, V string }

// MergeKV renders lines, then applies raw extra text: a "key = value" line whose key
// is already generated replaces it in place; other lines (new keys, comments) are
// appended in order. sep is the separator written between key and value.
func MergeKV(lines []KV, extra, sep string) string {
	lines = append([]KV(nil), lines...)
	var rest []string
	for _, raw := range strings.Split(extra, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		k = strings.TrimSpace(k)
		replaced := false
		if ok && !strings.HasPrefix(line, "#") && !strings.HasPrefix(line, "//") && !strings.HasPrefix(line, ";") {
			for i := range lines {
				if lines[i].K == k {
					lines[i].V = strings.TrimSpace(v)
					replaced = true
				}
			}
		}
		if !replaced {
			rest = append(rest, line)
		}
	}
	var b strings.Builder
	for _, l := range lines {
		b.WriteString(l.K + sep + l.V + "\n")
	}
	for _, l := range rest {
		b.WriteString(l + "\n")
	}
	return b.String()
}

// FirstGroup is the name of the first group, or DIRECT.
func FirstGroup(groups []Group) string {
	if len(groups) > 0 {
		return groups[0].Name
	}
	return "DIRECT"
}
