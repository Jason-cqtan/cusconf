// Package singbox renders sing-box JSON configs (1.11+ format; 1.12+ DNS servers).
package singbox

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/netip"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/cusconf/cusconf/internal/node"
	"github.com/cusconf/cusconf/internal/profile"
	"github.com/cusconf/cusconf/internal/render"
)

// obj is an insertion-ordered JSON object.
type obj []kv
type kv struct {
	k string
	v any
}

// set appends k unless v is a zero value.
func (o *obj) set(k string, v any) {
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
	case []int:
		if len(x) == 0 {
			return
		}
	case obj:
		if len(x) == 0 {
			return
		}
	}
	*o = append(*o, kv{k, v})
}

func (o *obj) force(k string, v any) { *o = append(*o, kv{k, v}) }

func (o obj) get(k string) (int, bool) {
	for i, e := range o {
		if e.k == k {
			return i, true
		}
	}
	return -1, false
}

func (o obj) MarshalJSON() ([]byte, error) {
	var b bytes.Buffer
	b.WriteByte('{')
	for i, e := range o {
		if i > 0 {
			b.WriteByte(',')
		}
		k, _ := json.Marshal(e.k)
		b.Write(k)
		b.WriteByte(':')
		v, err := marshal(e.v)
		if err != nil {
			return nil, err
		}
		b.Write(v)
	}
	b.WriteByte('}')
	return b.Bytes(), nil
}

func marshal(v any) ([]byte, error) {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimRight(b.Bytes(), "\n"), nil
}

const directTag = "DIRECT"

func Render(c *render.Config) render.Result {
	var w render.Warnings
	g := c.General
	typedDNS := c.Has("dns.typed-servers")

	// Outbounds: nodes first, so groups can refer to them.
	var nodeOuts, endpoints, helpers []obj
	kept := map[string]bool{}
	for _, n := range c.Nodes {
		o, helper, isEndpoint, err := outbound(c, n)
		if err != nil {
			w.Add("node", n.Name, "%v", err)
			continue
		}
		kept[n.Name] = true
		if isEndpoint {
			endpoints = append(endpoints, o)
		} else {
			nodeOuts = append(nodeOuts, o)
		}
		if helper != nil {
			helpers = append(helpers, helper)
		}
	}

	groups := render.FinalizeGroups(c.Groups, kept, &w)
	var groupOuts []obj
	for i := range groups {
		groupOuts = append(groupOuts, group(c, &groups[i], &w))
	}
	outbounds := append(groupOuts, nodeOuts...)
	outbounds = append(outbounds, helpers...)
	outbounds = append(outbounds, obj{{"type", "direct"}, {"tag", directTag}})

	proxyTag := directTag
	if len(groups) > 0 {
		proxyTag = groups[0].Name
	}

	dns, dnsTags := dnsConfig(c, typedDNS, &w)
	route, ruleSets := routeConfig(c, groups, proxyTag, &w)
	if len(ruleSets) > 0 {
		route.force("rule_set", ruleSets)
	}
	if typedDNS && len(dnsTags) > 0 {
		route.force("default_domain_resolver", dnsTags[0])
	}

	root := obj{}
	root.force("log", obj{{"level", "info"}, {"timestamp", true}})
	root.force("dns", dns)
	tun := obj{{"type", "tun"}, {"tag", "tun-in"}, {"address", tunAddresses(g.IPv6)}, {"auto_route", true}, {"strict_route", true}, {"stack", "mixed"}}
	root.force("inbounds", []obj{tun, {{"type", "mixed"}, {"tag", "mixed-in"}, {"listen", "127.0.0.1"}, {"listen_port", 7890}}})
	root.force("outbounds", outbounds)
	if len(endpoints) > 0 {
		root.force("endpoints", endpoints)
	}
	root.force("route", route)
	root.force("experimental", obj{
		{"cache_file", obj{{"enabled", true}}},
		{"clash_api", obj{{"external_controller", "127.0.0.1:9090"}}},
	})

	if extra := strings.TrimSpace(g.Extra["sing-box"]); extra != "" {
		if err := mergeExtra(&root, extra); err != nil {
			w.Add("general", "sing-box 追加 JSON", "ignored: %v", err)
		}
	}

	var b bytes.Buffer
	raw, err := marshal(root)
	if err != nil {
		w.Add("general", "json", "%v", err)
	}
	json.Indent(&b, raw, "", "  ")
	b.WriteByte('\n')
	return render.Result{Content: b.String(), ContentType: "application/json; charset=utf-8", Warnings: w}
}

func tunAddresses(ipv6 bool) []string {
	if ipv6 {
		return []string{"172.19.0.1/30", "fdfe:dcba:9876::1/126"}
	}
	return []string{"172.19.0.1/30"}
}

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

// --- outbounds ---

// outbound converts a node. helper is an extra outbound the node depends on
// (shadow-tls); endpoint marks WireGuard, which sing-box 1.11+ models as an endpoint.
func outbound(c *render.Config, n *node.Node) (o, helper obj, endpoint bool, err error) {
	feat := "proxy." + string(n.Type)
	if n.Type == node.TUIC && n.Version == 5 {
		feat = "proxy.tuic-v5"
	}
	if !c.Has(feat) {
		return nil, nil, false, fmt.Errorf("%s %s", n.Type, c.Req(feat))
	}
	typ := map[node.Type]string{node.SS: "shadowsocks", node.SOCKS5: "socks"}[n.Type]
	if typ == "" {
		typ = string(n.Type)
	}
	o = obj{{"type", typ}, {"tag", n.Name}}
	server := func() {
		o.force("server", n.Server)
		o.force("server_port", n.Port)
	}

	switch n.Type {
	case node.SS:
		server()
		o.force("method", n.Cipher)
		o.force("password", n.Password)
		switch n.Plugin {
		case "":
		case "obfs":
			opts := "obfs=" + n.PluginOpts["mode"]
			if h := n.PluginOpts["host"]; h != "" {
				opts += ";obfs-host=" + h
			}
			o.force("plugin", "obfs-local")
			o.force("plugin_opts", opts)
		case "v2ray-plugin":
			var opts []string
			if n.PluginOpts["tls"] == "true" {
				opts = append(opts, "tls")
			}
			for _, k := range []string{"host", "path"} {
				if v := n.PluginOpts[k]; v != "" {
					opts = append(opts, k+"="+v)
				}
			}
			o.force("plugin", "v2ray-plugin")
			o.force("plugin_opts", strings.Join(opts, ";"))
		case "shadow-tls":
			v := or(n.PluginOpts["version"], "2")
			if !c.Has("shadow-tls.v" + v) {
				return nil, nil, false, fmt.Errorf("shadow-tls v%s %s", v, c.Req("shadow-tls.v"+v))
			}
			ver, _ := strconv.Atoi(v)
			tag := n.Name + " (shadow-tls)"
			helper = obj{{"type", "shadowtls"}, {"tag", tag}, {"server", n.Server}, {"server_port", n.Port}, {"version", ver}}
			helper.set("password", n.PluginOpts["password"])
			helper.force("tls", obj{{"enabled", true}, {"server_name", n.PluginOpts["host"]}, {"utls", obj{{"enabled", true}, {"fingerprint", "chrome"}}}})
			o = obj{{"type", typ}, {"tag", n.Name}, {"method", n.Cipher}, {"password", n.Password}, {"detour", tag}}
		default:
			return nil, nil, false, fmt.Errorf("ss plugin %q not supported by sing-box", n.Plugin)
		}
		if n.UDP != nil && !*n.UDP {
			o.force("network", "tcp")
		}
	case node.VMess:
		server()
		o.force("uuid", n.UUID)
		o.force("security", or(n.Cipher, "auto"))
		o.set("alter_id", n.AlterID)
		if err := tlsAndTransport(c, n, &o); err != nil {
			return nil, nil, false, err
		}
	case node.VLESS:
		if n.Cipher != "" && n.Cipher != "none" {
			return nil, nil, false, fmt.Errorf("VLESS encryption %s", c.Req("vless.encryption"))
		}
		server()
		o.force("uuid", n.UUID)
		o.set("flow", n.Flow)
		if err := tlsAndTransport(c, n, &o); err != nil {
			return nil, nil, false, err
		}
		o.force("packet_encoding", "xudp")
	case node.Trojan:
		server()
		o.force("password", n.Password)
		if n.TLS == nil {
			n2 := *n
			n2.TLS = &node.TLS{Enabled: true}
			n = &n2
		}
		if err := tlsAndTransport(c, n, &o); err != nil {
			return nil, nil, false, err
		}
	case node.Hysteria2:
		server()
		if n.Ports != "" {
			o.force("server_ports", portRanges(n.Ports))
			if n.HopInterval > 0 {
				o.force("hop_interval", fmt.Sprintf("%ds", n.HopInterval))
			}
		}
		o.set("up_mbps", n.UpMbps)
		o.set("down_mbps", n.DownMbps)
		if n.Obfs != "" {
			if !c.Has("hysteria2." + n.Obfs) {
				return nil, nil, false, fmt.Errorf("hysteria2 %s obfs %s", n.Obfs, c.Req("hysteria2."+n.Obfs))
			}
			o.force("obfs", obj{{"type", n.Obfs}, {"password", n.ObfsPassword}})
		}
		o.force("password", n.Password)
		o.force("tls", tlsObj(n, true))
	case node.Hysteria:
		server()
		if n.Ports != "" {
			o.force("server_ports", portRanges(n.Ports))
		}
		o.force("up_mbps", or0(n.UpMbps, 50))
		o.force("down_mbps", or0(n.DownMbps, 200))
		o.set("obfs", n.Obfs)
		o.set("auth_str", n.Password)
		o.force("tls", tlsObj(n, true))
	case node.TUIC:
		if n.Version == 4 || (n.Token != "" && n.UUID == "") {
			return nil, nil, false, fmt.Errorf("TUIC v4 not supported by sing-box")
		}
		server()
		o.force("uuid", n.UUID)
		o.force("password", n.Password)
		o.set("congestion_control", n.CongestionControl)
		o.set("udp_relay_mode", n.UDPRelayMode)
		o.force("tls", tlsObj(n, true))
	case node.AnyTLS:
		server()
		o.force("password", n.Password)
		o.force("tls", tlsObj(n, true))
	case node.HTTP:
		server()
		o.set("username", n.Username)
		o.set("password", n.Password)
		if n.TLSEnabled() {
			o.force("tls", tlsObj(n, true))
		}
	case node.SOCKS5:
		if n.TLSEnabled() {
			return nil, nil, false, fmt.Errorf("SOCKS5 over TLS not supported by sing-box")
		}
		server()
		o.force("version", "5")
		o.set("username", n.Username)
		o.set("password", n.Password)
	case node.WireGuard:
		wg := n.WireGuard
		if len(wg.Peers) > 1 {
			return nil, nil, false, fmt.Errorf("multi-peer wireguard not supported here")
		}
		peer := wg.Peers[0]
		var addrs []string
		for _, a := range []string{wg.IP, wg.IPv6} {
			if a == "" {
				continue
			}
			if !strings.Contains(a, "/") {
				if ip, err := netip.ParseAddr(a); err == nil {
					a = netip.PrefixFrom(ip, ip.BitLen()).String()
				}
			}
			addrs = append(addrs, a)
		}
		o = obj{{"type", "wireguard"}, {"tag", n.Name}}
		o.set("mtu", wg.MTU)
		o.force("address", addrs)
		o.force("private_key", wg.PrivateKey)
		p := obj{{"address", n.Server}, {"port", n.Port}, {"public_key", peer.PublicKey}}
		p.set("pre_shared_key", peer.PresharedKey)
		allowed := peer.AllowedIPs
		if len(allowed) == 0 {
			allowed = []string{"0.0.0.0/0", "::/0"}
		}
		p.force("allowed_ips", allowed)
		p.set("persistent_keepalive_interval", peer.Keepalive)
		if len(peer.Reserved) == 3 {
			p.force("reserved", peer.Reserved)
		}
		o.force("peers", []obj{p})
		return o, nil, true, nil
	default:
		return nil, nil, false, fmt.Errorf("%s not supported by sing-box", n.Type)
	}
	if n.TFO && n.Type != node.Hysteria2 && n.Type != node.TUIC && n.Type != node.Hysteria {
		o.force("tcp_fast_open", true)
	}
	return o, helper, false, nil
}

// portRanges converts "443,5000-6000" to sing-box server_ports ["443:443", "5000:6000"].
func portRanges(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if a, b, ok := strings.Cut(p, "-"); ok {
			out = append(out, a+":"+b)
		} else {
			out = append(out, p+":"+p)
		}
	}
	return out
}

func tlsObj(n *node.Node, enabled bool) obj {
	t := n.TLS
	o := obj{{"enabled", enabled}}
	if t == nil {
		return o
	}
	o.set("server_name", t.SNI)
	o.set("insecure", t.SkipVerify)
	o.set("alpn", t.ALPN)
	fp := t.Fingerprint
	if t.Reality() && fp == "" {
		fp = "chrome" // REALITY needs uTLS
	}
	if fp != "" {
		o.force("utls", obj{{"enabled", true}, {"fingerprint", fp}})
	}
	if t.Reality() {
		r := obj{{"enabled", true}, {"public_key", t.RealityPubKey}}
		r.set("short_id", t.RealityShortID)
		o.force("reality", r)
	}
	return o
}

func tlsAndTransport(c *render.Config, n *node.Node, o *obj) error {
	if n.TLSEnabled() {
		if n.TLS.Reality() {
			feat := "tls.reality"
			if n.Type != node.VLESS {
				feat = "tls.reality-non-vless"
			}
			if !c.Has(feat) {
				return fmt.Errorf("REALITY %s", c.Req(feat))
			}
		}
		o.force("tls", tlsObj(n, true))
	}
	t := n.TransportType()
	if t == "tcp" {
		return nil
	}
	if !c.Has("transport." + t) {
		return fmt.Errorf("transport %s %s", t, c.Req("transport."+t))
	}
	tr := n.Transport
	headers := obj{}
	for k, v := range tr.Headers {
		if !strings.EqualFold(k, "host") {
			headers.force(k, v)
		}
	}
	x := obj{}
	switch t {
	case "ws":
		x.force("type", "ws")
		path, ed := earlyData(tr.Path)
		x.set("path", path)
		if tr.Host != "" {
			headers = append(obj{{"Host", tr.Host}}, headers...)
		}
		x.set("headers", headers)
		if ed > 0 {
			x.force("max_early_data", ed)
			x.force("early_data_header_name", "Sec-WebSocket-Protocol")
		}
	case "httpupgrade":
		x.force("type", "httpupgrade")
		x.set("host", tr.Host)
		x.set("path", tr.Path)
		x.set("headers", headers)
	case "grpc":
		x.force("type", "grpc")
		x.set("service_name", tr.ServiceName)
	case "h2", "http":
		x.force("type", "http")
		if tr.Host != "" {
			x.force("host", []string{tr.Host})
		}
		x.set("path", tr.Path)
		x.set("headers", headers)
	default:
		return fmt.Errorf("transport %s not supported by sing-box", t)
	}
	o.force("transport", x)
	return nil
}

// earlyData splits Xray's "?ed=2048" early-data hint off a WebSocket path.
func earlyData(p string) (string, int) {
	u, err := url.Parse(p)
	if err != nil || u.RawQuery == "" {
		return p, 0
	}
	q := u.Query()
	ed, _ := strconv.Atoi(q.Get("ed"))
	if ed <= 0 {
		return p, 0
	}
	q.Del("ed")
	u.RawQuery = q.Encode()
	return u.String(), ed
}

// --- groups ---

func group(c *render.Config, g *render.Group, w *render.Warnings) obj {
	typ := render.DowngradeGroupType(c, g, w)
	var members []string
	for _, m := range g.Members {
		switch strings.ToUpper(m) {
		case "REJECT":
			w.Add("group", g.Name, "REJECT cannot be a sing-box group member; removed (use a REJECT rule instead)")
		case "DIRECT":
			members = append(members, directTag)
		default:
			members = append(members, m)
		}
	}
	if len(members) == 0 {
		members = []string{directTag}
	}
	if typ == profile.Select {
		return obj{{"type", "selector"}, {"tag", g.Name}, {"outbounds", members}, {"interrupt_exist_connections", true}}
	}
	o := obj{{"type", "urltest"}, {"tag", g.Name}, {"outbounds", members}}
	o.force("url", or(g.URL, or(c.General.TestURL, render.DefaultTestURL)))
	o.force("interval", fmt.Sprintf("%ds", or0(g.Interval, 600)))
	o.force("tolerance", or0(g.Tolerance, 100))
	return o
}

// --- DNS ---

// dnsServer converts a DNS address (IP, host, tls://, https://, quic://, h3://, system).
func dnsServer(addr, tag string, typed bool) obj {
	if !typed {
		if addr == "system" {
			addr = "local"
		}
		return obj{{"tag", tag}, {"address", addr}, {"detour", directTag}}
	}
	if addr == "system" {
		return obj{{"type", "local"}, {"tag", tag}}
	}
	o := obj{}
	if u, err := url.Parse(addr); err == nil && u.Scheme != "" && u.Host != "" {
		typ := map[string]string{"https": "https", "tls": "tls", "quic": "quic", "h3": "h3", "tcp": "tcp", "udp": "udp"}[u.Scheme]
		if typ == "" {
			typ = "udp"
		}
		o.force("type", typ)
		o.force("tag", tag)
		o.force("server", u.Hostname())
		if p, _ := strconv.Atoi(u.Port()); p > 0 {
			o.force("server_port", p)
		}
		if (typ == "https" || typ == "h3") && u.Path != "" && u.Path != "/dns-query" {
			o.force("path", u.Path)
		}
		return o
	}
	host := addr
	port := 0
	if h, p, err := splitHostPort(addr); err == nil {
		host, port = h, p
	}
	o = obj{{"type", "udp"}, {"tag", tag}, {"server", host}}
	o.set("server_port", port)
	return o
}

func splitHostPort(s string) (string, int, error) {
	if ap, err := netip.ParseAddrPort(s); err == nil {
		return ap.Addr().String(), int(ap.Port()), nil
	}
	return "", 0, fmt.Errorf("no port")
}

func dnsConfig(c *render.Config, typed bool, w *render.Warnings) (obj, []string) {
	g := c.General
	addrs := g.DNS
	if len(addrs) == 0 {
		addrs = []string{"223.5.5.5", "119.29.29.29"}
	}
	var servers []obj
	var tags []string
	byAddr := map[string]string{}
	addServer := func(a string) string {
		a = strings.TrimSpace(a)
		if t, ok := byAddr[a]; ok {
			return t
		}
		t := fmt.Sprintf("dns-%d", len(servers)+1)
		if a == "system" {
			t = "dns-system"
		}
		byAddr[a] = t
		servers = append(servers, dnsServer(a, t, typed))
		tags = append(tags, t)
		return t
	}
	for _, a := range addrs {
		addServer(a)
	}
	var rules []obj
	predefined := obj{}
	var predefinedDomains []string
	for _, h := range g.Hosts {
		if strings.Contains(h.Domain, ":") {
			w.Add("host", h.Domain, "rule-set based host entries are Surge-only; skipped")
			continue
		}
		match := domainMatch(h.Domain)
		switch h.Kind() {
		case "server":
			srv := strings.Split(strings.TrimPrefix(h.Value, "server:"), ",")[0]
			r := append(obj{}, match...)
			r.force("server", addServer(srv))
			rules = append(rules, r)
		case "ip":
			if !typed {
				w.Add("host", h.Domain, "fixed IP hosts %s; skipped", c.Req("dns.typed-servers"))
				continue
			}
			if strings.HasPrefix(h.Domain, "*.") {
				w.Add("host", h.Domain, "wildcard fixed IP hosts are not supported by sing-box; skipped")
				continue
			}
			predefined.force(h.Domain, h.IPs())
			predefinedDomains = append(predefinedDomains, h.Domain)
		default:
			w.Add("host", h.Domain, "domain alias (%s) is not supported by sing-box; skipped", h.Value)
		}
	}
	if len(predefined) > 0 {
		servers = append(servers, obj{{"type", "hosts"}, {"tag", "hosts"}, {"predefined", predefined}})
		rules = append([]obj{{{"domain", predefinedDomains}, {"server", "hosts"}}}, rules...)
	}
	d := obj{{"servers", servers}}
	if len(rules) > 0 {
		d.force("rules", rules)
	}
	d.force("final", tags[0])
	if !g.IPv6 {
		d.force("strategy", "ipv4_only")
	}
	return d, tags
}

// domainMatch turns a hosts domain ("*.corp.com" or "a.com") into a rule matcher.
func domainMatch(d string) obj {
	if strings.HasPrefix(d, "*.") {
		return obj{{"domain_suffix", []string{d[2:]}}}
	}
	return obj{{"domain", []string{d}}}
}

// --- route ---

var ruleField = map[string]string{
	"DOMAIN": "domain", "DOMAIN-SUFFIX": "domain_suffix", "DOMAIN-KEYWORD": "domain_keyword", "DOMAIN-REGEX": "domain_regex",
	"IP-CIDR": "ip_cidr", "IP-CIDR6": "ip_cidr", "SRC-IP-CIDR": "source_ip_cidr",
	"PROCESS-NAME": "process_name", "PROCESS-PATH": "process_path",
	"DST-PORT": "port", "SRC-PORT": "source_port", "NETWORK": "network",
}

func routeConfig(c *render.Config, groups []render.Group, proxyTag string, w *render.Warnings) (obj, []obj) {
	known := map[string]bool{directTag: true}
	for _, g := range groups {
		known[g.Name] = true
	}
	var rules []obj
	rules = append(rules, obj{{"action", "sniff"}}, obj{{"protocol", "dns"}, {"action", "hijack-dns"}}, obj{{"ip_is_private", true}, {"outbound", directTag}})
	var sets []obj
	setTags := map[string]bool{}
	addSet := func(tag, u, format, detour string, interval int) {
		if setTags[tag] {
			return
		}
		setTags[tag] = true
		s := obj{{"type", "remote"}, {"tag", tag}, {"format", format}, {"url", u}, {"download_detour", detour}}
		s.force("update_interval", fmt.Sprintf("%dh", max(or0(interval, 86400)/3600, 1)))
		sets = append(sets, s)
	}
	final := ""
	for _, r := range c.Rules {
		policy := r.Policy
		action := obj{}
		switch strings.ToUpper(policy) {
		case "REJECT":
			action.force("action", "reject")
		case "DIRECT":
			action.force("outbound", directTag)
		default:
			if !known[policy] {
				w.Add("rule", r.Type+","+r.Value, "unknown policy %q; skipped", policy)
				continue
			}
			action.force("outbound", policy)
		}
		var m obj
		switch t := strings.ToUpper(r.Type); t {
		case "FINAL", "MATCH":
			if strings.EqualFold(policy, "REJECT") {
				w.Add("rule", "FINAL", "REJECT as final is not supported by sing-box; using DIRECT")
				final = directTag
			} else {
				final = action[0].v.(string)
			}
			continue
		case "RULE-SET":
			rs := c.RuleSets[r.Value]
			u, mirrored := c.RuleSetURL(rs, rs.SingboxURL, "json")
			if u == "" {
				w.Add("rule", r.Value, "rule set has no sing-box URL and the mirror is unavailable; skipped")
				continue
			}
			format := "source"
			detour := directTag
			if !mirrored {
				if strings.HasSuffix(strings.SplitN(u, "?", 2)[0], ".srs") {
					format = "binary"
				}
				detour = proxyTag
			}
			addSet(rs.Name, u, format, detour, rs.Interval)
			m = obj{{"rule_set", []string{rs.Name}}}
		case "GEOIP", "GEOSITE":
			v := strings.ToLower(r.Value)
			if t == "GEOIP" && v == "lan" {
				m = obj{{"ip_is_private", true}}
				break
			}
			kind := strings.ToLower(t)
			tag := kind + "-" + v
			addSet(tag, fmt.Sprintf("https://raw.githubusercontent.com/SagerNet/sing-%s/rule-set/%s.srs", kind, tag), "binary", proxyTag, 0)
			m = obj{{"rule_set", []string{tag}}}
		case "DOMAIN-WILDCARD":
			m = obj{{"domain_regex", []string{WildcardRegex(r.Value)}}}
		case "PROTOCOL":
			t, v, ok := render.ProtocolRule(c, r.Value, w)
			if !ok {
				continue
			}
			if t == "NETWORK" {
				m = obj{{"network", v}}
			} else {
				m = obj{{"protocol", []string{v}}}
			}
		default:
			f, ok := ruleField[t]
			if !ok {
				w.Add("rule", r.Type+","+r.Value, "rule type %s not supported by sing-box; skipped", r.Type)
				continue
			}
			var v any = []string{r.Value}
			switch f {
			case "port", "source_port":
				n, err := strconv.Atoi(r.Value)
				if err != nil {
					w.Add("rule", r.Type+","+r.Value, "port ranges are not supported here; skipped")
					continue
				}
				v = []int{n}
			case "network":
				v = strings.ToLower(r.Value)
			}
			m = obj{{f, v}}
		}
		rules = appendRule(rules, append(m, action...))
	}
	if final == "" {
		final = proxyTag
		w.Add("rule", "FINAL", "no final rule; using %s", final)
	}
	return obj{{"rules", rules}, {"final", final}, {"auto_detect_interface", true}}, sets
}

// appendRule adds a rule, folding it into the previous one when both match on the
// same single list field with the same action (e.g. runs of DOMAIN,x,DIRECT).
func appendRule(rules []obj, r obj) []obj {
	if len(rules) == 0 || len(r) != 2 {
		return append(rules, r)
	}
	last := rules[len(rules)-1]
	if len(last) != 2 || last[0].k != r[0].k || last[1].k != r[1].k || last[1].v != r[1].v {
		return append(rules, r)
	}
	a, ok1 := last[0].v.([]string)
	b, ok2 := r[0].v.([]string)
	if !ok1 || !ok2 {
		return append(rules, r)
	}
	last[0].v = append(append([]string(nil), a...), b...)
	return rules
}

// WildcardRegex converts a DOMAIN-WILDCARD pattern (* and ?) to a regex.
func WildcardRegex(p string) string {
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

// --- extra JSON ---

// mergeExtra merges a user JSON object into the config: objects merge key by key,
// anything else replaces the generated value or is added.
func mergeExtra(root *obj, extra string) error {
	dec := json.NewDecoder(strings.NewReader(extra))
	dec.UseNumber()
	var m map[string]any
	if err := dec.Decode(&m); err != nil {
		return fmt.Errorf("must be a JSON object: %w", err)
	}
	mergeInto(root, m)
	return nil
}

func mergeInto(dst *obj, m map[string]any) {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		v := m[k]
		i, ok := dst.get(k)
		if !ok {
			dst.force(k, v)
			continue
		}
		if sub, isMap := v.(map[string]any); isMap {
			if cur, isObj := (*dst)[i].v.(obj); isObj {
				mergeInto(&cur, sub)
				(*dst)[i].v = cur
				continue
			}
		}
		(*dst)[i].v = v
	}
}
