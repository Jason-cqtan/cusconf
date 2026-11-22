package engine

import (
	"encoding/json"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/cusconf/cusconf/internal/capability"
	"github.com/cusconf/cusconf/internal/render"
)

func fakeMirror(u, behavior, format string) string {
	return "https://conf.example.com/rs/" + behavior + "." + format
}

func genWith(t *testing.T, target capability.Target, mirror bool) render.Result {
	t.Helper()
	req := Request{Profile: fixtureNodes(t), Nodes: loadNodes(t), Target: target, ManagedURL: "https://conf.example.com/s/abc"}
	if mirror {
		req.Mirror = fakeMirror
	}
	res, err := Generate(req)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func TestClashMeta(t *testing.T) {
	for _, c := range []capability.Client{capability.ClashMeta, capability.FlClash} {
		res := genWith(t, capability.Target{Client: c}, false)
		var doc struct {
			DNS     map[string]any            `yaml:"dns"`
			Proxies []map[string]any          `yaml:"proxies"`
			Groups  []map[string]any          `yaml:"proxy-groups"`
			RP      map[string]map[string]any `yaml:"rule-providers"`
		}
		if err := yaml.Unmarshal([]byte(res.Content), &doc); err != nil {
			t.Fatalf("%s: %v", c, err)
		}
		if doc.DNS["enable"] != true || doc.DNS["enhanced-mode"] != "fake-ip" {
			t.Errorf("%s: dns %v", c, doc.DNS)
		}
		byName := map[string]map[string]any{}
		for _, p := range doc.Proxies {
			byName[p["name"].(string)] = p
		}
		if hy := byName["HY2"]; hy["password"] != "hypass" || hy["auth"] != nil {
			t.Errorf("%s: hysteria2 should use password: %v", c, hy)
		}
		if _, ok := byName["SSR Node"]; !ok {
			t.Errorf("%s: mihomo supports ssr", c)
		}
		if tu := byName["TUIC5"]; tu["version"] != nil {
			t.Errorf("%s: tuic version key is Stash-only", c)
		}
		if doc.Groups[0]["type"] != "url-test" || !hasWarning(res.Warnings, "全局代理", "smart") {
			t.Errorf("%s: smart should downgrade", c)
		}
		for name, p := range doc.RP {
			if p["type"] != "http" || !strings.HasPrefix(p["path"].(string), "./rules/") {
				t.Errorf("%s: provider %s: %v", c, name, p)
			}
		}
	}
	// Stash keeps its own dialect.
	st := genWith(t, capability.Target{Client: capability.Stash}, false)
	if !strings.Contains(st.Content, "auth: hypass") || strings.Contains(st.Content, "enhanced-mode") {
		t.Error("stash dialect changed")
	}
}

func TestSingBox(t *testing.T) {
	res := genWith(t, capability.Target{Client: capability.SingBox}, true)
	var doc struct {
		DNS       map[string]any   `json:"dns"`
		Outbounds []map[string]any `json:"outbounds"`
		Endpoints []map[string]any `json:"endpoints"`
		Route     struct {
			Rules   []map[string]any `json:"rules"`
			RuleSet []map[string]any `json:"rule_set"`
			Final   string           `json:"final"`
		} `json:"route"`
	}
	if err := json.Unmarshal([]byte(res.Content), &doc); err != nil {
		t.Fatalf("%v\n%s", err, res.Content)
	}
	tags := map[string]map[string]any{}
	for _, o := range doc.Outbounds {
		tags[o["tag"].(string)] = o
	}
	if tags["全局代理"]["type"] != "urltest" || tags["DIRECT"]["type"] != "direct" {
		t.Errorf("groups: %v", tags["全局代理"])
	}
	if r := tags["US Reality"]["tls"].(map[string]any)["reality"]; r == nil {
		t.Error("reality missing")
	}
	if _, ok := tags["SSR Node"]; ok || !hasWarning(res.Warnings, "SSR Node", "not supported") {
		t.Error("ssr should be dropped")
	}
	if len(doc.Endpoints) != 1 || doc.Endpoints[0]["type"] != "wireguard" {
		t.Errorf("wireguard endpoint: %v", doc.Endpoints)
	}
	if hy := tags["HY2"]; hy["server_ports"].([]any)[0] != "20000:30000" {
		t.Errorf("port hopping: %v", hy)
	}
	if len(doc.Route.RuleSet) == 0 || doc.Route.RuleSet[0]["format"] != "source" || !strings.HasSuffix(doc.Route.RuleSet[0]["url"].(string), ".json") {
		t.Errorf("rule sets should come from the mirror: %v", doc.Route.RuleSet)
	}
	if doc.Route.Final != "漏网之鱼" {
		t.Errorf("final: %s", doc.Route.Final)
	}
	foundGeo, foundReject := false, false
	for _, r := range doc.Route.Rules {
		if rs, ok := r["rule_set"].([]any); ok {
			for _, x := range rs {
				foundGeo = foundGeo || x == "geoip-cn"
			}
		}
		if r["action"] == "reject" {
			foundReject = true
		}
	}
	if !foundGeo || !foundReject {
		t.Errorf("GEOIP,CN -> geoip-cn rule set and REJECT -> reject action expected: %v", doc.Route.Rules)
	}

	// Without a mirror, lists without a sing-box URL are skipped with a warning.
	plain := genWith(t, capability.Target{Client: capability.SingBox}, false)
	if !hasWarning(plain.Warnings, "ProxyGFWlist", "mirror") {
		t.Errorf("expected mirror warning: %v", plain.Warnings)
	}
	// 1.11 uses legacy DNS servers.
	old := genWith(t, capability.Target{Client: capability.SingBox, Version: "1.11.0"}, true)
	if !strings.Contains(old.Content, `"address": "223.5.5.5"`) {
		t.Error("1.11 should use legacy dns server format")
	}
	if _, err := Generate(Request{Profile: fixtureNodes(t), Nodes: loadNodes(t), Target: capability.Target{Client: capability.SingBox, Version: "1.10.0"}}); err == nil {
		t.Error("1.10 should be rejected")
	}
}

func TestLoon(t *testing.T) {
	res := genWith(t, capability.Target{Client: capability.Loon}, false)
	for _, want := range []string{
		"US Reality = VLESS,us.example.com,443,",
		"public-key=\"SomePublicKeyBase64\"",
		"HY2 = Hysteria2,hy.example.com,443,\"hypass\",salamander-password=ob",
		"全局代理 = url,",
		"[Remote Rule]\nhttps://raw.githubusercontent.com/ACL4SSR/ACL4SSR/master/Clash/LocalAreaNetwork.list, policy=DIRECT, tag=LocalAreaNetwork",
		"\nFINAL,漏网之鱼\n",
		"# Subscription: https://conf.example.com/s/abc",
	} {
		if !strings.Contains(res.Content, want) {
			t.Errorf("missing %q", want)
		}
	}
	if strings.Contains(res.Content, "TUIC5 =") || !hasWarning(res.Warnings, "TUIC5", "not supported") {
		t.Error("tuic should be dropped")
	}
	// With the mirror on for all lists, Loon loads converted copies.
	p := fixtureNodes(t)
	p.General.MirrorRules = true
	m, _ := Generate(Request{Profile: p, Nodes: loadNodes(t), Target: capability.Target{Client: capability.Loon}, Mirror: fakeMirror})
	if !strings.Contains(m.Content, "https://conf.example.com/rs/classical.loon, policy=DIRECT") {
		t.Errorf("mirror not used:\n%s", m.Content)
	}
}

func TestQX(t *testing.T) {
	res := genWith(t, capability.Target{Client: capability.QX}, true)
	for _, want := range []string{
		"vmess=jp.example.com:443, method=chacha20-ietf-poly1305, password=b831b1f8-5a5e-4b57-a03e-1234567890ab, obfs=wss, obfs-host=jp.example.com, obfs-uri=/ws, tag=JP VMess",
		"trojan=tw.example.com:443, password=p@ss, over-tls=true, tls-host=tw.example.com, tls-verification=false, tag=TW Trojan",
		"url-latency-benchmark=全局代理, ",
		"https://conf.example.com/rs/classical.qx, tag=ProxyGFWlist, force-policy=全局代理",
		"geoip, cn, direct",
		"\nfinal, 漏网之鱼\n",
	} {
		if !strings.Contains(res.Content, want) {
			t.Errorf("missing %q", want)
		}
	}
	if strings.Contains(res.Content, "tag=HY2") || !hasWarning(res.Warnings, "HY2", "not supported") {
		t.Error("hysteria2 should be dropped")
	}
	noMirror := genWith(t, capability.Target{Client: capability.QX}, false)
	if !hasWarning(noMirror.Warnings, "ProxyGFWlist", "mirror") {
		t.Error("expected mirror warning")
	}
}

func TestDefaultPlatform(t *testing.T) {
	for c, want := range map[capability.Client]string{capability.Surge: "ios", capability.ClashMeta: "core", capability.SingBox: "all"} {
		tg := capability.Target{Client: c}
		if err := capability.Default.Validate(&tg); err != nil || tg.Platform != want {
			t.Errorf("%s: %v %s", c, err, tg.Platform)
		}
	}
}

func TestMirrorLocal(t *testing.T) {
	p := fixtureNodes(t)
	p.General.MirrorRules = true
	req := Request{Profile: p, Nodes: loadNodes(t), Target: capability.Target{Client: capability.Stash}, Mirror: fakeMirror, MirrorLocal: true}
	res, err := Generate(req)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(res.Content, "conf.example.com/rs/") || !strings.Contains(res.Content, "raw.githubusercontent.com") {
		t.Error("local mirror URLs should fall back to the original URLs")
	}
	if !hasWarning(res.Warnings, "规则集镜像", "localhost") {
		t.Errorf("expected warning: %v", res.Warnings)
	}
	req.Target = capability.Target{Client: capability.SingBox}
	sb, _ := Generate(req)
	if !hasWarning(sb.Warnings, "规则集镜像", "will not load on other devices") {
		t.Errorf("sing-box should warn: %v", sb.Warnings)
	}
	req.MirrorLocal = false
	req.Target = capability.Target{Client: capability.Stash}
	pub, _ := Generate(req)
	if !strings.Contains(pub.Content, "conf.example.com/rs/") {
		t.Error("public mirror URLs should be used")
	}
}
