package parser

import (
	"os"
	"testing"

	"github.com/cusconf/cusconf/internal/node"
)

func load(t *testing.T, f string) Result {
	t.Helper()
	b, err := os.ReadFile("testdata/" + f)
	if err != nil {
		t.Fatal(err)
	}
	return Parse(b)
}

func byName(nodes []*node.Node) map[string]*node.Node {
	out := map[string]*node.Node{}
	for _, n := range nodes {
		out[n.Name] = n
	}
	return out
}

func TestURIList(t *testing.T) {
	for _, f := range []string{"uri.txt", "uri.b64"} {
		r := load(t, f)
		if r.Format != FormatURI {
			t.Fatalf("%s: format %s", f, r.Format)
		}
		for _, e := range r.Errors {
			t.Errorf("%s: %v", f, e)
		}
		if len(r.Nodes) != 14 {
			t.Fatalf("%s: got %d nodes", f, len(r.Nodes))
		}
		n := byName(r.Nodes)

		ss := n["🇭🇰 HK 01"]
		if ss == nil || ss.Cipher != "aes-256-gcm" || ss.Password != "password" || ss.Port != 8388 {
			t.Errorf("ss sip002: %+v", ss)
		}
		ss2 := n["HK 02"]
		if ss2.Cipher != "2022-blake3-aes-128-gcm" || ss2.Password != "YctPZ6U7xPPcU+gp3u+I3w==" || ss2.Plugin != "obfs" || ss2.PluginOpts["host"] != "bing.com" {
			t.Errorf("ss2022: %+v", ss2)
		}
		if l := n["Legacy"]; l.Server != "192.168.100.1" || l.Password != "test" {
			t.Errorf("legacy ss: %+v", l)
		}
		vm := n["JP VMess"]
		if vm.Type != node.VMess || vm.TransportType() != "ws" || vm.Transport.Path != "/ws" || !vm.TLSEnabled() || vm.TLS.SNI != "jp.example.com" {
			t.Errorf("vmess: %+v", vm)
		}
		vl := n["US Reality"]
		if vl.Flow != "xtls-rprx-vision" || !vl.TLS.Reality() || vl.TLS.RealityShortID != "0123abcd" || vl.TLS.Fingerprint != "chrome" || vl.Transport != nil {
			t.Errorf("vless reality: %+v %+v", vl, vl.TLS)
		}
		if g := n["SG gRPC"]; g.TransportType() != "grpc" || g.Transport.ServiceName != "grpcsvc" {
			t.Errorf("vless grpc: %+v", g.Transport)
		}
		if tr := n["TW Trojan"]; tr.Password != "p@ss" || !tr.TLS.SkipVerify {
			t.Errorf("trojan: %+v", tr)
		}
		if h := n["HY2"]; h.Ports != "20000-30000" || h.Obfs != "salamander" || h.ObfsPassword != "ob" {
			t.Errorf("hy2: %+v", h)
		}
		if h := n["HY2b"]; h.Port != 8443 || !h.TLS.SkipVerify {
			t.Errorf("hy2b: %+v", h)
		}
		if tu := n["TUIC5"]; tu.Version != 5 || tu.Password != "tuicpass" || tu.CongestionControl != "bbr" {
			t.Errorf("tuic: %+v", tu)
		}
		wg := n["WG"]
		if wg.WireGuard.PrivateKey != "cHJpdmF0ZWtleQ==" || wg.WireGuard.IP != "10.0.0.2" || wg.WireGuard.IPv6 != "fd00::2" || len(wg.WireGuard.Peers[0].Reserved) != 3 {
			t.Errorf("wg: %+v", wg.WireGuard)
		}
		if s := n["SSR Node"]; s == nil || s.Protocol != "auth_aes128_md5" || s.Obfs != "tls1.2_ticket_auth" || s.Password != "pass" {
			t.Errorf("ssr: %+v", s)
		}
		if s := n["Socks"]; s.Username != "user" || s.Password != "pass" {
			t.Errorf("socks: %+v", s)
		}
	}
}

func TestClash(t *testing.T) {
	r := load(t, "clash.yaml")
	if r.Format != FormatClash {
		t.Fatal(r.Format)
	}
	if len(r.Errors) != 1 { // mieru unsupported
		t.Errorf("errors: %v", r.Errors)
	}
	n := byName(r.Nodes)
	if len(n) != 7 {
		t.Fatalf("got %d", len(n))
	}
	if s := n["HK SS"]; s.Plugin != "obfs" || s.PluginOpts["mode"] != "tls" || s.UDP == nil || !*s.UDP {
		t.Errorf("ss: %+v", s)
	}
	if v := n["JP VMess"]; v.Transport.Host != "jp.example.com" || v.Transport.Headers != nil {
		t.Errorf("vmess ws: %+v", v.Transport)
	}
	if v := n["US Reality"]; v.TLS.RealityPubKey != "PK" || v.TLS.RealityShortID != "01ab" {
		t.Errorf("reality: %+v", v.TLS)
	}
	if h := n["HY2"]; h.UpMbps != 50 || h.DownMbps != 200 || h.Ports != "20000-30000" {
		t.Errorf("hy2: %+v", h)
	}
	if w := n["WG"]; w.WireGuard.Peers[0].PublicKey != "pub" || len(w.WireGuard.Peers[0].Reserved) != 3 {
		t.Errorf("wg: %+v", w.WireGuard)
	}
	if s := n["Snell"]; s.Version != 4 || s.Obfs != "http" {
		t.Errorf("snell: %+v", s)
	}
}

func TestSurge(t *testing.T) {
	r := load(t, "surge.conf")
	if r.Format != FormatSurge {
		t.Fatal(r.Format)
	}
	for _, e := range r.Errors {
		t.Error(e)
	}
	n := byName(r.Nodes)
	if len(n) != 5 {
		t.Fatalf("got %d: %v", len(n), n)
	}
	if v := n["JP VMess"]; v.Transport.Host != "jp.example.com" || v.AlterID != 0 || !v.TLSEnabled() {
		t.Errorf("vmess: %+v", v)
	}
	if v := n["US Reality"]; !v.TLS.Reality() || v.UUID != "uuid-1" {
		t.Errorf("vless: %+v", v)
	}
	if h := n["HY2"]; h.Ports != "20000-30000,40000" || h.DownMbps != 100 {
		t.Errorf("hy2: %+v", h)
	}
	w := n["Office"]
	if w.Server != "wg.example.com" || w.Port != 51820 || len(w.WireGuard.Peers) != 1 ||
		len(w.WireGuard.Peers[0].AllowedIPs) != 2 || w.WireGuard.Peers[0].Keepalive != 25 || len(w.WireGuard.Peers[0].Reserved) != 3 {
		t.Errorf("wg: %+v %+v", w, w.WireGuard)
	}
}

func TestDuplicateNames(t *testing.T) {
	r := Parse([]byte("trojan://a@h:443#X\ntrojan://b@h:443#X"))
	if r.Nodes[0].Name != "X" || r.Nodes[1].Name != "X 2" {
		t.Errorf("%s %s", r.Nodes[0].Name, r.Nodes[1].Name)
	}
}
