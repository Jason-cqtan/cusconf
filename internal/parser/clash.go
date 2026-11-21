package parser

import (
	"fmt"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/cusconf/cusconf/internal/node"
)

// m is a loosely typed YAML/JSON map with lenient accessors.
type m map[string]any

func (o m) str(keys ...string) string {
	for _, k := range keys {
		switch v := o[k].(type) {
		case string:
			if v != "" {
				return v
			}
		case int, int64, float64, bool:
			return fmt.Sprint(v)
		}
	}
	return ""
}

func (o m) int(keys ...string) int {
	for _, k := range keys {
		switch v := o[k].(type) {
		case int:
			return v
		case int64:
			return int(v)
		case float64:
			return int(v)
		case string:
			if i, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
				return i
			}
		}
	}
	return 0
}

func (o m) bool(keys ...string) bool {
	for _, k := range keys {
		switch v := o[k].(type) {
		case bool:
			return v
		case string:
			return truthy(v)
		case int:
			return v != 0
		}
	}
	return false
}

func (o m) boolPtr(k string) *bool {
	if _, ok := o[k]; !ok {
		return nil
	}
	return node.BoolPtr(o.bool(k))
}

func (o m) sub(k string) m {
	switch v := o[k].(type) {
	case map[string]any:
		return m(v)
	case m:
		return v
	}
	return m{}
}

func (o m) strs(k string) []string {
	switch v := o[k].(type) {
	case []any:
		var out []string
		for _, x := range v {
			out = append(out, fmt.Sprint(x))
		}
		return out
	case string:
		return splitList(v)
	}
	return nil
}

func (o m) strMap(k string) map[string]string {
	sm := o.sub(k)
	if len(sm) == 0 {
		return nil
	}
	out := map[string]string{}
	for key, v := range sm {
		switch vv := v.(type) {
		case []any:
			if len(vv) > 0 {
				out[key] = fmt.Sprint(vv[0])
			}
		default:
			out[key] = fmt.Sprint(vv)
		}
	}
	return out
}

func parseClash(text string) Result {
	r := Result{Format: FormatClash}
	var doc struct {
		Proxies []map[string]any `yaml:"proxies"`
	}
	if err := yaml.Unmarshal([]byte(text), &doc); err != nil {
		r.Errors = append(r.Errors, fmt.Errorf("clash yaml: %w", err))
		return r
	}
	for _, p := range doc.Proxies {
		n, err := clashProxy(m(p))
		if err != nil {
			r.Errors = append(r.Errors, err)
			continue
		}
		r.Nodes = append(r.Nodes, n)
	}
	return r
}

func clashTLS(p m, enabled bool) *node.TLS {
	t := &node.TLS{
		Enabled:     enabled,
		SNI:         p.str("servername", "sni"),
		ALPN:        p.strs("alpn"),
		SkipVerify:  p.bool("skip-cert-verify"),
		Fingerprint: p.str("client-fingerprint"),
		CertSHA256:  p.str("fingerprint", "server-cert-fingerprint"),
	}
	ro := p.sub("reality-opts")
	t.RealityPubKey = ro.str("public-key")
	t.RealityShortID = ro.str("short-id")
	if !t.Enabled && !t.Reality() && t.SNI == "" && !t.SkipVerify {
		return nil
	}
	return t
}

func clashTransport(p m) *node.Transport {
	netw := p.str("network")
	switch netw {
	case "", "tcp":
		return nil
	case "ws":
		o := p.sub("ws-opts")
		t := &node.Transport{Type: "ws", Path: o.str("path"), Headers: o.strMap("headers")}
		if t.Headers != nil {
			t.Host = t.Headers["Host"]
			delete(t.Headers, "Host")
			if len(t.Headers) == 0 {
				t.Headers = nil
			}
		}
		if o.bool("v2ray-http-upgrade") {
			t.Type = "httpupgrade"
		}
		return t
	case "grpc":
		return &node.Transport{Type: "grpc", ServiceName: p.sub("grpc-opts").str("grpc-service-name")}
	case "h2":
		o := p.sub("h2-opts")
		hosts := o.strs("host")
		t := &node.Transport{Type: "h2", Path: o.str("path")}
		if len(hosts) > 0 {
			t.Host = hosts[0]
		}
		return t
	case "http":
		o := p.sub("http-opts")
		t := &node.Transport{Type: "http"}
		if ps := o.strs("path"); len(ps) > 0 {
			t.Path = ps[0]
		}
		t.Host = o.strMap("headers")["Host"]
		return t
	case "xhttp":
		o := p.sub("xhttp-opts")
		return &node.Transport{Type: "xhttp", Path: o.str("path"), Host: o.str("host"), Mode: o.str("mode")}
	}
	return &node.Transport{Type: netw}
}

func clashProxy(p m) (*node.Node, error) {
	n := &node.Node{
		Name:   p.str("name"),
		Type:   node.Type(p.str("type")),
		Server: p.str("server"),
		Port:   p.int("port"),
		UDP:    p.boolPtr("udp"),
		TFO:    p.bool("tfo", "fast-open"),
	}
	switch n.Type {
	case node.SS:
		n.Cipher, n.Password = p.str("cipher"), p.str("password")
		if pl := p.str("plugin"); pl != "" {
			n.Plugin = pl
			n.PluginOpts = p.strMap("plugin-opts")
		}
	case node.SSR:
		n.Cipher, n.Password = p.str("cipher"), p.str("password")
		n.Protocol, n.ProtocolParam = p.str("protocol"), p.str("protocol-param")
		n.Obfs, n.ObfsParam = p.str("obfs"), p.str("obfs-param")
	case node.VMess:
		n.UUID, n.AlterID, n.Cipher = p.str("uuid"), p.int("alterId"), p.str("cipher")
		n.TLS, n.Transport = clashTLS(p, p.bool("tls")), clashTransport(p)
	case node.VLESS:
		n.UUID, n.Flow = p.str("uuid"), p.str("flow")
		if e := p.str("encryption"); e != "" && e != "none" {
			n.Cipher = e
		}
		n.TLS, n.Transport = clashTLS(p, p.bool("tls")), clashTransport(p)
	case node.Trojan:
		n.Password = p.str("password")
		n.TLS, n.Transport = clashTLS(p, true), clashTransport(p)
	case node.Hysteria2:
		n.Password = p.str("password", "auth")
		n.Ports = p.str("ports")
		n.HopInterval = p.int("hop-interval")
		n.Obfs, n.ObfsPassword = p.str("obfs"), p.str("obfs-password")
		n.UpMbps, n.DownMbps = mbps(p.str("up", "up-speed")), mbps(p.str("down", "down-speed"))
		n.TLS = clashTLS(p, true)
	case node.Hysteria:
		n.Password = p.str("auth-str", "auth_str", "auth")
		n.Protocol, n.Obfs, n.Ports = p.str("protocol"), p.str("obfs"), p.str("ports")
		n.UpMbps, n.DownMbps = mbps(p.str("up", "up-speed")), mbps(p.str("down", "down-speed"))
		n.TLS = clashTLS(p, true)
	case node.TUIC:
		n.Token, n.UUID, n.Password = p.str("token"), p.str("uuid"), p.str("password")
		n.Version = 5
		if n.Token != "" {
			n.Version = 4
		}
		n.CongestionControl = p.str("congestion-controller")
		n.UDPRelayMode = p.str("udp-relay-mode")
		n.TLS = clashTLS(p, true)
	case node.WireGuard:
		wg := &node.WireGuardOpts{PrivateKey: p.str("private-key"), IP: p.str("ip"), IPv6: p.str("ipv6"),
			DNS: p.strs("dns"), MTU: p.int("mtu")}
		peer := node.WireGuardPeer{PublicKey: p.str("public-key"), PresharedKey: p.str("pre-shared-key", "preshared-key"),
			AllowedIPs: p.strs("allowed-ips"), Keepalive: p.int("persistent-keepalive", "keepalive")}
		if rs, ok := p["reserved"].([]any); ok {
			for _, x := range rs {
				peer.Reserved = append(peer.Reserved, m{"v": x}.int("v"))
			}
		}
		if len(peer.AllowedIPs) == 0 {
			peer.AllowedIPs = []string{"0.0.0.0/0", "::/0"}
		}
		wg.Peers = []node.WireGuardPeer{peer}
		n.WireGuard = wg
	case node.Snell:
		n.Password, n.Version = p.str("psk"), p.int("version")
		o := p.sub("obfs-opts")
		n.Obfs, n.ObfsHost = o.str("mode"), o.str("host")
	case node.AnyTLS:
		n.Password = p.str("password")
		n.TLS = clashTLS(p, true)
	case node.HTTP, node.SOCKS5:
		n.Username, n.Password = p.str("username"), p.str("password")
		if p.bool("tls") {
			n.TLS = clashTLS(p, true)
		}
	case "socks":
		n.Type = node.SOCKS5
	default:
		return nil, fmt.Errorf("clash: unsupported proxy type %q (%s)", n.Type, n.Name)
	}
	return n, nil
}

// mbps parses "100", "100 Mbps", "100mbps".
func mbps(s string) int {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.TrimSpace(strings.TrimSuffix(strings.TrimSuffix(s, "mbps"), "m"))
	return atoi(s)
}
