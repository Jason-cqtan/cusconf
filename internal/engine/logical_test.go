package engine

import (
	"strings"
	"testing"

	"github.com/cusconf/cusconf/internal/capability"
	"github.com/cusconf/cusconf/internal/profile"
	"github.com/cusconf/cusconf/internal/render"
)

func logicalRules() []profile.Rule {
	return []profile.Rule{
		// STUN from the Claude app or to AI hosts is rejected.
		{Type: "AND", Policy: "REJECT", Rules: []profile.Rule{
			{Type: "PROTOCOL", Value: "STUN"},
			{Type: "OR", Rules: []profile.Rule{
				{Type: "PROCESS-NAME", Value: "/Applications/Claude.app/*"},
				{Type: "RULE-SET", Value: "AI"},
			}},
		}},
		{Type: "OR", Policy: "DIRECT", Rules: []profile.Rule{
			{Type: "DOMAIN", Value: "a.example.com"},
			{Type: "IP-CIDR", Value: "192.0.2.0/24", NoResolve: true},
		}},
		{Type: "NOT", Policy: "DIRECT", Rules: []profile.Rule{{Type: "DOMAIN-SUFFIX", Value: "b.example.com"}}},
		{Type: "OR", Policy: "DIRECT", Rules: []profile.Rule{
			{Type: "PROCESS-NAME", Value: "/Applications/ChatGPT.app/*"},
			{Type: "DOMAIN", Value: "c.example.com"},
		}},
	}
}

func genLogical(t *testing.T, target capability.Target) render.Result {
	t.Helper()
	p := fixtureNodes(t)
	p.Rules = append(logicalRules(), p.Rules...)
	if err := p.Validate(); err != nil {
		t.Fatal(err)
	}
	res, err := Generate(Request{Profile: p, Nodes: loadNodes(t), Target: target})
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func TestLogicalRules(t *testing.T) {
	const stun = "AND,((PROTOCOL,STUN),(OR,((PROCESS-NAME,/Applications/Claude.app/*),(RULE-SET,"
	or := "OR,((DOMAIN,a.example.com),(IP-CIDR,192.0.2.0/24,no-resolve)),DIRECT"
	not := "NOT,((DOMAIN-SUFFIX,b.example.com)),DIRECT"
	proc := "OR,((PROCESS-NAME,/Applications/ChatGPT.app/*),(DOMAIN,c.example.com)),DIRECT"
	procRegex := `OR,((PROCESS-PATH-REGEX,^/Applications/ChatGPT\.app/.*$),(DOMAIN,c.example.com)),DIRECT`
	cases := []struct {
		target  capability.Target
		want    []string
		notWant []string
		warned  []string // prefixes of rule names expected in warnings
	}{
		{capability.Target{Client: capability.Surge, Platform: "mac"}, []string{stun, ")))),REJECT", or, not, proc}, nil, nil},
		// iOS cannot match processes: the OR keeps only its rule set branch.
		{capability.Target{Client: capability.Surge, Platform: "ios"}, []string{"AND,((PROTOCOL,STUN),(OR,((RULE-SET,", or, not}, []string{"PROCESS-NAME"}, nil},
		{capability.Target{Client: capability.Stash, Platform: "mac"}, []string{"AND,((PROTOCOL,STUN),(OR,((PROCESS-PATH,/Applications/Claude.app/*),(RULE-SET,AI)))),REJECT", or, not}, nil, nil},
		// mihomo has no STUN matcher: the whole AND is skipped, not just the condition.
		{capability.Target{Client: capability.ClashMeta}, []string{or, not, procRegex}, []string{"PROTOCOL", "STUN"}, []string{"AND,((PROTOCOL,STUN)"}},
		// Loon logical rules cannot refer to remote rule sets.
		{capability.Target{Client: capability.Loon}, []string{or, not}, []string{"AND,("}, []string{"RULE-SET,AI"}},
		{capability.Target{Client: capability.SingBox}, []string{`"type": "logical"`, `"mode": "or"`, `"invert": true`, `"process_path_regex"`}, nil, nil},
		{capability.Target{Client: capability.QX}, nil, []string{"AND,", "OR,(", "NOT,("}, []string{"NOT,((DOMAIN-SUFFIX,b.example.com))"}},
	}
	for _, c := range cases {
		res := genLogical(t, c.target)
		for _, s := range c.want {
			if !strings.Contains(res.Content, s) {
				t.Errorf("%s: missing %q", c.target, s)
			}
		}
		for _, s := range c.notWant {
			if strings.Contains(res.Content, s) {
				t.Errorf("%s: unexpected %q", c.target, s)
			}
		}
		for _, n := range c.warned {
			found := false
			for _, w := range res.Warnings {
				found = found || strings.HasPrefix(w.Name, n)
			}
			if !found {
				t.Errorf("%s: no warning for %s; got %v", c.target, n, res.Warnings)
			}
		}
	}
}

func TestLogicalRuleValidation(t *testing.T) {
	bad := []profile.Rule{
		{Type: "NOT", Policy: "DIRECT", Rules: []profile.Rule{{Type: "DOMAIN", Value: "a.com"}, {Type: "DOMAIN", Value: "b.com"}}},
		{Type: "AND", Policy: "DIRECT"},
		{Type: "AND", Policy: "DIRECT", Rules: []profile.Rule{{Type: "RULE-SET", Value: "nope"}}},
		{Type: "OR", Policy: "DIRECT", Rules: []profile.Rule{{Type: "FINAL"}}},
		{Type: "OR", Policy: "DIRECT", Rules: []profile.Rule{{Type: "DOMAIN"}}},
	}
	for _, r := range bad {
		p := fixtureNodes(t)
		p.Rules = append([]profile.Rule{r}, p.Rules...)
		if err := p.Validate(); err == nil {
			t.Errorf("%s: expected a validation error", r.Expr())
		}
	}
}

func TestAdaptLogicalProcessOnIOS(t *testing.T) {
	ios := capability.Target{Client: capability.Surge, Platform: "ios"}
	proc := profile.Rule{Type: "PROCESS-NAME", Value: "Claude"}
	dom := profile.Rule{Type: "DOMAIN", Value: "a.com"}
	in := []profile.Rule{
		{Type: "AND", Policy: "REJECT", Rules: []profile.Rule{proc, dom}},                                       // never matches: dropped
		{Type: "OR", Policy: "DIRECT", Rules: []profile.Rule{proc, dom}},                                        // keeps the domain
		{Type: "AND", Policy: "DIRECT", Rules: []profile.Rule{{Type: "NOT", Rules: []profile.Rule{proc}}, dom}}, // NOT(process) is always true
	}
	out := adaptProcessRules(in, ios)
	if len(out) != 2 {
		t.Fatalf("got %d rules: %+v", len(out), out)
	}
	for _, r := range out {
		if len(r.Rules) != 1 || r.Rules[0].Type != "DOMAIN" {
			t.Errorf("%s: want only the domain condition left", r.Expr())
		}
	}
}
