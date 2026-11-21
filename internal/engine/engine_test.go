package engine

import (
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
	p := profile.Default()
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
	if !strings.Contains(newer.Content, "自动选择 = smart,") {
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
	if !strings.Contains(older.Content, "自动选择 = url-test,") || !hasWarning(older.Warnings, "自动选择", "using url-test") {
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
	if doc.RuleProviders["Netflix"]["behavior"] != "classical" {
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

func TestTemplateDirectOnlyInFinal(t *testing.T) {
	p := profile.Default()
	for _, g := range p.Groups {
		has := false
		for _, m := range g.Members {
			has = has || m == "DIRECT"
		}
		if has != (g.Name == "漏网之鱼") {
			t.Errorf("group %s: DIRECT member = %v", g.Name, has)
		}
	}
}

func TestHosts(t *testing.T) {
	p := profile.Default()
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
	bad := profile.Default()
	bad.General.Hosts = []profile.Host{{Domain: "a b", Value: "1.1.1.1"}}
	if err := bad.Validate(); err == nil {
		t.Error("expected validation error")
	}
}

func TestGeneralExtra(t *testing.T) {
	p := profile.Default()
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
