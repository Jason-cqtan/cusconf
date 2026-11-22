package engine

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/cusconf/cusconf/internal/capability"
	"github.com/cusconf/cusconf/internal/node"
	"github.com/cusconf/cusconf/internal/parser"
	"github.com/cusconf/cusconf/internal/profile"
	"github.com/cusconf/cusconf/internal/render"
)

func fixtureNodes(t *testing.T) *profile.Profile {
	t.Helper()
	p := profile.Default(nil, 0)
	// Renderer tests exercise a smart group with every node in it.
	p.Groups[0].Type, p.Groups[0].Members, p.Groups[0].Nodes = profile.Smart, nil, &profile.Filter{}
	return &p
}

func gen(t *testing.T, target capability.Target) render.Result {
	t.Helper()
	var all = loadNodes(t)
	p := fixtureNodes(t)
	res, err := Generate(Request{Profile: p, Nodes: all, Target: target, ManagedURL: "https://example.com/sub/x"})
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func loadNodes(t *testing.T) []*node.Node {
	b, err := os.ReadFile("../parser/testdata/uri.txt")
	if err != nil {
		t.Fatal(err)
	}
	r := parser.Parse(b)
	for _, n := range r.Nodes {
		n.Source = "s1"
	}
	return r.Nodes
}

func hasWarning(ws render.Warnings, name, substr string) bool {
	for _, w := range ws {
		if w.Name == name && strings.Contains(w.Message, substr) {
			return true
		}
	}
	return false
}

func TestSurgeVersions(t *testing.T) {
	newer := gen(t, capability.Target{Client: capability.Surge, Platform: "ios", Version: "6.1.0"})
	if !strings.Contains(newer.Content, "US Reality = vless, us.example.com, 443, username=") ||
		!strings.Contains(newer.Content, "reality-public-key=SomePublicKeyBase64") {
		t.Errorf("vless missing in 6.1:\n%s", newer.Content)
	}
	if !strings.Contains(newer.Content, "全局代理 = smart,") {
		t.Error("smart group missing")
	}
	if !strings.Contains(newer.Content, "[WireGuard wg-") || !strings.Contains(newer.Content, "client-id = 1/2/3") {
		t.Error("wireguard section missing")
	}
	if !strings.Contains(newer.Content, "#!MANAGED-CONFIG https://example.com/sub/x") {
		t.Error("managed header missing")
	}
	if strings.Contains(newer.Content, "SSR Node =") || !hasWarning(newer.Warnings, "SSR Node", "not supported") {
		t.Error("ssr should be dropped with warning")
	}

	older := gen(t, capability.Target{Client: capability.Surge, Platform: "ios", Version: "5.10.0"})
	if strings.Contains(older.Content, "= vless,") || !hasWarning(older.Warnings, "US Reality", "6.0.0") {
		t.Errorf("vless should be dropped on 5.10: %v", older.Warnings)
	}
	if !strings.Contains(older.Content, "全局代理 = url-test,") || !hasWarning(older.Warnings, "全局代理", "using url-test") {
		t.Error("smart should downgrade to url-test on 5.10")
	}
	if strings.Contains(older.Content, "AnyTLS =") {
		t.Error("anytls requires 5.17")
	}
	if !strings.Contains(older.Content, "HY2 = hysteria2") {
		t.Error("hysteria2 should exist on 5.10")
	}
	// Mac icon-url gating
	mac := gen(t, capability.Target{Client: capability.Surge, Platform: "mac", Version: "6.4.0"})
	if strings.Contains(mac.Content, "icon-url=") {
		t.Error("icon-url requires mac 6.5.0")
	}
	if !strings.Contains(gen(t, capability.Target{Client: capability.Surge, Platform: "mac"}).Content, "icon-url=") {
		t.Error("latest mac should have icon-url")
	}
}

func TestStash(t *testing.T) {
	res := gen(t, capability.Target{Client: capability.Stash, Platform: "ios"})
	var doc struct {
		Proxies       []map[string]any          `yaml:"proxies"`
		ProxyGroups   []map[string]any          `yaml:"proxy-groups"`
		RuleProviders map[string]map[string]any `yaml:"rule-providers"`
		Rules         []string                  `yaml:"rules"`
	}
	if err := yaml.Unmarshal([]byte(res.Content), &doc); err != nil {
		t.Fatalf("invalid yaml: %v\n%s", err, res.Content)
	}
	if len(doc.Proxies) != 14 {
		t.Errorf("proxies: %d, warnings %v", len(doc.Proxies), res.Warnings)
	}
	byName := map[string]map[string]any{}
	for _, p := range doc.Proxies {
		byName[p["name"].(string)] = p
	}
	if ro, ok := byName["US Reality"]["reality-opts"].(map[string]any); !ok || ro["public-key"] != "SomePublicKeyBase64" {
		t.Errorf("reality-opts: %v", byName["US Reality"])
	}
	if byName["HY2"]["ports"] != "20000-30000" {
		t.Errorf("hy2: %v", byName["HY2"])
	}
	for _, g := range doc.ProxyGroups {
		if g["type"] == "smart" {
			t.Error("stash has no smart group")
		}
	}
	if doc.RuleProviders["ProxyGFWlist"]["behavior"] != "classical" {
		t.Errorf("providers: %v", doc.RuleProviders)
	}
	if doc.Rules[len(doc.Rules)-1] != "MATCH,漏网之鱼" {
		t.Errorf("last rule: %s", doc.Rules[len(doc.Rules)-1])
	}
	old := gen(t, capability.Target{Client: capability.Stash, Platform: "ios", Version: "3.0.0"})
	if !hasWarning(old.Warnings, "US Reality", "3.1.0") || !hasWarning(old.Warnings, "AnyTLS", "3.3.0") {
		t.Errorf("old stash warnings: %v", old.Warnings)
	}
}

func TestRegionGroups(t *testing.T) {
	res := gen(t, capability.Target{Client: capability.Surge, Platform: "ios"})
	// HK region group should contain the two HK nodes.
	for _, line := range strings.Split(res.Content, "\n") {
		if strings.HasPrefix(line, "香港 = url-test") {
			if !strings.Contains(line, "🇭🇰 HK 01") || !strings.Contains(line, "HK 02") {
				t.Errorf("hk group: %s", line)
			}
			return
		}
	}
	t.Error("hk group not found")
}

func TestApplyOrder(t *testing.T) {
	got := applyOrder([]string{"A", "B", "C", "D"}, []string{"C", "gone", "A"})
	want := []string{"C", "A", "B", "D"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("got %v want %v", got, want)
	}
}

func TestTemplateMembers(t *testing.T) {
	p := profile.Default(map[string]int{"HK": 3, "TW": 2, "JP": 2, "SG": 2, "US": 2, "KR": 2}, 1)
	got := map[string]profile.Group{}
	var order []string
	for _, g := range p.Groups {
		got[g.Name] = g
		order = append(order, g.Name)
	}
	if strings.Join(order, ",") != "全局代理,漏网之鱼,自建,香港,台湾,日本,新加坡,美国,其他国家" {
		t.Errorf("order: %v", order)
	}
	if g := got["全局代理"]; g.Type != profile.URLTest || g.Nodes != nil || strings.Join(g.Members, ",") != "香港,台湾,日本,新加坡,美国" {
		t.Errorf("全局代理: %+v", g)
	}
	if g := got["漏网之鱼"]; g.Type != profile.Fallback || strings.Join(g.Members, ",") != "全局代理,DIRECT" {
		t.Errorf("漏网之鱼: %+v", g)
	}
	if err := p.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestHosts(t *testing.T) {
	p := profile.Default(nil, 0)
	p.General.ServerDNS = "" // covered by TestServerDNS
	p.General.Hosts = []profile.Host{
		{Domain: "router.lan", Value: "192.168.1.1"},
		{Domain: "*.corp.example.com", Value: "server:10.0.0.53"},
		{Domain: "intra.example.com", Value: "server:system"},
		{Domain: "alias.example.com", Value: "real.example.com"},
	}
	nodes := loadNodes(t)
	sg, err := Generate(Request{Profile: &p, Nodes: nodes, Target: capability.Target{Client: capability.Surge, Platform: "ios"}})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"[Host]\nrouter.lan = 192.168.1.1\n*.corp.example.com = server:10.0.0.53\nintra.example.com = server:system\nalias.example.com = real.example.com\n"} {
		if !strings.Contains(sg.Content, want) {
			t.Errorf("surge missing %q", want)
		}
	}
	st, err := Generate(Request{Profile: &p, Nodes: nodes, Target: capability.Target{Client: capability.Stash, Platform: "ios"}})
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Hosts map[string]string `yaml:"hosts"`
		DNS   struct {
			Policy map[string]string `yaml:"nameserver-policy"`
		} `yaml:"dns"`
	}
	if err := yaml.Unmarshal([]byte(st.Content), &doc); err != nil {
		t.Fatal(err)
	}
	if doc.Hosts["router.lan"] != "192.168.1.1" || doc.DNS.Policy["*.corp.example.com"] != "10.0.0.53" || len(doc.Hosts) != 1 || len(doc.DNS.Policy) != 1 {
		t.Errorf("stash hosts %v policy %v", doc.Hosts, doc.DNS.Policy)
	}
	if !hasWarning(st.Warnings, "intra.example.com", "system") || !hasWarning(st.Warnings, "alias.example.com", "alias") {
		t.Errorf("warnings: %v", st.Warnings)
	}
	bad := profile.Default(nil, 0)
	bad.General.Hosts = []profile.Host{{Domain: "a b", Value: "1.1.1.1"}}
	if err := bad.Validate(); err == nil {
		t.Error("expected validation error")
	}
}

func TestGeneralExtra(t *testing.T) {
	p := profile.Default(nil, 0)
	p.General.Hosts = []profile.Host{{Domain: "router.lan", Value: "192.168.1.1"}, {Domain: "DOMAIN-SET:https://x/y.txt", Value: "server:https://dns.alidns.com/dns-query"}}
	p.General.Extra = map[string]string{
		"surge": "dns-server = 1.1.1.1\nshow-error-page-for-reject = true\n# note",
		"stash": "dns:\n  fake-ip-filter:\n    - '*.lan'\nlog-level: warning\nexternal-controller: 127.0.0.1:9090",
	}
	nodes := loadNodes(t)
	sg, _ := Generate(Request{Profile: &p, Nodes: nodes, Target: capability.Target{Client: capability.Surge, Platform: "mac"}})
	if strings.Count(sg.Content, "dns-server =") != 1 || !strings.Contains(sg.Content, "dns-server = 1.1.1.1\n") ||
		!strings.Contains(sg.Content, "show-error-page-for-reject = true\n# note\n") {
		t.Errorf("surge general:\n%s", sg.Content[:600])
	}
	if !strings.Contains(sg.Content, "DOMAIN-SET:https://x/y.txt = server:https://dns.alidns.com/dns-query") {
		t.Error("surge rule-set host missing")
	}
	st, _ := Generate(Request{Profile: &p, Nodes: nodes, Target: capability.Target{Client: capability.Stash, Platform: "ios"}})
	var doc struct {
		LogLevel string `yaml:"log-level"`
		Ctl      string `yaml:"external-controller"`
		Hosts    map[string]string
		DNS      struct {
			NS     []string `yaml:"nameserver"`
			Filter []string `yaml:"fake-ip-filter"`
		}
	}
	if err := yaml.Unmarshal([]byte(st.Content), &doc); err != nil {
		t.Fatalf("%v\n%s", err, st.Content)
	}
	if doc.LogLevel != "warning" || doc.Ctl != "127.0.0.1:9090" || len(doc.DNS.NS) == 0 || len(doc.DNS.Filter) != 1 || doc.Hosts["router.lan"] == "" || len(doc.Hosts) != 1 {
		t.Errorf("stash merge: %+v", doc)
	}
	if strings.Count(st.Content, "\nlog-level:") != 1 || !hasWarning(st.Warnings, "DOMAIN-SET:https://x/y.txt", "Surge-only") {
		t.Errorf("dup keys or warning missing: %v", st.Warnings)
	}
	p.General.Extra["stash"] = "not: [valid"
	st, _ = Generate(Request{Profile: &p, Nodes: nodes, Target: capability.Target{Client: capability.Stash, Platform: "ios"}})
	if !hasWarning(st.Warnings, "Stash 追加 YAML", "ignored") {
		t.Error("invalid extra should warn")
	}
}

func TestProbeFilter(t *testing.T) {
	nodes := []*node.Node{
		{Name: "a", Probe: &node.Probe{Latency: 80, ExitCountry: "US", Scores: map[string]float64{"IPPure": 95, "Net.Coffee": 90}, Unlocked: []string{"netflix", "chatgpt"}}},
		{Name: "b", Probe: &node.Probe{Latency: 300, ExitCountry: "US", Scores: map[string]float64{"IPPure": 30, "Net.Coffee": 92}, Unlocked: []string{"chatgpt"}, Flags: []string{"机房", "滥用"}}},
		{Name: "c", Probe: &node.Probe{Latency: 0}},
		{Name: "d"}, // never tested
	}
	names := func(f profile.Filter) string {
		var out []string
		for _, n := range nodes {
			if f.Match(n) {
				out = append(out, n.Name)
			}
		}
		return strings.Join(out, ",")
	}
	cases := []struct {
		f    profile.Filter
		want string
	}{
		{profile.Filter{}, "a,b,c,d"},
		{profile.Filter{Reachable: true}, "a,b"},
		{profile.Filter{MaxLatency: 200}, "a"},
		{profile.Filter{MinScores: map[string]float64{"IPPure": 50}}, "a"},
		{profile.Filter{MinScores: map[string]float64{"Net.Coffee": 91}}, "b"},
		{profile.Filter{MinScores: map[string]float64{"IPPure": 20, "Net.Coffee": 85}}, "a,b"},
		{profile.Filter{MinScores: map[string]float64{"proxycheck.io": 1}}, ""},
		{profile.Filter{Unlocks: []string{"chatgpt"}}, "a,b"},
		{profile.Filter{Unlocks: []string{"netflix", "chatgpt"}}, "a"},
		{profile.Filter{ExitRegions: []string{"US"}, MinScores: map[string]float64{"IPPure": 20}}, "a,b"},
		{profile.Filter{Names: []string{"d"}, Reachable: true}, "a,b,d"},
		{profile.Filter{ExcludeFlags: []string{"滥用"}}, "a,c"},
		{profile.Filter{ExcludeFlags: []string{"VPN"}, Reachable: true}, "a,b"},
	}
	for i, c := range cases {
		if got := names(c.f); got != c.want {
			t.Errorf("case %d: got %s want %s", i, got, c.want)
		}
	}
}

func TestRegionTemplate(t *testing.T) {
	p := profile.Default(map[string]int{"HK": 24, "JP": 25, "TW": 9, "KR": 5, "DE": 4, "GB": 1, "": 3}, 0)
	got := map[string]profile.Group{}
	var order []string
	for _, g := range p.Groups {
		got[g.Name] = g
		order = append(order, g.Name)
	}
	if strings.Join(order, ",") != "全局代理,漏网之鱼,香港,台湾,日本,其他国家" {
		t.Errorf("order: %v", order)
	}
	for _, name := range []string{"香港", "日本", "台湾"} {
		if g := got[name]; g.Type != profile.URLTest || !g.Hidden {
			t.Errorf("%s: type %q hidden %v, want hidden url-test", name, g.Type, g.Hidden)
		}
	}
	if g := got["全局代理"]; strings.Join(g.Members, ",") != "香港,台湾,日本" {
		t.Errorf("全局代理 members: %v", g.Members)
	}
	other := got[profile.OtherGroup]
	if other.Type != profile.Select || strings.Join(other.Nodes.ExcludeRegions, ",") != "HK,TW,JP,SG,US" {
		t.Errorf("其他国家: %+v %+v", other, other.Nodes)
	}
	if err := p.Validate(); err != nil {
		t.Fatal(err)
	}
	// 其他国家 picks up other countries and nodes with no region.
	f := other.Nodes
	for name, want := range map[string]bool{"🇬🇧 London": true, "🇰🇷 Seoul": true, "Mystery": true, "🇭🇰 HK 01": false, "🇺🇸 LA": false} {
		if f.Match(&node.Node{Name: name}) != want {
			t.Errorf("其他国家 match %s: want %v", name, want)
		}
	}
	// Only main regions: no 其他国家.
	q := profile.Default(map[string]int{"HK": 2, "US": 1}, 0)
	if len(q.Groups) != 4 || q.Groups[3].Name != "美国" {
		t.Errorf("groups: %+v", q.Groups)
	}
}

func TestServerDirectRules(t *testing.T) {
	nodes := []*node.Node{
		{Name: "a", Server: "B.example.com"}, {Name: "b", Server: "b.example.com"},
		{Name: "c", Server: "1.2.3.4"}, {Name: "d", Server: "2001:db8::1"}, {Name: "e", Server: "a.example.com"},
	}
	got := ServerDirectRules(nodes, []string{"https://sub.airport.com/api/v1/client?token=x", "::bad"})
	var lines []string
	for _, r := range got {
		lines = append(lines, fmt.Sprintf("%s,%s,%s,%v", r.Type, r.Value, r.Policy, r.NoResolve))
	}
	want := "DOMAIN,a.example.com,DIRECT,false|DOMAIN,b.example.com,DIRECT,false|DOMAIN,sub.airport.com,DIRECT,false|IP-CIDR,1.2.3.4/32,DIRECT,true|IP-CIDR6,2001:db8::1/128,DIRECT,true"
	if strings.Join(lines, "|") != want {
		t.Errorf("got  %s\nwant %s", strings.Join(lines, "|"), want)
	}

	// On by default, first in the config; off when disabled.
	p := profile.Default(nil, 0)
	res, err := Generate(Request{Profile: &p, Nodes: loadNodes(t), Target: capability.Target{Client: capability.Surge, Platform: "ios"}, SubscriptionURLs: []string{"https://sub.airport.com/x"}})
	if err != nil {
		t.Fatal(err)
	}
	rules := res.Content[strings.Index(res.Content, "[Rule]\n")+7:]
	if !strings.HasPrefix(rules, "DOMAIN,dns.alidns.com,DIRECT\nDOMAIN,at.example.com,DIRECT\n") || !strings.Contains(rules, "DOMAIN,sub.airport.com,DIRECT\n") || !strings.Contains(rules, "IP-CIDR,1.2.3.4/32,DIRECT,no-resolve\n") {
		t.Errorf("rules:\n%s", rules[:400])
	}
	st, _ := Generate(Request{Profile: &p, Nodes: loadNodes(t), Target: capability.Target{Client: capability.Stash, Platform: "ios"}})
	if !strings.Contains(st.Content, "- DOMAIN,hy.example.com,DIRECT") {
		t.Error("stash should have server direct rules")
	}
	p.General.DisableServerDirect = true
	res, _ = Generate(Request{Profile: &p, Nodes: loadNodes(t), Target: capability.Target{Client: capability.Surge, Platform: "ios"}})
	if strings.Contains(res.Content, "DOMAIN,at.example.com,DIRECT") {
		t.Error("disabled but rules present")
	}
}

func TestSelfHosted(t *testing.T) {
	nodes := []*node.Node{
		{Name: "🇭🇰 HK airport"}, {Name: "🇯🇵 JP airport"}, {Name: "🇬🇧 UK airport"},
		{Name: "🇭🇰 HK my vps", SelfHosted: true}, {Name: "home", SelfHosted: true},
	}
	match := func(f profile.Filter) string {
		var out []string
		for _, n := range nodes {
			if f.Match(n) {
				out = append(out, n.Name)
			}
		}
		return strings.Join(out, ",")
	}
	if got := match(profile.Filter{Regions: []string{"HK"}}); got != "🇭🇰 HK airport" {
		t.Errorf("region group took self-hosted: %s", got)
	}
	if got := match(profile.Filter{ExcludeRegions: []string{"HK", "JP"}}); got != "🇬🇧 UK airport" {
		t.Errorf("其他 took self-hosted: %s", got)
	}
	if got := match(profile.Filter{NodeKind: "selfhosted"}); got != "🇭🇰 HK my vps,home" {
		t.Errorf("selfhosted: %s", got)
	}
	if got := match(profile.Filter{NodeKind: "airport"}); got != "🇭🇰 HK airport,🇯🇵 JP airport,🇬🇧 UK airport" {
		t.Errorf("airport: %s", got)
	}
	if got := match(profile.Filter{}); strings.Count(got, ",") != 4 {
		t.Errorf("empty filter should match all: %s", got)
	}
	p := profile.Default(map[string]int{"HK": 2}, 2)
	if g := p.Groups[2]; g.Name != profile.SelfHostedGroup || g.Nodes.NodeKind != "selfhosted" {
		t.Errorf("自建 group: %+v", g)
	}
	for _, g := range profile.Default(map[string]int{"HK": 2}, 0).Groups {
		if g.Name == profile.SelfHostedGroup {
			t.Error("no self-hosted nodes, no 自建 group")
		}
	}
}

func TestMemberFilter(t *testing.T) {
	nodes := []*node.Node{
		{Name: "🇸🇬 SG 1", Probe: &node.Probe{Latency: 50, Unlocked: []string{"claude"}}},
		{Name: "🇸🇬 SG 2", Probe: &node.Probe{Latency: 60}},
		{Name: "🇯🇵 JP 1", Probe: &node.Probe{Latency: 70, Unlocked: []string{"claude"}}},
		{Name: "🇯🇵 JP 2"},
	}
	p := profile.Profile{Groups: []profile.Group{
		{Name: "Claude", Type: profile.Select, Members: []string{"亚洲", "DIRECT"}, MemberFilter: &profile.Filter{Unlocks: []string{"claude"}}},
		{Name: "亚洲", Type: profile.Select, Members: []string{"新加坡", "日本"}},
		{Name: "新加坡", Type: profile.URLTest, Nodes: &profile.Filter{Regions: []string{"SG"}}},
		{Name: "日本", Type: profile.URLTest, Nodes: &profile.Filter{Regions: []string{"JP"}}},
		{Name: "Plain", Type: profile.Select, Members: []string{"新加坡"}, MemberFilter: &profile.Filter{}},
	}}
	if err := p.Validate(); err != nil {
		t.Fatal(err)
	}
	var w render.Warnings
	got := map[string]string{}
	for _, g := range ResolveGroups(&p, nodes, &w) {
		got[g.Name] = strings.Join(g.Members, ",")
	}
	if got["Claude"] != "🇸🇬 SG 1,🇯🇵 JP 1,DIRECT" {
		t.Errorf("Claude: %s", got["Claude"])
	}
	if got["亚洲"] != "新加坡,日本" {
		t.Errorf("亚洲 should be untouched: %s", got["亚洲"])
	}
	if got["Plain"] != "🇸🇬 SG 1,🇸🇬 SG 2" {
		t.Errorf("empty filter flattens all: %s", got["Plain"])
	}
}

func TestRuleComments(t *testing.T) {
	p := profile.Default(nil, 0)
	nodes := loadNodes(t)
	sg, err := Generate(Request{Profile: &p, Nodes: nodes, Target: capability.Target{Client: capability.Surge, Platform: "mac"}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sg.Content, "# 解决观看B 站卡顿\nDOMAIN-WILDCARD,*pcdn*.biliapi.net,REJECT\nDOMAIN-SUFFIX,mcdn.bilivideo.cn,REJECT\n") {
		t.Errorf("surge rules:\n%s", sg.Content[strings.Index(sg.Content, "[Rule]"):])
	}
	st, err := Generate(Request{Profile: &p, Nodes: nodes, Target: capability.Target{Client: capability.Stash, Platform: "ios"}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(st.Content, "  # 解决观看B 站卡顿\n  - DOMAIN-WILDCARD,*pcdn*.biliapi.net,REJECT\n") {
		t.Errorf("stash rules:\n%s", st.Content[strings.Index(st.Content, "rules:"):])
	}
	var doc struct{ Rules []string }
	if err := yaml.Unmarshal([]byte(st.Content), &doc); err != nil || len(doc.Rules) == 0 {
		t.Fatalf("stash yaml: %v", err)
	}
}

func TestServerDNS(t *testing.T) {
	p := profile.Default(nil, 0)
	p.General.Hosts = []profile.Host{
		{Domain: "dns.alidns.com", Value: "223.5.5.5,223.6.6.6"},
		{Domain: "hk.example.com", Value: "server:system"}, // the user's own mapping wins
	}
	if err := p.Validate(); err != nil {
		t.Fatal(err)
	}
	nodes := loadNodes(t)
	sg, err := Generate(Request{Profile: &p, Nodes: nodes, Target: capability.Target{Client: capability.Surge, Platform: "mac"}, SubscriptionURLs: []string{"https://sub.airport.com/x"}})
	if err != nil {
		t.Fatal(err)
	}
	host := sg.Content[strings.Index(sg.Content, "\n[Host]\n"):]
	for _, want := range []string{
		"dns.alidns.com = 223.5.5.5, 223.6.6.6\n",
		"hk.example.com = server:system\n",
		"# 节点与订阅域名（自动生成）",
		"sub.airport.com = server:https://dns.alidns.com/dns-query\n",
		"jp.example.com = server:https://dns.alidns.com/dns-query\n",
	} {
		if !strings.Contains(host, want) {
			t.Errorf("missing %q in\n%s", want, host)
		}
	}
	if strings.Count(host, "hk.example.com") != 1 || strings.Contains(host, "1.2.3.4 =") {
		t.Errorf("duplicate or IP host entries:\n%s", host)
	}
	if !strings.Contains(sg.Content, "[Rule]\nDOMAIN,dns.alidns.com,DIRECT\n") {
		t.Error("DoH host should be DIRECT first")
	}
	st, _ := Generate(Request{Profile: &p, Nodes: nodes, Target: capability.Target{Client: capability.Stash, Platform: "ios"}})
	var doc struct {
		Hosts map[string]any `yaml:"hosts"`
		DNS   struct {
			Policy map[string]string `yaml:"nameserver-policy"`
		} `yaml:"dns"`
	}
	if err := yaml.Unmarshal([]byte(st.Content), &doc); err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(doc.Hosts["dns.alidns.com"]) != "[223.5.5.5 223.6.6.6]" || doc.DNS.Policy["jp.example.com"] != "https://dns.alidns.com/dns-query" {
		t.Errorf("stash hosts %v policy %v", doc.Hosts, doc.DNS.Policy)
	}
	for _, bad := range []string{"a.com,b.com", "1.2.3.4, b.com", "1.2.3.4 5.6.7.8"} {
		q := profile.Default(nil, 0)
		q.General.Hosts = []profile.Host{{Domain: "x.com", Value: bad}}
		if q.Validate() == nil {
			t.Errorf("%q should be rejected", bad)
		}
	}
}

func TestDefaultGeneral(t *testing.T) {
	p := profile.Default(nil, 0)
	if err := p.Validate(); err != nil {
		t.Fatal(err)
	}
	nodes := loadNodes(t)
	mac, _ := Generate(Request{Profile: &p, Nodes: nodes, Target: capability.Target{Client: capability.Surge, Platform: "mac"}})
	for _, want := range []string{
		"proxy-test-url = https://cp.cloudflare.com/generate_204\n",
		"dns-server = 223.5.5.5, 119.29.29.29\n",
		"skip-proxy = 127.0.0.1, 192.168.0.0/16, 10.0.0.0/8, 172.16.0.0/12, 100.64.0.0/10, 17.0.0.0/8, localhost, *.local, captive.apple.com",
		"hijack-dns = *:53\n",
		"encrypted-dns-follow-outbound-mode = true\n",
		"# 加密 DNS 服务器固定地址，连接它们无需先做明文 DNS 查询\ndns.alidns.com = 223.5.5.5, 223.6.6.6\n",
		"*.cn = server:https://dns.alidns.com/dns-query\n",
		"RULE-SET:https://raw.githubusercontent.com/ACL4SSR/ACL4SSR/master/Clash/ChinaDomain.list = server:https://dns.alidns.com/dns-query\n",
	} {
		if !strings.Contains(mac.Content, want) {
			t.Errorf("surge mac missing %q", want)
		}
	}
	old, _ := Generate(Request{Profile: &p, Nodes: nodes, Target: capability.Target{Client: capability.Surge, Platform: "mac", Version: "5.9.0"}})
	if strings.Contains(old.Content, "RULE-SET:https") || !hasWarning(old.Warnings, "RULE-SET:https://raw.githubusercontent.com/ACL4SSR/ACL4SSR/master/Clash/ChinaDomain.list", "5.10.0") {
		t.Error("rule-set host entries need Surge Mac 5.10.0")
	}
	st, _ := Generate(Request{Profile: &p, Nodes: nodes, Target: capability.Target{Client: capability.Stash, Platform: "ios"}})
	if !strings.Contains(st.Content, "https://cp.cloudflare.com/generate_204") {
		t.Error("stash test url")
	}
}

func TestServerDirectAllSources(t *testing.T) {
	p := profile.Default(nil, 0)
	p.Sources = []string{"mine"}
	nodes := []*node.Node{
		{Name: "A", Type: node.SS, Server: "a.example.com", Port: 1, Cipher: "aes-128-gcm", Password: "x", Source: "mine"},
		{Name: "B", Type: node.SS, Server: "b.other.com", Port: 1, Cipher: "aes-128-gcm", Password: "x", Source: "other"},
		{Name: "流量：1G / 2G", Type: node.SS, Server: "1.1.1.1", Port: 1, Cipher: "aes-128-gcm", Password: "x", Source: "other"},
		{Name: "LAN", Type: node.SS, Server: "192.168.1.2", Port: 1, Cipher: "aes-128-gcm", Password: "x", Source: "other"},
	}
	res, err := Generate(Request{Profile: &p, Nodes: nodes, Target: capability.Target{Client: capability.Surge}})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(res.Content, "B = ss") {
		t.Error("node of another source must not be emitted")
	}
	for _, want := range []string{"DOMAIN,a.example.com,DIRECT", "DOMAIN,b.other.com,DIRECT"} {
		if !strings.Contains(res.Content, want) {
			t.Errorf("missing %s", want)
		}
	}
	for _, bad := range []string{"1.1.1.1/32", "192.168.1.2/32"} {
		if strings.Contains(res.Content, bad) {
			t.Errorf("unexpected DIRECT rule for %s", bad)
		}
	}
}

func TestDefaultDoHRules(t *testing.T) {
	withHK := profile.Default(map[string]int{"HK": 3, "JP": 2}, 0)
	noHK := profile.Default(map[string]int{"JP": 2}, 0)
	for _, c := range []struct {
		p    profile.Profile
		want string
	}{{withHK, node.RegionName("HK")}, {noHK, profile.GlobalGroup}} {
		if err := c.p.Validate(); err != nil {
			t.Fatal(err)
		}
		got := map[string]string{}
		for _, r := range c.p.Rules {
			if r.Type == "DOMAIN" {
				got[r.Value] = r.Policy
			}
		}
		if got["cloudflare-dns.com"] != c.want || got["dns.google"] != c.want {
			t.Errorf("want %s, got %v", c.want, got)
		}
	}
	sg, err := Generate(Request{Profile: &withHK, Nodes: loadNodes(t), Target: capability.Target{Client: capability.Surge}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sg.Content, "# 指向自动切换的稳定组\nDOMAIN,cloudflare-dns.com,香港\nDOMAIN,dns.google,香港\n") {
		t.Error("DoH rules missing from Surge output")
	}
}

func TestDefaultRuleSets(t *testing.T) {
	policyOf := func(p profile.Profile) map[string]string {
		m := map[string]string{}
		for _, r := range p.Rules {
			if r.Type == "RULE-SET" {
				m[r.Value] = r.Policy
			}
		}
		return m
	}
	p := profile.Default(map[string]int{"HK": 3, "JP": 2, "US": 1}, 0)
	if err := p.Validate(); err != nil {
		t.Fatal(err)
	}
	got := policyOf(p)
	want := map[string]string{
		"BanAD": "REJECT", "BanProgramAD": "REJECT", "Apple": "DIRECT", "AI": node.RegionName("US"),
		"Twitter": profile.GlobalGroup, "ChinaIp": "DIRECT", "Download": "DIRECT",
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s: got %q, want %q", k, got[k], v)
		}
	}
	if ai := policyOf(profile.Default(map[string]int{"HK": 3, "JP": 2}, 0))["AI"]; ai != node.RegionName("JP") {
		t.Errorf("AI without US nodes: %s", ai)
	}
	if ai := policyOf(profile.Default(map[string]int{"HK": 3}, 0))["AI"]; ai != profile.GlobalGroup {
		t.Errorf("AI with only HK nodes: %s", ai)
	}
}
