package ruleset

import (
	"encoding/json"
	"strings"
	"testing"
)

const surgeList = `# comment
DOMAIN,a.com
DOMAIN-SUFFIX,b.com,no-resolve
DOMAIN-KEYWORD,goog
DOMAIN-WILDCARD,*.cdn?.c.com
IP-CIDR,10.0.0.0/8,no-resolve
IP-CIDR6,2001:db8::/32
PROCESS-NAME,Telegram
USER-AGENT,Foo*
`

func TestParseFormats(t *testing.T) {
	es := Parse([]byte(surgeList), "classical")
	if len(es) != 8 {
		t.Fatalf("got %d entries: %+v", len(es), es)
	}
	if es[1].Type != "DOMAIN-SUFFIX" || es[1].NoResolve != true || es[4].Value != "10.0.0.0/8" {
		t.Errorf("bad parse: %+v", es)
	}

	list, _ := Format(es, "classical", List)
	if !strings.Contains(string(list), "IP-CIDR,10.0.0.0/8,no-resolve\n") || !strings.Contains(string(list), "DOMAIN-SUFFIX,b.com\n") {
		t.Errorf("list:\n%s", list)
	}

	qx, skipped := Format(es, "classical", QX)
	if !strings.Contains(string(qx), "host-suffix, b.com, proxy\n") || !strings.Contains(string(qx), "ip6-cidr, 2001:db8::/32, proxy\n") || skipped != 1 {
		t.Errorf("qx (skipped %d):\n%s", skipped, qx)
	}

	js, skipped := Format(es, "classical", JSON)
	var rs struct {
		Version int              `json:"version"`
		Rules   []map[string]any `json:"rules"`
	}
	if err := json.Unmarshal(js, &rs); err != nil {
		t.Fatal(err)
	}
	// domain kinds share one rule; IPs and processes get their own (sing-box ANDs different kinds).
	if rs.Version != 2 || len(rs.Rules) != 3 || skipped != 1 {
		t.Fatalf("json (skipped %d): %s", skipped, js)
	}
	if rs.Rules[0]["domain_regex"].([]any)[0] != `^.*\.cdn.\.c\.com$` {
		t.Errorf("wildcard regex: %v", rs.Rules[0]["domain_regex"])
	}
}

func TestDomainAndYAML(t *testing.T) {
	yamlProvider := "payload:\n  - '+.google.com'\n  - 'example.org'\n  - '.apple.com'\n"
	es := Parse([]byte(yamlProvider), "domain")
	if len(es) != 3 || es[0].Type != "DOMAIN-SUFFIX" || es[1].Type != "DOMAIN" || es[2].Value != "apple.com" {
		t.Fatalf("%+v", es)
	}
	set, _ := Format(es, "domain", List)
	if string(set) != ".google.com\nexample.org\n.apple.com\n" {
		t.Errorf("domain set:\n%s", set)
	}
	loon, _ := Format(es, "domain", Loon)
	if string(loon) != "DOMAIN-SUFFIX,google.com\nDOMAIN,example.org\nDOMAIN-SUFFIX,apple.com\n" {
		t.Errorf("loon:\n%s", loon)
	}
	txt, _ := Format(es, "domain", Text)
	if string(txt) != "+.google.com\nexample.org\n+.apple.com\n" {
		t.Errorf("clash text:\n%s", txt)
	}
	y, _ := Format(es, "domain", YAML)
	if !strings.HasPrefix(string(y), "payload:\n  - \"+.google.com\"\n") {
		t.Errorf("yaml:\n%s", y)
	}

	ips := Parse([]byte("1.2.3.0/24\n2001:db8::1\n"), "ipcidr")
	if len(ips) != 2 || ips[1].Value != "2001:db8::1/128" || ips[1].Type != "IP-CIDR6" {
		t.Errorf("%+v", ips)
	}
}
