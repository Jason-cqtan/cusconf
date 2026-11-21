package parser

import (
	"fmt"
	"strings"

	"github.com/cusconf/cusconf/internal/node"
)

// SplitParams splits a Surge-style comma list, honoring double quotes and parentheses.
func SplitParams(s string) []string {
	var (
		out   []string
		cur   strings.Builder
		quote bool
		depth int
	)
	for _, r := range s {
		switch {
		case r == '"':
			quote = !quote
		case r == '(' && !quote:
			depth++
		case r == ')' && !quote:
			depth--
		case r == ',' && !quote && depth == 0:
			out = append(out, strings.TrimSpace(cur.String()))
			cur.Reset()
			continue
		}
		cur.WriteRune(r)
	}
	if t := strings.TrimSpace(cur.String()); t != "" || len(out) > 0 {
		out = append(out, t)
	}
	return out
}

func unquote(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		return s[1 : len(s)-1]
	}
	return s
}

type surgeDoc struct {
	proxyLines []string
	wgSections map[string]map[string][]string // name -> key -> values (peer may repeat)
}

func splitSurgeSections(text string) surgeDoc {
	doc := surgeDoc{wgSections: map[string]map[string][]string{}}
	section := ""
	hasSections := strings.Contains(text, "[Proxy]")
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || line[0] == '#' || line[0] == ';' || strings.HasPrefix(line, "//") {
			continue
		}
		if line[0] == '[' && strings.HasSuffix(line, "]") {
			section = line[1 : len(line)-1]
			continue
		}
		switch {
		case section == "Proxy" || (!hasSections && section == ""):
			doc.proxyLines = append(doc.proxyLines, line)
		case strings.HasPrefix(section, "WireGuard "):
			name := strings.TrimPrefix(section, "WireGuard ")
			if doc.wgSections[name] == nil {
				doc.wgSections[name] = map[string][]string{}
			}
			k, v, ok := strings.Cut(line, "=")
			if ok {
				k = strings.TrimSpace(k)
				doc.wgSections[name][k] = append(doc.wgSections[name][k], strings.TrimSpace(v))
			}
		}
	}
	return doc
}

func parseSurge(text string) Result {
	r := Result{Format: FormatSurge}
	doc := splitSurgeSections(text)
	for _, line := range doc.proxyLines {
		n, err := ParseSurgeLine(line, doc.wgSections)
		if err != nil {
			r.Errors = append(r.Errors, err)
			continue
		}
		if n != nil {
			r.Nodes = append(r.Nodes, n)
		}
	}
	return r
}

// ParseSurgeLine parses one `Name = type, host, port, k=v...` proxy line.
// Returns (nil, nil) for built-in policies like DIRECT.
func ParseSurgeLine(line string, wg map[string]map[string][]string) (*node.Node, error) {
	name, rest, ok := strings.Cut(line, "=")
	if !ok {
		return nil, fmt.Errorf("surge: bad line %.40q", line)
	}
	name = unquote(name)
	parts := SplitParams(rest)
	typ := strings.ToLower(strings.TrimSpace(parts[0]))
	switch typ {
	case "direct", "reject", "reject-tinygif", "reject-drop", "reject-no-drop":
		return nil, nil
	}
	n := &node.Node{Name: name}
	kv := map[string]string{}
	var positional []string
	for _, p := range parts[1:] {
		if k, v, ok := strings.Cut(p, "="); ok && !strings.Contains(k, " ") {
			kv[strings.TrimSpace(k)] = unquote(v)
		} else {
			positional = append(positional, unquote(p))
		}
	}
	if typ != "wireguard" {
		if len(positional) < 2 {
			return nil, fmt.Errorf("surge: %s missing server/port", name)
		}
		n.Server, n.Port = positional[0], atoi(positional[1])
	}
	if v, ok := kv["udp-relay"]; ok {
		n.UDP = node.BoolPtr(truthy(v))
	}
	n.TFO = truthy(kv["tfo"])

	tls := func(enabled bool) *node.TLS {
		t := &node.TLS{Enabled: enabled, SNI: kv["sni"], SkipVerify: truthy(kv["skip-cert-verify"]),
			ALPN: splitList(kv["alpn"]), Fingerprint: kv["client-fingerprint"], CertSHA256: kv["server-cert-fingerprint-sha256"],
			RealityPubKey: kv["reality-public-key"], RealityShortID: kv["reality-short-id"]}
		if !t.Enabled && !t.Reality() && t.SNI == "" {
			return nil
		}
		return t
	}
	ws := func() *node.Transport {
		switch {
		case truthy(kv["ws"]):
			t := &node.Transport{Type: "ws", Path: kv["ws-path"]}
			if truthy(kv["ws-http-upgrade"]) {
				t.Type = "httpupgrade"
			}
			t.Host, t.Headers = parseWSHeaders(kv["ws-headers"])
			return t
		case truthy(kv["grpc"]):
			return &node.Transport{Type: "grpc", ServiceName: kv["grpc-service-name"], Mode: kv["grpc-mode"]}
		case truthy(kv["xhttp"]):
			return &node.Transport{Type: "xhttp", Path: kv["xhttp-path"], Host: kv["xhttp-host"], Mode: kv["xhttp-mode"]}
		}
		return nil
	}

	switch typ {
	case "ss", "custom":
		n.Type = node.SS
		n.Cipher, n.Password = kv["encrypt-method"], kv["password"]
		if typ == "custom" && len(positional) >= 4 {
			n.Cipher, n.Password = positional[2], positional[3]
		}
		if o := kv["obfs"]; o != "" {
			n.Plugin = "obfs"
			n.PluginOpts = map[string]string{"mode": o, "host": kv["obfs-host"], "path": kv["obfs-uri"]}
		}
		if kv["shadow-tls-password"] != "" {
			n.Plugin = "shadow-tls"
			n.PluginOpts = map[string]string{"password": kv["shadow-tls-password"], "host": kv["shadow-tls-sni"], "version": kv["shadow-tls-version"]}
		}
	case "vmess":
		n.Type = node.VMess
		n.UUID, n.Cipher = kv["username"], kv["encrypt-method"]
		if n.Cipher == "" {
			n.Cipher = "auto"
		}
		if !truthy(kv["vmess-aead"]) && kv["vmess-aead"] != "" {
			n.AlterID = 1
		}
		n.TLS, n.Transport = tls(truthy(kv["tls"])), ws()
	case "vless":
		n.Type = node.VLESS
		n.UUID, n.Flow = kv["username"], kv["flow"]
		if e := kv["encryption"]; e != "" && e != "none" {
			n.Cipher = e
		}
		n.TLS, n.Transport = tls(truthy(kv["tls"])), ws()
	case "trojan":
		n.Type = node.Trojan
		n.Password = kv["password"]
		n.TLS, n.Transport = tls(true), ws()
	case "hysteria2":
		n.Type = node.Hysteria2
		n.Password = kv["password"]
		n.DownMbps = atoi(kv["download-bandwidth"])
		if ph := kv["port-hopping"]; ph != "" {
			n.Ports = strings.ReplaceAll(ph, ";", ",")
			n.HopInterval = atoi(kv["port-hopping-interval"])
		}
		if s := kv["salamander-password"]; s != "" {
			n.Obfs, n.ObfsPassword = "salamander", s
		}
		if s := kv["gecko-password"]; s != "" {
			n.Obfs, n.ObfsPassword = "gecko", s
		}
		n.TLS = tls(true)
	case "tuic", "tuic-v5":
		n.Type = node.TUIC
		if typ == "tuic" {
			n.Token, n.Version = kv["token"], 4
		} else {
			n.UUID, n.Password, n.Version = kv["uuid"], kv["password"], 5
		}
		n.TLS = tls(true)
	case "snell":
		n.Type = node.Snell
		n.Password, n.Version = kv["psk"], atoi(kv["version"])
		n.Obfs, n.ObfsHost = kv["obfs"], kv["obfs-host"]
	case "anytls":
		n.Type = node.AnyTLS
		n.Password = kv["password"]
		n.TLS = tls(true)
	case "http", "https", "socks5", "socks5-tls":
		n.Type = node.HTTP
		if strings.HasPrefix(typ, "socks5") {
			n.Type = node.SOCKS5
		}
		n.Username, n.Password = kv["username"], kv["password"]
		if len(positional) >= 4 {
			n.Username, n.Password = positional[2], positional[3]
		}
		if typ == "https" || typ == "socks5-tls" {
			n.TLS = tls(true)
		}
	case "wireguard":
		n.Type = node.WireGuard
		sec := wg[kv["section-name"]]
		if sec == nil {
			return nil, fmt.Errorf("surge: %s: wireguard section %q not found", name, kv["section-name"])
		}
		if err := surgeWireGuard(n, sec); err != nil {
			return nil, fmt.Errorf("surge: %s: %w", name, err)
		}
	default:
		return nil, fmt.Errorf("surge: %s: unsupported type %q", name, typ)
	}
	return n, nil
}

// parseWSHeaders parses `Host:"a.com"|User-Agent:x`.
func parseWSHeaders(s string) (host string, rest map[string]string) {
	if s == "" {
		return "", nil
	}
	for _, h := range strings.Split(s, "|") {
		k, v, ok := strings.Cut(h, ":")
		if !ok {
			continue
		}
		k, v = strings.TrimSpace(k), unquote(v)
		if strings.EqualFold(k, "host") {
			host = v
			continue
		}
		if rest == nil {
			rest = map[string]string{}
		}
		rest[k] = v
	}
	return
}

func surgeWireGuard(n *node.Node, sec map[string][]string) error {
	get := func(k string) string {
		if v := sec[k]; len(v) > 0 {
			return unquote(v[0])
		}
		return ""
	}
	wg := &node.WireGuardOpts{PrivateKey: get("private-key"), IP: get("self-ip"), IPv6: get("self-ip-v6"),
		DNS: splitList(get("dns-server")), MTU: atoi(get("mtu"))}
	for _, line := range sec["peer"] {
		// peer = (a = b, c = "d, e"), (...)
		for _, tuple := range SplitParams(line) {
			tuple = strings.TrimSuffix(strings.TrimPrefix(strings.TrimSpace(tuple), "("), ")")
			f := map[string]string{}
			for _, kvs := range SplitParams(tuple) {
				k, v, _ := strings.Cut(kvs, "=")
				f[strings.TrimSpace(k)] = unquote(v)
			}
			p := node.WireGuardPeer{PublicKey: f["public-key"], PresharedKey: f["preshared-key"],
				AllowedIPs: splitList(f["allowed-ips"]), Keepalive: atoi(f["keepalive"])}
			if ep := f["endpoint"]; ep != "" && n.Server == "" {
				i := strings.LastIndex(ep, ":")
				if i > 0 {
					n.Server = strings.Trim(ep[:i], "[]")
					n.Port = atoi(ep[i+1:])
				}
			}
			if cid := f["client-id"]; strings.Count(cid, "/") == 2 {
				for _, x := range strings.Split(cid, "/") {
					p.Reserved = append(p.Reserved, atoi(x))
				}
			}
			wg.Peers = append(wg.Peers, p)
		}
	}
	n.WireGuard = wg
	n.UDP = node.BoolPtr(true)
	if wg.PrivateKey == "" || len(wg.Peers) == 0 {
		return fmt.Errorf("incomplete wireguard section")
	}
	return nil
}
