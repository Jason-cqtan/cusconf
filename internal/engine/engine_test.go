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
	if !strings.Contains(newer.Content, "♻️ 自动选择 = smart,") {
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
	if !strings.Contains(older.Content, "♻️ 自动选择 = url-test,") || !hasWarning(older.Warnings, "♻️ 自动选择", "using url-test") {
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
	if doc.Rules[len(doc.Rules)-1] != "MATCH,🐟 漏网之鱼" {
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
		if strings.HasPrefix(line, "🇭🇰 香港 = url-test") {
			if !strings.Contains(line, "🇭🇰 HK 01") || !strings.Contains(line, "HK 02") {
				t.Errorf("hk group: %s", line)
			}
			return
		}
	}
	t.Error("hk group not found")
}
