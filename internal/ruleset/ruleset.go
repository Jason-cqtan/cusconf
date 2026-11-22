// Package ruleset parses remote rule lists (Surge / Clash / domain-set / plain
// CIDR lists) and converts them between client formats, for the rule set mirror.
package ruleset

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"net/netip"
	"regexp"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// Entry is one rule of a list, without policy.
type Entry struct {
	Type      string // DOMAIN, DOMAIN-SUFFIX, DOMAIN-KEYWORD, IP-CIDR, IP-CIDR6, PROCESS-NAME, …
	Value     string
	NoResolve bool
}

// Formats the mirror can serve.
const (
	List = "list" // Surge / Loon: classical RULE-SET lines, or a DOMAIN-SET for domain behavior
	Text = "txt"  // Clash rule-provider, format: text
	YAML = "yaml" // Clash rule-provider, format: yaml
	JSON = "json" // sing-box rule set, format: source
	QX   = "qx"   // Quantumult X filter_remote (used with force-policy)
	Loon = "loon" // classical lines for any behavior (Loon has no domain-set)
)

func ValidFormat(f string) bool {
	switch f {
	case List, Text, YAML, JSON, QX, Loon:
		return true
	}
	return false
}

var typeAlias = map[string]string{
	"HOST": "DOMAIN", "HOST-SUFFIX": "DOMAIN-SUFFIX", "HOST-KEYWORD": "DOMAIN-KEYWORD", "HOST-WILDCARD": "DOMAIN-WILDCARD",
	"IP6-CIDR": "IP-CIDR6", "IP-ASN": "IP-ASN", "DST-PORT": "DST-PORT", "DEST-PORT": "DST-PORT",
}

// Parse reads a rule list. behavior (classical | domain | ipcidr) tells how
// bare lines without a type are read.
func Parse(body []byte, behavior string) []Entry {
	lines := payload(body)
	if lines == nil {
		sc := bufio.NewScanner(bytes.NewReader(body))
		sc.Buffer(make([]byte, 1<<20), 1<<20)
		for sc.Scan() {
			lines = append(lines, sc.Text())
		}
	}
	var out []Entry
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if l == "" || strings.HasPrefix(l, "#") || strings.HasPrefix(l, "//") || strings.HasPrefix(l, ";") {
			continue
		}
		l = strings.Trim(l, `'"`)
		if e, ok := parseLine(l, behavior); ok {
			out = append(out, e)
		}
	}
	return out
}

// payload returns the entries of a Clash YAML provider ("payload: [...]"), or nil.
func payload(body []byte) []string {
	if !bytes.Contains(body, []byte("payload:")) {
		return nil
	}
	var doc struct {
		Payload []string `yaml:"payload"`
	}
	if err := yaml.Unmarshal(body, &doc); err != nil {
		return nil
	}
	if doc.Payload == nil {
		return []string{}
	}
	return doc.Payload
}

func parseLine(l, behavior string) (Entry, bool) {
	if i := strings.Index(l, " #"); i > 0 {
		l = strings.TrimSpace(l[:i])
	}
	if strings.Contains(l, ",") && behavior != "domain" && behavior != "ipcidr" {
		parts := strings.Split(l, ",")
		for i := range parts {
			parts[i] = strings.TrimSpace(parts[i])
		}
		t := strings.ToUpper(parts[0])
		if a, ok := typeAlias[t]; ok {
			t = a
		}
		if len(parts) < 2 || parts[1] == "" {
			return Entry{}, false
		}
		e := Entry{Type: t, Value: parts[1]}
		for _, p := range parts[2:] {
			if strings.EqualFold(p, "no-resolve") {
				e.NoResolve = true
			}
		}
		return e, true
	}
	// Bare entry.
	if p, err := netip.ParsePrefix(l); err == nil {
		return cidr(p), true
	}
	if a, err := netip.ParseAddr(l); err == nil {
		return cidr(netip.PrefixFrom(a, a.BitLen())), true
	}
	if behavior == "ipcidr" {
		return Entry{}, false
	}
	switch {
	case strings.HasPrefix(l, "+."):
		return Entry{Type: "DOMAIN-SUFFIX", Value: l[2:]}, true
	case strings.HasPrefix(l, "*."):
		// Clash "*.x" is one label; the closest portable form is a suffix.
		return Entry{Type: "DOMAIN-SUFFIX", Value: l[2:]}, true
	case strings.HasPrefix(l, "."):
		return Entry{Type: "DOMAIN-SUFFIX", Value: l[1:]}, true
	case strings.Contains(l, "*"):
		return Entry{Type: "DOMAIN-WILDCARD", Value: l}, true
	}
	return Entry{Type: "DOMAIN", Value: l}, true
}

func cidr(p netip.Prefix) Entry {
	p = p.Masked()
	if p.Addr().Is4() {
		return Entry{Type: "IP-CIDR", Value: p.String(), NoResolve: true}
	}
	return Entry{Type: "IP-CIDR6", Value: p.String(), NoResolve: true}
}

// Format writes entries in a client format. Entries the format cannot express are
// dropped; skipped counts them.
func Format(entries []Entry, behavior, format string) (out []byte, skipped int) {
	var b strings.Builder
	switch format {
	case Loon:
		for _, e := range entries {
			b.WriteString(classical(e) + "\n")
		}
	case List:
		for _, e := range entries {
			if behavior == "domain" {
				switch e.Type {
				case "DOMAIN":
					b.WriteString(e.Value + "\n")
				case "DOMAIN-SUFFIX":
					b.WriteString("." + e.Value + "\n")
				default:
					skipped++
				}
				continue
			}
			b.WriteString(classical(e) + "\n")
		}
	case Text, YAML:
		var lines []string
		for _, e := range entries {
			switch behavior {
			case "domain":
				switch e.Type {
				case "DOMAIN":
					lines = append(lines, e.Value)
				case "DOMAIN-SUFFIX":
					lines = append(lines, "+."+e.Value)
				default:
					skipped++
				}
			case "ipcidr":
				if e.Type == "IP-CIDR" || e.Type == "IP-CIDR6" {
					lines = append(lines, e.Value)
				} else {
					skipped++
				}
			default:
				lines = append(lines, classical(e))
			}
		}
		if format == Text {
			for _, l := range lines {
				b.WriteString(l + "\n")
			}
		} else {
			b.WriteString("payload:\n")
			for _, l := range lines {
				b.WriteString("  - " + strconv.Quote(l) + "\n")
			}
		}
	case JSON:
		return singbox(entries)
	case QX:
		for _, e := range entries {
			t, ok := qxType[e.Type]
			if !ok {
				skipped++
				continue
			}
			fmt.Fprintf(&b, "%s, %s, proxy\n", t, e.Value)
		}
	}
	return []byte(b.String()), skipped
}

func classical(e Entry) string {
	s := e.Type + "," + e.Value
	if e.NoResolve && (e.Type == "IP-CIDR" || e.Type == "IP-CIDR6" || e.Type == "GEOIP" || e.Type == "IP-ASN") {
		s += ",no-resolve"
	}
	return s
}

var qxType = map[string]string{
	"DOMAIN": "host", "DOMAIN-SUFFIX": "host-suffix", "DOMAIN-KEYWORD": "host-keyword", "DOMAIN-WILDCARD": "host-wildcard",
	"IP-CIDR": "ip-cidr", "IP-CIDR6": "ip6-cidr", "GEOIP": "geoip", "USER-AGENT": "user-agent",
}

// singbox writes a sing-box source rule set (version 2). Fields of different kinds
// are ANDed inside one sing-box rule, so each kind gets its own rule.
func singbox(entries []Entry) ([]byte, int) {
	field := map[string]string{
		"DOMAIN": "domain", "DOMAIN-SUFFIX": "domain_suffix", "DOMAIN-KEYWORD": "domain_keyword", "DOMAIN-REGEX": "domain_regex",
		"IP-CIDR": "ip_cidr", "IP-CIDR6": "ip_cidr", "SRC-IP-CIDR": "source_ip_cidr",
		"PROCESS-NAME": "process_name", "PROCESS-PATH": "process_path", "DST-PORT": "port", "SRC-PORT": "source_port",
	}
	group := map[string]string{
		"domain": "domain", "domain_suffix": "domain", "domain_keyword": "domain", "domain_regex": "domain",
		"ip_cidr": "ip", "source_ip_cidr": "source_ip", "process_name": "process", "process_path": "process",
		"port": "port", "source_port": "source_port",
	}
	order := []string{"domain", "ip", "source_ip", "process", "port", "source_port"}
	rules := map[string]map[string][]any{}
	skipped := 0
	for _, e := range entries {
		f, ok := field[e.Type]
		if e.Type == "DOMAIN-WILDCARD" {
			f, ok = "domain_regex", true
		}
		if !ok {
			skipped++
			continue
		}
		var v any = e.Value
		if e.Type == "DOMAIN-WILDCARD" {
			f, v = "domain_regex", wildcardRegex(e.Value)
		}
		if f == "port" || f == "source_port" {
			n, err := strconv.Atoi(e.Value)
			if err != nil {
				skipped++ // port ranges are not mapped
				continue
			}
			v = n
		}
		g := group[f]
		if rules[g] == nil {
			rules[g] = map[string][]any{}
		}
		rules[g][f] = append(rules[g][f], v)
	}
	out := struct {
		Version int              `json:"version"`
		Rules   []map[string]any `json:"rules"`
	}{Version: 2, Rules: []map[string]any{}}
	for _, g := range order {
		if r := rules[g]; r != nil {
			m := map[string]any{}
			for k, v := range r {
				m[k] = v
			}
			out.Rules = append(out.Rules, m)
		}
	}
	b, _ := json.MarshalIndent(out, "", "  ")
	return b, skipped
}

func wildcardRegex(p string) string {
	var b strings.Builder
	b.WriteString("^")
	for _, r := range p {
		switch r {
		case '*':
			b.WriteString(".*")
		case '?':
			b.WriteString(".")
		default:
			b.WriteString(regexp.QuoteMeta(string(r)))
		}
	}
	b.WriteString("$")
	return b.String()
}
