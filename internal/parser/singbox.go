package parser

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/cusconf/cusconf/internal/node"
)

func parseSingBox(text string) Result {
	r := Result{Format: FormatSingBox}
	var doc struct {
		Outbounds []map[string]any `json:"outbounds"`
	}
	if err := json.Unmarshal([]byte(text), &doc); err != nil {
		r.Errors = append(r.Errors, fmt.Errorf("sing-box json: %w", err))
		return r
	}
	for _, raw := range doc.Outbounds {
		o := m(raw)
		n, err := singBoxOutbound(o)
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

func singBoxOutbound(o m) (*node.Node, error) {
	n := &node.Node{Name: o.str("tag"), Server: o.str("server"), Port: o.int("server_port")}
	tlsO := o.sub("tls")
	tlsFn := func() *node.TLS {
		if !tlsO.bool("enabled") {
			return nil
		}
		re := tlsO.sub("reality")
		return &node.TLS{Enabled: true, SNI: tlsO.str("server_name"), ALPN: tlsO.strs("alpn"),
			SkipVerify: tlsO.bool("insecure"), Fingerprint: tlsO.sub("utls").str("fingerprint"),
			RealityPubKey: re.str("public_key"), RealityShortID: re.str("short_id")}
	}
	trFn := func() *node.Transport {
		t := o.sub("transport")
		typ := t.str("type")
		if typ == "" {
			return nil
		}
		tr := &node.Transport{Type: typ, Path: t.str("path"), ServiceName: t.str("service_name")}
		if h := t.strMap("headers"); h != nil {
			tr.Host = h["Host"]
		}
		if typ == "http" {
			if hs := t.strs("host"); len(hs) > 0 {
				tr.Host = hs[0]
			}
		}
		return tr
	}
	switch o.str("type") {
	case "shadowsocks":
		n.Type, n.Cipher, n.Password = node.SS, o.str("method"), o.str("password")
		if pl := o.str("plugin"); pl != "" {
			n.Plugin = map[string]string{"obfs-local": "obfs"}[pl]
			if n.Plugin == "" {
				n.Plugin = pl
			}
			n.PluginOpts = map[string]string{"raw": o.str("plugin_opts")}
		}
	case "vmess":
		n.Type, n.UUID, n.AlterID, n.Cipher = node.VMess, o.str("uuid"), o.int("alter_id"), o.str("security")
		n.TLS, n.Transport = tlsFn(), trFn()
	case "vless":
		n.Type, n.UUID, n.Flow = node.VLESS, o.str("uuid"), o.str("flow")
		n.TLS, n.Transport = tlsFn(), trFn()
	case "trojan":
		n.Type, n.Password = node.Trojan, o.str("password")
		n.TLS, n.Transport = tlsFn(), trFn()
	case "hysteria2":
		n.Type, n.Password = node.Hysteria2, o.str("password")
		ob := o.sub("obfs")
		n.Obfs, n.ObfsPassword = ob.str("type"), ob.str("password")
		n.UpMbps, n.DownMbps = o.int("up_mbps"), o.int("down_mbps")
		n.TLS = tlsFn()
	case "tuic":
		n.Type, n.UUID, n.Password, n.Version = node.TUIC, o.str("uuid"), o.str("password"), 5
		n.CongestionControl, n.UDPRelayMode = o.str("congestion_control"), o.str("udp_relay_mode")
		n.TLS = tlsFn()
	case "anytls":
		n.Type, n.Password = node.AnyTLS, o.str("password")
		n.TLS = tlsFn()
	case "socks":
		n.Type, n.Username, n.Password = node.SOCKS5, o.str("username"), o.str("password")
	case "http":
		n.Type, n.Username, n.Password = node.HTTP, o.str("username"), o.str("password")
		n.TLS = tlsFn()
	case "wireguard":
		n.Type = node.WireGuard
		wg := &node.WireGuardOpts{PrivateKey: o.str("private_key"), MTU: o.int("mtu")}
		for _, a := range o.strs("local_address") {
			ip, _, _ := strings.Cut(a, "/")
			if strings.Contains(ip, ":") {
				wg.IPv6 = ip
			} else {
				wg.IP = ip
			}
		}
		p := node.WireGuardPeer{PublicKey: o.str("peer_public_key"), PresharedKey: o.str("pre_shared_key"),
			AllowedIPs: []string{"0.0.0.0/0", "::/0"}}
		wg.Peers = []node.WireGuardPeer{p}
		n.WireGuard = wg
	case "direct", "block", "dns", "selector", "urltest":
		return nil, nil
	default:
		return nil, fmt.Errorf("sing-box: unsupported outbound type %q (%s)", o.str("type"), n.Name)
	}
	return n, nil
}
