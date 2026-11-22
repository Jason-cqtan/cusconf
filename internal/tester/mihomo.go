package tester

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/cusconf/cusconf/internal/node"
)

// ToMihomo converts a node to a mihomo (Clash.Meta) proxy mapping, written into
// the config of the mihomo core process (see core.go).
func ToMihomo(n *node.Node) (map[string]any, error) {
	m := map[string]any{"name": n.Name, "type": string(n.Type), "server": n.Server, "port": n.Port, "udp": true}
	if n.TFO {
		m["tfo"] = true
	}
	setTLS := func(sniKey string) {
		t := n.TLS
		if t == nil {
			return
		}
		if t.SNI != "" {
			m[sniKey] = t.SNI
		}
		if t.SkipVerify {
			m["skip-cert-verify"] = true
		}
		if len(t.ALPN) > 0 {
			m["alpn"] = t.ALPN
		}
		if t.Fingerprint != "" {
			m["client-fingerprint"] = t.Fingerprint
		}
		if t.CertSHA256 != "" {
			m["fingerprint"] = t.CertSHA256
		}
		if t.Reality() {
			m["reality-opts"] = map[string]any{"public-key": t.RealityPubKey, "short-id": t.RealityShortID}
		}
	}
	setTransport := func() {
		tr := n.Transport
		if tr == nil || tr.Type == "" || tr.Type == "tcp" {
			return
		}
		headers := map[string]any{}
		for k, v := range tr.Headers {
			headers[k] = v
		}
		if tr.Host != "" {
			headers["Host"] = tr.Host
		}
		switch tr.Type {
		case "ws", "httpupgrade":
			m["network"] = "ws"
			o := map[string]any{"path": or(tr.Path, "/"), "headers": headers}
			if tr.Type == "httpupgrade" {
				o["v2ray-http-upgrade"] = true
			}
			m["ws-opts"] = o
		case "grpc":
			m["network"] = "grpc"
			m["grpc-opts"] = map[string]any{"grpc-service-name": tr.ServiceName}
		case "h2":
			m["network"] = "h2"
			o := map[string]any{"path": or(tr.Path, "/")}
			if tr.Host != "" {
				o["host"] = []string{tr.Host}
			}
			m["h2-opts"] = o
		case "http":
			m["network"] = "http"
			o := map[string]any{"path": []string{or(tr.Path, "/")}}
			if tr.Host != "" {
				o["headers"] = map[string]any{"Host": []string{tr.Host}}
			}
			m["http-opts"] = o
		case "xhttp":
			m["network"] = "xhttp"
			m["xhttp-opts"] = map[string]any{"path": or(tr.Path, "/"), "host": tr.Host, "mode": tr.Mode}
		default:
			m["network"] = tr.Type
		}
	}
	ports := func() {
		if n.Ports != "" {
			m["ports"] = n.Ports
			if n.HopInterval > 0 {
				m["hop-interval"] = n.HopInterval
			}
		}
	}

	switch n.Type {
	case node.SS:
		m["cipher"], m["password"] = n.Cipher, n.Password
		if n.Plugin != "" {
			opts := map[string]any{}
			for k, v := range n.PluginOpts {
				opts[k] = v
			}
			switch n.Plugin {
			case "v2ray-plugin":
				opts["tls"] = n.PluginOpts["tls"] == "true"
			case "shadow-tls":
				if v, err := strconv.Atoi(n.PluginOpts["version"]); err == nil {
					opts["version"] = v
				}
			}
			m["plugin"], m["plugin-opts"] = n.Plugin, opts
		}
	case node.SSR:
		m["cipher"], m["password"] = n.Cipher, n.Password
		m["protocol"], m["protocol-param"] = n.Protocol, n.ProtocolParam
		m["obfs"], m["obfs-param"] = n.Obfs, n.ObfsParam
	case node.VMess:
		m["uuid"], m["alterId"], m["cipher"] = n.UUID, n.AlterID, or(n.Cipher, "auto")
		m["tls"] = n.TLSEnabled()
		setTLS("servername")
		setTransport()
	case node.VLESS:
		m["uuid"] = n.UUID
		if n.Flow != "" {
			m["flow"] = n.Flow
		}
		if n.Cipher != "" {
			m["encryption"] = n.Cipher
		}
		m["tls"] = n.TLSEnabled()
		setTLS("servername")
		setTransport()
	case node.Trojan:
		m["password"] = n.Password
		setTLS("sni")
		setTransport()
	case node.Hysteria2:
		m["password"] = n.Password
		if n.Obfs != "" {
			m["obfs"], m["obfs-password"] = n.Obfs, n.ObfsPassword
		}
		if n.UpMbps > 0 {
			m["up"] = fmt.Sprintf("%d Mbps", n.UpMbps)
		}
		if n.DownMbps > 0 {
			m["down"] = fmt.Sprintf("%d Mbps", n.DownMbps)
		}
		ports()
		setTLS("sni")
	case node.Hysteria:
		m["auth-str"], m["protocol"], m["obfs"] = n.Password, n.Protocol, n.Obfs
		m["up"], m["down"] = fmt.Sprintf("%d Mbps", max(n.UpMbps, 10)), fmt.Sprintf("%d Mbps", max(n.DownMbps, 50))
		ports()
		setTLS("sni")
	case node.TUIC:
		if n.Version == 4 {
			m["token"] = n.Token
		} else {
			m["uuid"], m["password"] = n.UUID, n.Password
		}
		if n.CongestionControl != "" {
			m["congestion-controller"] = n.CongestionControl
		}
		if n.UDPRelayMode != "" {
			m["udp-relay-mode"] = n.UDPRelayMode
		}
		setTLS("sni")
		if _, ok := m["alpn"]; !ok {
			m["alpn"] = []string{"h3"}
		}
	case node.WireGuard:
		wg := n.WireGuard
		p := wg.Peers[0]
		m["private-key"], m["public-key"] = wg.PrivateKey, p.PublicKey
		if wg.IP != "" {
			m["ip"] = wg.IP
		}
		if wg.IPv6 != "" {
			m["ipv6"] = wg.IPv6
		}
		if p.PresharedKey != "" {
			m["pre-shared-key"] = p.PresharedKey
		}
		if len(p.Reserved) == 3 {
			m["reserved"] = p.Reserved
		}
		if wg.MTU > 0 {
			m["mtu"] = wg.MTU
		}
	case node.Snell:
		m["psk"] = n.Password
		if n.Version > 0 {
			m["version"] = n.Version
		}
		if n.Obfs != "" {
			m["obfs-opts"] = map[string]any{"mode": n.Obfs, "host": or(n.ObfsHost, "bing.com")}
		}
	case node.AnyTLS:
		m["password"] = n.Password
		setTLS("sni")
	case node.HTTP, node.SOCKS5:
		if n.Type == node.SOCKS5 {
			m["type"] = "socks5"
		}
		if n.Username != "" {
			m["username"], m["password"] = n.Username, n.Password
		}
		if n.TLSEnabled() {
			m["tls"] = true
			setTLS("sni")
		}
	default:
		return nil, fmt.Errorf("%s not supported by the tester", n.Type)
	}
	return m, nil
}

func or(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// Proxy is a dialable node: a SOCKS5 listener of the mihomo core process that
// forwards everything through that node.
type Proxy struct{ addr string }

// Dial opens a TCP connection to addr ("host:port") through the node.
// The host name is resolved on the remote side.
func (p *Proxy) Dial(ctx context.Context, addr string) (net.Conn, error) {
	return socks5Dial(ctx, p.addr, addr)
}

// Client returns an HTTP client that sends every request through the node.
// Redirects are not followed unless follow is true.
func (p *Proxy) Client(timeout time.Duration, follow bool) *http.Client {
	tr := &http.Transport{
		DialContext: func(ctx context.Context, _, addr string) (net.Conn, error) {
			return p.Dial(ctx, addr)
		},
		TLSClientConfig:       &tls.Config{},
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: timeout,
		MaxIdleConnsPerHost:   2,
		IdleConnTimeout:       30 * time.Second,
		ForceAttemptHTTP2:     true,
	}
	c := &http.Client{Transport: tr, Timeout: timeout}
	if !follow {
		c.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	}
	return c
}

// Latency measures an HTTP round trip through the node, like Surge/Clash url-test:
// one warm-up request to establish the tunnel, then the timed request on the same connection.
func (p *Proxy) Latency(ctx context.Context, url string, timeout time.Duration) (time.Duration, error) {
	c := p.Client(timeout, false)
	defer c.CloseIdleConnections()
	do := func() (time.Duration, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodHead, url, nil)
		if err != nil {
			return 0, err
		}
		start := time.Now()
		resp, err := c.Do(req)
		if err != nil {
			return 0, err
		}
		resp.Body.Close()
		return time.Since(start), nil
	}
	if _, err := do(); err != nil {
		return 0, err
	}
	return do()
}
