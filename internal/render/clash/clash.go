// Package clash renders Clash-style YAML configs: Stash, and the mihomo
// (Clash Meta) dialect used by Clash Meta clients and FlClash.
package clash

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/cusconf/cusconf/internal/node"
	"github.com/cusconf/cusconf/internal/profile"
	"github.com/cusconf/cusconf/internal/render"
)

// om is an insertion-ordered YAML mapping.
type om []kv
type kv struct {
	k string
	v any
}

// set appends k unless v is a zero value.
func (o *om) set(k string, v any) {
	switch x := v.(type) {
	case nil:
		return
	case string:
		if x == "" {
			return
		}
	case int:
		if x == 0 {
			return
		}
	case bool:
		if !x {
			return
		}
	case []string:
		if len(x) == 0 {
			return
		}
	case om:
		if len(x) == 0 {
			return
		}
	case map[string]string:
		if len(x) == 0 {
			return
		}
	}
	*o = append(*o, kv{k, v})
}

// force appends k even when v is a zero value.
func (o *om) force(k string, v any) { *o = append(*o, kv{k, v}) }

func (o om) MarshalYAML() (any, error) {
	n := &yaml.Node{Kind: yaml.MappingNode}
	for _, e := range o {
		vn, ok := e.v.(*yaml.Node) // used as is, so its comments survive
		if !ok {
			vn = &yaml.Node{}
			if err := vn.Encode(e.v); err != nil {
				return nil, err
			}
		}
		n.Content = append(n.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: e.k}, vn)
	}
	return n, nil
}

// dialect returns the client name for messages and the extra-config key.
func dialect(c *render.Config) (label, extraKey string) {
	if c.Target.Client.Mihomo() {
		return c.Matrix.Clients[c.Target.Client].Label, "mihomo"
	}
	return "Stash", "stash"
}

func Render(c *render.Config) render.Result {
	var w render.Warnings
	root := om{}
	g := c.General
	meta := c.Target.Client.Mihomo()
	label, extraKey := dialect(c)

	root.force("mixed-port", 7890)
	root.force("allow-lan", false)
	root.force("mode", "rule")
	root.force("log-level", "info")
	root.force("ipv6", g.IPv6)
	if meta {
		root.force("unified-delay", true)
		root.force("tcp-concurrent", true)
		root.force("profile", om{{"store-selected", true}, {"store-fake-ip", true}})
	}
	dns := g.DNS
	var ns []string
	for _, d := range dns {
		if d != "system" {
			ns = append(ns, d)
		}
	}
	if len(ns) == 0 {
		ns = []string{"223.5.5.5", "119.29.29.29"}
	}
	dnsCfg := om{}
	if meta {
		var plain []string // default-nameserver must be IP addresses
		for _, d := range ns {
			if !strings.Contains(d, "://") {
				plain = append(plain, d)
			}
		}
		if len(plain) == 0 {
			plain = []string{"223.5.5.5", "119.29.29.29"}
		}
		dnsCfg = om{{"enable", true}, {"ipv6", g.IPv6}, {"enhanced-mode", "fake-ip"}, {"fake-ip-range", "198.18.0.1/16"},
			{"fake-ip-filter", []string{"*.lan", "*.local", "localhost.ptlogin2.qq.com", "+.msftconnecttest.com", "+.msftncsi.com"}},
			{"default-nameserver", plain}, {"nameserver", ns}}
	} else {
		dnsCfg = om{{"default-nameserver", ns}, {"nameserver", ns}}
	}
	hosts, policy := convertHosts(g.Hosts, meta, label, &w)
	if len(policy) > 0 {
		dnsCfg.force("nameserver-policy", policy)
	}
	root.force("dns", dnsCfg)
	if len(hosts) > 0 {
		root.force("hosts", hosts)
	}

	var proxies []om
	kept := map[string]bool{}
	for _, n := range c.Nodes {
		p, err := proxy(c, n)
		if err != nil {
			w.Add("node", n.Name, "%v", err)
			continue
		}
		kept[n.Name] = true
		proxies = append(proxies, p)
	}
	root.force("proxies", proxies)

	groups := render.FinalizeGroups(c.Groups, kept, &w)
	var pgs []om
	for i := range groups {
		pgs = append(pgs, group(c, &groups[i], &w))
	}
	root.force("proxy-groups", pgs)

	providers := om{}
	var rules []string
	var comments []string // per rule, YAML head comment
	hasFinal := false
	used := map[string]bool{}
	for _, r := range c.Rules {
		n := len(rules)
		switch r.Type {
		case "FINAL", "MATCH":
			hasFinal = true
			rules = append(rules, "MATCH,"+r.Policy)
		case "RULE-SET":
			rs := c.RuleSets[r.Value]
			u, mirrored := c.RuleSetURL(rs, rs.StashURL, "txt")
			if u == "" {
				w.Add("rule", r.Value, "rule set has no Clash/Stash URL; skipped")
				continue
			}
			format := or(rs.StashFormat, "yaml")
			if mirrored {
				format = "text"
			}
			if !used[rs.Name] {
				used[rs.Name] = true
				pv := om{}
				if meta {
					pv.force("type", "http")
				}
				pv.force("behavior", or(rs.Behavior, "classical"))
				pv.force("format", format)
				pv.force("url", u)
				if meta {
					pv.force("path", "./rules/"+safeFile(rs.Name)+"."+map[string]string{"text": "txt", "yaml": "yaml", "mrs": "mrs"}[format])
				}
				pv.force("interval", or0(rs.Interval, 86400))
				providers.force(rs.Name, pv)
			}
			line := "RULE-SET," + rs.Name + "," + r.Policy
			if rs.Behavior == "ipcidr" || r.NoResolve {
				line += ",no-resolve"
			}
			rules = append(rules, line)
		case "PROTOCOL":
			t, v, ok := render.ProtocolRule(c, r.Value, &w)
			if !ok {
				continue
			}
			rules = append(rules, fmt.Sprintf("%s,%s,%s", t, v, r.Policy))
		default:
			line := fmt.Sprintf("%s,%s,%s", r.Type, r.Value, r.Policy)
			if r.NoResolve {
				line += ",no-resolve"
			}
			rules = append(rules, line)
		}
		if len(rules) > n {
			comments = append(comments, headComment(r.CommentLines()))
		}
	}
	if !hasFinal {
		final := "DIRECT"
		if len(groups) > 0 {
			final = groups[0].Name
		}
		w.Add("rule", "MATCH", "no final rule; added MATCH,%s", final)
		rules = append(rules, "MATCH,"+final)
		comments = append(comments, "")
	}
	if len(providers) > 0 {
		root.force("rule-providers", providers)
	}
	root.force("rules", ruleList(rules, comments))

	if extra := strings.TrimSpace(g.Extra[extraKey]); extra != "" {
		if err := mergeExtra(&root, extra); err != nil {
			w.Add("general", label+" 追加 YAML", "ignored: %v", err)
		}
	}

	var b strings.Builder
	if meta && c.ManagedURL != "" {
		// Clash Meta for Android / Verge show this as the profile's update URL.
		fmt.Fprintf(&b, "# Subscription: %s\n", c.ManagedURL)
	}
	fmt.Fprintf(&b, "# Generated by cusconf for %s\n", c.Target)
	enc := yaml.NewEncoder(&b)
	enc.SetIndent(2)
	if err := enc.Encode(root); err != nil {
		w.Add("general", "yaml", "%v", err)
	}
	enc.Close()
	content := unescapeUnicode(b.String())
	b.Reset()
	b.WriteString(content)
	return render.Result{Content: b.String(), ContentType: "text/yaml; charset=utf-8", Warnings: w}
}

var unsafeFile = regexp.MustCompile(`[^\p{L}\p{N}._-]+`)

func safeFile(s string) string { return unsafeFile.ReplaceAllString(s, "_") }

func or(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func or0(a, b int) int {
	if a > 0 {
		return a
	}
	return b
}

func proxy(c *render.Config, n *node.Node) (om, error) {
	feat := "proxy." + string(n.Type)
	if n.Type == node.TUIC && n.Version == 5 {
		feat = "proxy.tuic-v5"
	}
	if !c.Has(feat) {
		return nil, fmt.Errorf("%s %s", n.Type, c.Req(feat))
	}
	meta := c.Target.Client.Mihomo()
	label, _ := dialect(c)
	p := om{}
	p.force("name", n.Name)
	p.force("type", string(n.Type))
	p.force("server", n.Server)
	p.force("port", n.Port)
	if n.UDP != nil {
		p.force("udp", *n.UDP)
	}
	if n.TFO && n.Type != node.Hysteria2 {
		p.set("tfo", true)
	}

	switch n.Type {
	case node.SS:
		p.force("cipher", n.Cipher)
		p.force("password", n.Password)
		if n.Plugin != "" {
			switch n.Plugin {
			case "obfs":
				p.force("plugin", "obfs")
				p.force("plugin-opts", om{{"mode", n.PluginOpts["mode"]}, {"host", n.PluginOpts["host"]}})
			case "v2ray-plugin":
				po := om{{"mode", "websocket"}}
				po.set("tls", n.PluginOpts["tls"] == "true")
				po.set("host", n.PluginOpts["host"])
				po.set("path", n.PluginOpts["path"])
				p.force("plugin", "v2ray-plugin")
				p.force("plugin-opts", po)
			case "shadow-tls":
				v := or(n.PluginOpts["version"], "2")
				if !c.Has("shadow-tls.v" + v) {
					return nil, fmt.Errorf("shadow-tls v%s %s", v, c.Req("shadow-tls.v"+v))
				}
				p.force("plugin", "shadow-tls")
				p.force("plugin-opts", om{{"password", n.PluginOpts["password"]}, {"host", n.PluginOpts["host"]}, {"version", atoi(v)}})
			default:
				return nil, fmt.Errorf("ss plugin %q not supported by %s", n.Plugin, label)
			}
		}
	case node.SSR:
		p.force("cipher", n.Cipher)
		p.force("password", n.Password)
		p.force("obfs", n.Obfs)
		p.force("protocol", n.Protocol)
		p.set("obfs-param", n.ObfsParam)
		p.set("protocol-param", n.ProtocolParam)
	case node.VMess:
		p.force("uuid", n.UUID)
		p.force("alterId", n.AlterID)
		p.force("cipher", or(n.Cipher, "auto"))
		if err := tlsAndTransport(c, n, &p); err != nil {
			return nil, err
		}
	case node.VLESS:
		p.force("uuid", n.UUID)
		p.set("flow", n.Flow)
		if n.Cipher != "" {
			if !c.Has("vless.encryption") {
				return nil, fmt.Errorf("VLESS encryption %s", c.Req("vless.encryption"))
			}
			p.set("encryption", n.Cipher)
		}
		if err := tlsAndTransport(c, n, &p); err != nil {
			return nil, err
		}
	case node.Trojan:
		p.force("password", n.Password)
		if err := tlsAndTransport(c, n, &p); err != nil {
			return nil, err
		}
	case node.Hysteria2:
		if meta {
			p.force("password", n.Password)
		} else {
			p.force("auth", n.Password)
		}
		if n.Obfs != "" {
			if !c.Has("hysteria2." + n.Obfs) {
				return nil, fmt.Errorf("hysteria2 %s obfs %s", n.Obfs, c.Req("hysteria2."+n.Obfs))
			}
			p.force("obfs", n.Obfs)
			p.force("obfs-password", n.ObfsPassword)
		}
		if meta {
			p.set("up", mbps(n.UpMbps))
			p.set("down", mbps(n.DownMbps))
		} else {
			p.set("up-speed", n.UpMbps)
			p.set("down-speed", n.DownMbps)
			p.set("fast-open", n.TFO)
		}
		portHopping(n, &p)
		tlsFields(n, &p, false)
	case node.Hysteria:
		p.set("auth-str", n.Password)
		p.set("protocol", n.Protocol)
		p.set("obfs", n.Obfs)
		if meta {
			p.force("up", mbps(or0(n.UpMbps, 50)))
			p.force("down", mbps(or0(n.DownMbps, 200)))
		} else {
			p.force("up-speed", or0(n.UpMbps, 50))
			p.force("down-speed", or0(n.DownMbps, 200))
		}
		portHopping(n, &p)
		tlsFields(n, &p, false)
	case node.TUIC:
		if !meta { // mihomo tells v4 / v5 apart by token vs uuid+password
			p.force("version", or0(n.Version, 5))
		}
		if n.Version == 4 {
			p.force("token", n.Token)
		} else {
			p.force("uuid", n.UUID)
			p.force("password", n.Password)
		}
		p.set("congestion-controller", n.CongestionControl)
		if meta {
			p.set("udp-relay-mode", n.UDPRelayMode)
		}
		portHopping(n, &p)
		tlsFields(n, &p, false)
	case node.WireGuard:
		wg := n.WireGuard
		peer := wg.Peers[0]
		if len(wg.Peers) > 1 {
			return nil, fmt.Errorf("multi-peer wireguard not supported by %s", label)
		}
		p.set("ip", wg.IP)
		p.set("ipv6", wg.IPv6)
		p.force("private-key", wg.PrivateKey)
		p.force("public-key", peer.PublicKey)
		if meta {
			p.set("pre-shared-key", peer.PresharedKey)
			p.set("allowed-ips", peer.AllowedIPs)
		} else {
			p.set("preshared-key", peer.PresharedKey)
		}
		p.set("dns", wg.DNS)
		p.set("mtu", wg.MTU)
		p.set("keepalive", peer.Keepalive)
		if len(peer.Reserved) == 3 {
			p.force("reserved", peer.Reserved)
		}
	case node.Snell:
		p.force("psk", n.Password)
		p.set("version", n.Version)
		if n.Obfs != "" {
			p.force("obfs-opts", om{{"mode", n.Obfs}, {"host", or(n.ObfsHost, "bing.com")}})
		}
	case node.AnyTLS:
		p.force("password", n.Password)
		tlsFields(n, &p, false)
	case node.HTTP, node.SOCKS5:
		p.set("username", n.Username)
		p.set("password", n.Password)
		if n.TLSEnabled() {
			p.force("tls", true)
			tlsFields(n, &p, false)
		}
	default:
		return nil, fmt.Errorf("%s not supported by %s", n.Type, label)
	}
	return p, nil
}

func mbps(v int) string {
	if v <= 0 {
		return ""
	}
	return fmt.Sprintf("%d Mbps", v)
}

func atoi(s string) int {
	var i int
	fmt.Sscan(s, &i)
	return i
}

func portHopping(n *node.Node, p *om) {
	if n.Ports != "" {
		p.force("ports", n.Ports)
		p.set("hop-interval", n.HopInterval)
	}
}

// tlsFields writes sni/alpn/skip-cert-verify/fingerprint. vmessStyle uses
// "servername" (vmess/vless) instead of "sni".
func tlsFields(n *node.Node, p *om, vmessStyle bool) {
	t := n.TLS
	if t == nil {
		return
	}
	if vmessStyle {
		p.set("servername", t.SNI)
	} else {
		p.set("sni", t.SNI)
	}
	p.set("alpn", t.ALPN)
	p.set("skip-cert-verify", t.SkipVerify)
	p.set("server-cert-fingerprint", t.CertSHA256)
	p.set("client-fingerprint", t.Fingerprint)
}

func tlsAndTransport(c *render.Config, n *node.Node, p *om) error {
	vmessStyle := n.Type == node.VMess || n.Type == node.VLESS
	if n.Type != node.Trojan {
		p.force("tls", n.TLSEnabled())
	}
	tlsFields(n, p, vmessStyle)
	if n.TLS.Reality() {
		feat := "tls.reality"
		if n.Type != node.VLESS {
			feat = "tls.reality-non-vless"
		}
		if !c.Has(feat) {
			return fmt.Errorf("REALITY %s", c.Req(feat))
		}
		p.force("reality-opts", om{{"public-key", n.TLS.RealityPubKey}, {"short-id", n.TLS.RealityShortID}})
	}
	t := n.TransportType()
	if t == "tcp" {
		return nil
	}
	if !c.Has("transport." + t) {
		return fmt.Errorf("transport %s %s", t, c.Req("transport."+t))
	}
	tr := n.Transport
	switch t {
	case "ws", "httpupgrade":
		p.force("network", "ws")
		o := om{}
		o.set("path", tr.Path)
		h := map[string]string{}
		if tr.Host != "" {
			h["Host"] = tr.Host
		}
		for k, v := range tr.Headers {
			h[k] = v
		}
		o.set("headers", h)
		if t == "httpupgrade" {
			o.force("v2ray-http-upgrade", true)
		}
		p.set("ws-opts", o)
	case "grpc":
		p.force("network", "grpc")
		p.force("grpc-opts", om{{"grpc-service-name", tr.ServiceName}})
	case "h2":
		p.force("network", "h2")
		o := om{}
		if tr.Host != "" {
			o.force("host", []string{tr.Host})
		}
		o.set("path", tr.Path)
		p.set("h2-opts", o)
	case "http":
		p.force("network", "http")
		o := om{}
		if tr.Path != "" {
			o.force("path", []string{tr.Path})
		}
		if tr.Host != "" {
			o.force("headers", om{{"Host", []string{tr.Host}}})
		}
		p.set("http-opts", o)
	case "xhttp":
		p.force("network", "xhttp")
		o := om{}
		o.set("path", tr.Path)
		o.set("host", tr.Host)
		o.set("mode", tr.Mode)
		p.set("xhttp-opts", o)
	default:
		return fmt.Errorf("transport %s not supported", t)
	}
	return nil
}

func group(c *render.Config, g *render.Group, w *render.Warnings) om {
	typ := render.DowngradeGroupType(c, g, w)
	o := om{}
	o.force("name", g.Name)
	o.force("type", string(typ))
	o.force("proxies", g.Members)
	switch typ {
	case profile.URLTest, profile.Fallback, profile.LoadBalance:
		o.force("url", or(g.URL, or(c.General.TestURL, render.DefaultTestURL)))
		o.force("interval", or0(g.Interval, 600))
		if typ == profile.URLTest {
			o.set("tolerance", or0(g.Tolerance, 100))
		}
		o.force("lazy", true)
		if typ == profile.LoadBalance {
			s := "round-robin"
			if g.Persistent {
				s = "consistent-hashing"
			}
			o.force("strategy", s)
		}
	}
	o.set("hidden", g.Hidden)
	if g.Icon != "" && c.Has("group.icon") {
		o.set("icon", g.Icon)
	}
	return o
}

var escapedRune = regexp.MustCompile(`\\U([0-9A-Fa-f]{8})`)

// unescapeUnicode turns yaml.v3's \UXXXXXXXX escapes (emitted for emoji)
// back into literal characters; YAML double-quoted strings accept them as-is.
func unescapeUnicode(s string) string {
	return escapedRune.ReplaceAllStringFunc(s, func(m string) string {
		v, err := strconv.ParseUint(m[2:], 16, 32)
		if err != nil {
			return m
		}
		return string(rune(v))
	})
}

// convertHosts maps Surge-style [Host] entries to Clash config: fixed IPs go to
// top-level `hosts`, "server:X" entries to dns.nameserver-policy. mihomo also takes
// domain aliases in hosts and "system" as a nameserver; Stash has neither, so
// those entries are skipped with a warning.
func convertHosts(hs []profile.Host, meta bool, label string, w *render.Warnings) (hosts, policy om) {
	ips := map[string][]string{}
	var order []string
	for _, h := range hs {
		if strings.Contains(h.Domain, ":") { // RULE-SET:url / DOMAIN-SET:url (Surge only)
			w.Add("host", h.Domain, "rule-set based host entries are Surge-only; skipped")
			continue
		}
		switch h.Kind() {
		case "ip":
			if _, ok := ips[h.Domain]; !ok {
				order = append(order, h.Domain)
			}
			ips[h.Domain] = append(ips[h.Domain], h.IPs()...)
		case "server":
			srv := strings.TrimPrefix(h.Value, "server:")
			if srv == "system" && !meta {
				w.Add("host", h.Domain, "server:system has no %s equivalent; skipped", label)
				continue
			}
			var v any = srv
			if strings.Contains(srv, ",") {
				parts := strings.Split(srv, ",")
				for i := range parts {
					parts[i] = strings.TrimSpace(parts[i])
				}
				v = parts
			}
			policy.force(h.Domain, v)
		default:
			if meta {
				if _, ok := ips[h.Domain]; !ok {
					order = append(order, h.Domain)
				}
				ips[h.Domain] = append(ips[h.Domain], h.Value)
				continue
			}
			w.Add("host", h.Domain, "domain alias (%s) is not supported by %s hosts; skipped", h.Value, label)
		}
	}
	for _, d := range order {
		if v := ips[d]; len(v) == 1 {
			hosts.force(d, v[0])
		} else {
			hosts.force(d, v)
		}
	}
	return hosts, policy
}

// mergeExtra merges user YAML into the generated config. Mappings merge key by
// key (recursively), so "dns: {fake-ip-filter: [...]}" adds to the generated dns
// block; any other value replaces the generated one or is appended as a new key.
func mergeExtra(root *om, extra string) error {
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(extra), &doc); err != nil {
		return err
	}
	if len(doc.Content) == 0 {
		return nil
	}
	m := doc.Content[0]
	if m.Kind != yaml.MappingNode {
		return fmt.Errorf("must be a YAML mapping (key: value)")
	}
	mergeInto(root, m)
	return nil
}

func mergeInto(dst *om, m *yaml.Node) {
	for i := 0; i+1 < len(m.Content); i += 2 {
		k, v := m.Content[i].Value, m.Content[i+1]
		idx := -1
		for j, e := range *dst {
			if e.k == k {
				idx = j
			}
		}
		switch {
		case idx < 0:
			dst.force(k, v)
		case v.Kind == yaml.MappingNode:
			if sub, ok := (*dst)[idx].v.(om); ok {
				mergeInto(&sub, v)
				(*dst)[idx].v = sub
				continue
			}
			(*dst)[idx].v = v
		default:
			(*dst)[idx].v = v
		}
	}
}

func headComment(lines []string) string {
	for i, l := range lines {
		lines[i] = "# " + l
	}
	return strings.Join(lines, "\n")
}

// ruleList is the rules sequence, with each rule's comment above it.
func ruleList(rules, comments []string) *yaml.Node {
	n := &yaml.Node{Kind: yaml.SequenceNode}
	for i, r := range rules {
		n.Content = append(n.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: r, HeadComment: comments[i]})
	}
	return n
}
