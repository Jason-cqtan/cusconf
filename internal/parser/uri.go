package parser

import (
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"

	"github.com/cusconf/cusconf/internal/node"
)

// ParseURI parses one share link.
func ParseURI(raw string) (*node.Node, error) {
	scheme, _, ok := strings.Cut(raw, "://")
	if !ok {
		return nil, fmt.Errorf("not a share link: %.40q", raw)
	}
	var (
		n   *node.Node
		err error
	)
	switch strings.ToLower(scheme) {
	case "ss":
		n, err = parseSS(raw)
	case "ssr":
		n, err = parseSSR(raw)
	case "vmess":
		n, err = parseVMess(raw)
	case "vless":
		n, err = parseVLESS(raw)
	case "trojan":
		n, err = parseTrojan(raw)
	case "hysteria2", "hy2":
		n, err = parseHysteria2(raw)
	case "hysteria":
		n, err = parseHysteria(raw)
	case "tuic":
		n, err = parseTUIC(raw)
	case "wireguard", "wg":
		n, err = parseWireGuardURI(raw)
	case "anytls":
		n, err = parseAnyTLS(raw)
	case "socks", "socks5", "socks5h":
		n, err = parseSocks(raw)
	case "http", "https":
		n, err = parseHTTPProxy(raw)
	default:
		return nil, fmt.Errorf("unsupported scheme %q", scheme)
	}
	if err != nil {
		return nil, fmt.Errorf("%s: %w", scheme, err)
	}
	return n, nil
}

// --- helpers ---

func hostPort(u *url.URL) (string, int, error) {
	host := u.Hostname()
	p, err := strconv.Atoi(u.Port())
	if err != nil || host == "" {
		return "", 0, fmt.Errorf("invalid host:port %q", u.Host)
	}
	return host, p, nil
}

func fragmentName(u *url.URL) string {
	if u.Fragment != "" {
		return u.Fragment
	}
	if s, err := url.PathUnescape(u.RawFragment); err == nil {
		return s
	}
	return u.RawFragment
}

func truthy(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}

func splitList(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func atoi(s string) int {
	i, _ := strconv.Atoi(strings.TrimSpace(s))
	return i
}

// tlsFromQuery reads common TLS query params used by v2rayN-style links.
func tlsFromQuery(q url.Values, enabled bool) *node.TLS {
	t := &node.TLS{
		Enabled:        enabled,
		SNI:            first(q, "sni", "peer", "servername"),
		ALPN:           splitList(q.Get("alpn")),
		SkipVerify:     truthy(first(q, "allowInsecure", "insecure", "allow_insecure", "skip-cert-verify")),
		Fingerprint:    q.Get("fp"),
		RealityPubKey:  q.Get("pbk"),
		RealityShortID: q.Get("sid"),
		CertSHA256:     first(q, "pcs", "pinSHA256"),
	}
	if !t.Enabled && !t.Reality() && t.SNI == "" && !t.SkipVerify && len(t.ALPN) == 0 {
		return nil
	}
	return t
}

func first(q url.Values, keys ...string) string {
	for _, k := range keys {
		if v := q.Get(k); v != "" {
			return v
		}
	}
	return ""
}

func transportFromQuery(q url.Values) *node.Transport {
	typ := strings.ToLower(q.Get("type"))
	if typ == "" || typ == "tcp" || typ == "raw" {
		if q.Get("headerType") == "http" {
			return &node.Transport{Type: "http", Path: q.Get("path"), Host: q.Get("host")}
		}
		return nil
	}
	t := &node.Transport{Type: typ, Path: q.Get("path"), Host: q.Get("host")}
	switch typ {
	case "grpc":
		t.ServiceName = q.Get("serviceName")
		t.Mode = q.Get("mode")
		t.Path = ""
	case "xhttp", "splithttp":
		t.Type = "xhttp"
		t.Mode = q.Get("mode")
	}
	return t
}

// --- protocols ---

func parseSS(raw string) (*node.Node, error) {
	body := strings.TrimPrefix(raw[strings.Index(raw, "://")+3:], "//")
	name := ""
	if i := strings.Index(body, "#"); i >= 0 {
		name, _ = url.PathUnescape(body[i+1:])
		body = body[:i]
	}
	// Legacy: ss://base64(method:password@host:port)
	if !strings.Contains(body, "@") {
		main, query, _ := strings.Cut(body, "?")
		main = strings.TrimSuffix(main, "/")
		dec, ok := decodeBase64(main)
		if !ok {
			return nil, fmt.Errorf("bad legacy base64")
		}
		body = dec
		if query != "" {
			body += "/?" + query
		}
	}
	u, err := url.Parse("ss://" + body)
	if err != nil {
		return nil, err
	}
	host, port, err := hostPort(u)
	if err != nil {
		return nil, err
	}
	var method, password string
	if pw, has := u.User.Password(); has {
		method, password = u.User.Username(), pw
	} else {
		dec, ok := decodeBase64(u.User.Username())
		if !ok {
			return nil, fmt.Errorf("bad userinfo")
		}
		method, password, _ = strings.Cut(dec, ":")
	}
	n := &node.Node{Name: name, Type: node.SS, Server: host, Port: port, Cipher: method, Password: password}
	if p := u.Query().Get("plugin"); p != "" {
		parts := strings.Split(p, ";")
		opts := map[string]string{}
		for _, kv := range parts[1:] {
			k, v, _ := strings.Cut(kv, "=")
			opts[k] = v
		}
		switch parts[0] {
		case "obfs-local", "simple-obfs", "obfs":
			n.Plugin = "obfs"
			n.PluginOpts = map[string]string{"mode": opts["obfs"], "host": opts["obfs-host"]}
		case "v2ray-plugin":
			n.Plugin = "v2ray-plugin"
			n.PluginOpts = map[string]string{"mode": "websocket", "host": opts["host"], "path": opts["path"]}
			if _, ok := opts["tls"]; ok {
				n.PluginOpts["tls"] = "true"
			}
		case "shadow-tls":
			n.Plugin = "shadow-tls"
			n.PluginOpts = opts
		default:
			n.Plugin = parts[0]
			n.PluginOpts = opts
		}
	}
	if truthy(u.Query().Get("uot")) {
		n.UDP = node.BoolPtr(true)
	}
	return n, nil
}

func parseSSR(raw string) (*node.Node, error) {
	dec, ok := decodeBase64(raw[len("ssr://"):])
	if !ok {
		return nil, fmt.Errorf("bad base64")
	}
	main, query, _ := strings.Cut(dec, "/?")
	// host may be IPv6 containing ':', so split from the right.
	parts := strings.Split(main, ":")
	if len(parts) < 6 {
		return nil, fmt.Errorf("bad ssr body")
	}
	l := len(parts)
	pw, _ := decodeBase64(parts[l-1])
	n := &node.Node{
		Type:     node.SSR,
		Server:   strings.Join(parts[:l-5], ":"),
		Port:     atoi(parts[l-5]),
		Protocol: parts[l-4],
		Cipher:   parts[l-3],
		Obfs:     parts[l-2],
		Password: pw,
	}
	q, _ := url.ParseQuery(query)
	b64 := func(k string) string { s, _ := decodeBase64(q.Get(k)); return s }
	n.ObfsParam = b64("obfsparam")
	n.ProtocolParam = b64("protoparam")
	n.Name = b64("remarks")
	return n, nil
}

type vmessJSON struct {
	PS       string          `json:"ps"`
	Add      string          `json:"add"`
	Port     json.RawMessage `json:"port"`
	ID       string          `json:"id"`
	Aid      json.RawMessage `json:"aid"`
	Scy      string          `json:"scy"`
	Net      string          `json:"net"`
	Type     string          `json:"type"`
	Host     string          `json:"host"`
	Path     string          `json:"path"`
	TLS      string          `json:"tls"`
	SNI      string          `json:"sni"`
	ALPN     string          `json:"alpn"`
	FP       string          `json:"fp"`
	Insecure json.RawMessage `json:"allowInsecure"`
}

func rawInt(b json.RawMessage) int {
	s := strings.Trim(string(b), `" `)
	return atoi(s)
}

func parseVMess(raw string) (*node.Node, error) {
	body := raw[len("vmess://"):]
	dec, ok := decodeBase64(body)
	if !ok {
		return nil, fmt.Errorf("bad base64 (non-v2rayN vmess links unsupported)")
	}
	var v vmessJSON
	if err := json.Unmarshal([]byte(dec), &v); err != nil {
		return nil, err
	}
	n := &node.Node{
		Name: v.PS, Type: node.VMess, Server: v.Add, Port: rawInt(v.Port),
		UUID: v.ID, AlterID: rawInt(v.Aid), Cipher: v.Scy,
	}
	if n.Cipher == "" {
		n.Cipher = "auto"
	}
	if v.TLS == "tls" {
		n.TLS = &node.TLS{Enabled: true, SNI: v.SNI, ALPN: splitList(v.ALPN), Fingerprint: v.FP,
			SkipVerify: truthy(strings.Trim(string(v.Insecure), `"`))}
	}
	switch v.Net {
	case "", "tcp":
		if v.Type == "http" {
			n.Transport = &node.Transport{Type: "http", Host: v.Host, Path: v.Path}
		}
	case "grpc":
		n.Transport = &node.Transport{Type: "grpc", ServiceName: v.Path, Mode: v.Type}
	default:
		n.Transport = &node.Transport{Type: v.Net, Host: v.Host, Path: v.Path}
	}
	return n, nil
}

func parseVLESS(raw string) (*node.Node, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, err
	}
	host, port, err := hostPort(u)
	if err != nil {
		return nil, err
	}
	q := u.Query()
	sec := q.Get("security")
	n := &node.Node{
		Name: fragmentName(u), Type: node.VLESS, Server: host, Port: port,
		UUID: u.User.Username(), Flow: q.Get("flow"),
		TLS:       tlsFromQuery(q, sec == "tls" || sec == "xtls"),
		Transport: transportFromQuery(q),
	}
	if e := q.Get("encryption"); e != "" && e != "none" {
		n.Cipher = e
	}
	return n, nil
}

func parseTrojan(raw string) (*node.Node, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, err
	}
	host, port, err := hostPort(u)
	if err != nil {
		return nil, err
	}
	q := u.Query()
	pw, _ := url.PathUnescape(u.User.String())
	n := &node.Node{
		Name: fragmentName(u), Type: node.Trojan, Server: host, Port: port, Password: pw,
		TLS: tlsFromQuery(q, true), Transport: transportFromQuery(q),
	}
	return n, nil
}

func parseHysteria2(raw string) (*node.Node, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, err
	}
	q := u.Query()
	host := u.Hostname()
	portStr := u.Port()
	ports := q.Get("mport")
	// hysteria2 allows "host:443,5000-6000" in the authority.
	if strings.ContainsAny(portStr, ",-") {
		ports = portStr
		portStr = strings.FieldsFunc(portStr, func(r rune) bool { return r == ',' || r == '-' })[0]
	}
	if portStr == "" {
		portStr = "443"
	}
	if host == "" {
		// url.Parse fails to split hosts with multi-port authority
		h, p, err := net.SplitHostPort(u.Host)
		if err != nil {
			return nil, fmt.Errorf("invalid host %q", u.Host)
		}
		host, portStr = h, p
	}
	pw := u.User.Username()
	if p, ok := u.User.Password(); ok {
		pw += ":" + p
	}
	n := &node.Node{
		Name: fragmentName(u), Type: node.Hysteria2, Server: host, Port: atoi(portStr), Ports: ports,
		Password: pw, TLS: tlsFromQuery(q, true),
	}
	if o := q.Get("obfs"); o != "" {
		n.Obfs = o
		n.ObfsPassword = q.Get("obfs-password")
	}
	n.UpMbps = atoi(q.Get("upmbps"))
	n.DownMbps = atoi(q.Get("downmbps"))
	return n, nil
}

func parseHysteria(raw string) (*node.Node, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, err
	}
	host, port, err := hostPort(u)
	if err != nil {
		return nil, err
	}
	q := u.Query()
	n := &node.Node{
		Name: fragmentName(u), Type: node.Hysteria, Server: host, Port: port,
		Password: first(q, "auth", "auth_str"), Protocol: q.Get("protocol"), Obfs: q.Get("obfsParam"),
		UpMbps: atoi(q.Get("upmbps")), DownMbps: atoi(q.Get("downmbps")), Ports: q.Get("mport"),
		TLS: tlsFromQuery(q, true),
	}
	return n, nil
}

func parseTUIC(raw string) (*node.Node, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, err
	}
	host, port, err := hostPort(u)
	if err != nil {
		return nil, err
	}
	q := u.Query()
	n := &node.Node{
		Name: fragmentName(u), Type: node.TUIC, Server: host, Port: port,
		CongestionControl: first(q, "congestion_control", "congestion-controller"),
		UDPRelayMode:      first(q, "udp_relay_mode", "udp-relay-mode"),
		TLS:               tlsFromQuery(q, true),
	}
	if pw, ok := u.User.Password(); ok {
		n.UUID, n.Password, n.Version = u.User.Username(), pw, 5
	} else {
		n.Token, n.Version = u.User.Username(), 4
	}
	return n, nil
}

func parseWireGuardURI(raw string) (*node.Node, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, err
	}
	host, port, err := hostPort(u)
	if err != nil {
		return nil, err
	}
	q := u.Query()
	pk, _ := url.PathUnescape(u.User.String())
	wg := &node.WireGuardOpts{PrivateKey: first(q, "privatekey", "private-key"), MTU: atoi(q.Get("mtu")), DNS: splitList(q.Get("dns"))}
	if wg.PrivateKey == "" {
		wg.PrivateKey = pk
	}
	for _, a := range splitList(first(q, "address", "ip")) {
		ip := strings.SplitN(a, "/", 2)[0]
		if strings.Contains(ip, ":") {
			wg.IPv6 = ip
		} else {
			wg.IP = ip
		}
	}
	peer := node.WireGuardPeer{
		PublicKey:    first(q, "publickey", "public-key", "peer"),
		PresharedKey: first(q, "presharedkey", "pre-shared-key", "psk"),
		AllowedIPs:   splitList(q.Get("allowedips")),
		Keepalive:    atoi(q.Get("keepalive")),
	}
	for _, r := range splitList(q.Get("reserved")) {
		peer.Reserved = append(peer.Reserved, atoi(r))
	}
	if len(peer.AllowedIPs) == 0 {
		peer.AllowedIPs = []string{"0.0.0.0/0", "::/0"}
	}
	wg.Peers = []node.WireGuardPeer{peer}
	return &node.Node{Name: fragmentName(u), Type: node.WireGuard, Server: host, Port: port, WireGuard: wg, UDP: node.BoolPtr(true)}, nil
}

func parseAnyTLS(raw string) (*node.Node, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, err
	}
	host, port, err := hostPort(u)
	if err != nil {
		return nil, err
	}
	pw, _ := url.PathUnescape(u.User.String())
	return &node.Node{Name: fragmentName(u), Type: node.AnyTLS, Server: host, Port: port, Password: pw,
		TLS: tlsFromQuery(u.Query(), true)}, nil
}

func parseSocks(raw string) (*node.Node, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, err
	}
	host, port, err := hostPort(u)
	if err != nil {
		return nil, err
	}
	n := &node.Node{Name: fragmentName(u), Type: node.SOCKS5, Server: host, Port: port}
	if u.User != nil {
		if pw, ok := u.User.Password(); ok {
			n.Username, n.Password = u.User.Username(), pw
		} else if dec, ok := decodeBase64(u.User.Username()); ok {
			n.Username, n.Password, _ = strings.Cut(dec, ":")
		}
	}
	if truthy(u.Query().Get("tls")) {
		n.TLS = tlsFromQuery(u.Query(), true)
	}
	return n, nil
}

func parseHTTPProxy(raw string) (*node.Node, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, err
	}
	if u.Port() == "" {
		return nil, fmt.Errorf("http proxy link requires explicit port")
	}
	host, port, err := hostPort(u)
	if err != nil {
		return nil, err
	}
	n := &node.Node{Name: fragmentName(u), Type: node.HTTP, Server: host, Port: port}
	if u.User != nil {
		n.Username = u.User.Username()
		n.Password, _ = u.User.Password()
	}
	if u.Scheme == "https" {
		n.TLS = tlsFromQuery(u.Query(), true)
	}
	return n, nil
}
