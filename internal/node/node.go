// Package node defines the unified intermediate representation (IR) for proxy
// nodes. Every parser produces Nodes; every renderer consumes them.
package node

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"strings"
)

type Type string

const (
	SS        Type = "ss"
	SSR       Type = "ssr"
	VMess     Type = "vmess"
	VLESS     Type = "vless"
	Trojan    Type = "trojan"
	Hysteria  Type = "hysteria"
	Hysteria2 Type = "hysteria2"
	TUIC      Type = "tuic"
	WireGuard Type = "wireguard"
	Snell     Type = "snell"
	AnyTLS    Type = "anytls"
	HTTP      Type = "http"
	SOCKS5    Type = "socks5"
)

// TLS holds TLS / REALITY settings.
type TLS struct {
	Enabled        bool     `json:"enabled,omitempty"`
	SNI            string   `json:"sni,omitempty"`
	ALPN           []string `json:"alpn,omitempty"`
	SkipVerify     bool     `json:"skipVerify,omitempty"`
	Fingerprint    string   `json:"fingerprint,omitempty"`    // client fingerprint (uTLS), e.g. chrome
	CertSHA256     string   `json:"certSha256,omitempty"`     // server cert pinning
	RealityPubKey  string   `json:"realityPubKey,omitempty"`  // REALITY public key
	RealityShortID string   `json:"realityShortId,omitempty"` // REALITY short id
}

func (t *TLS) Reality() bool { return t != nil && t.RealityPubKey != "" }

// Transport describes the stream transport layer.
type Transport struct {
	Type        string            `json:"type,omitempty"` // tcp(default) | ws | grpc | h2 | http | httpupgrade | xhttp
	Path        string            `json:"path,omitempty"`
	Host        string            `json:"host,omitempty"`
	Headers     map[string]string `json:"headers,omitempty"`
	ServiceName string            `json:"serviceName,omitempty"` // grpc
	Mode        string            `json:"mode,omitempty"`        // grpc mode / xhttp mode
}

// WireGuardPeer is a WireGuard peer.
type WireGuardPeer struct {
	PublicKey    string   `json:"publicKey"`
	PresharedKey string   `json:"presharedKey,omitempty"`
	AllowedIPs   []string `json:"allowedIps,omitempty"`
	Reserved     []int    `json:"reserved,omitempty"`
	Keepalive    int      `json:"keepalive,omitempty"`
}

// WireGuardOpts holds the interface settings of a WireGuard node.
// Server/Port of the Node is the (first) peer endpoint.
type WireGuardOpts struct {
	PrivateKey string          `json:"privateKey"`
	IP         string          `json:"ip,omitempty"`
	IPv6       string          `json:"ipv6,omitempty"`
	DNS        []string        `json:"dns,omitempty"`
	MTU        int             `json:"mtu,omitempty"`
	Peers      []WireGuardPeer `json:"peers"`
}

// Node is a single proxy endpoint.
type Node struct {
	Name   string `json:"name"`
	Type   Type   `json:"type"`
	Server string `json:"server"`
	Port   int    `json:"port"`
	// Ports is a port-hopping spec ("443,5000-6000") for QUIC protocols.
	Ports string `json:"ports,omitempty"`

	UDP *bool `json:"udp,omitempty"`
	TFO bool  `json:"tfo,omitempty"`

	// Credentials. Which fields are meaningful depends on Type.
	Username string `json:"username,omitempty"` // http/socks5
	Password string `json:"password,omitempty"` // ss/ssr/trojan/hy2/tuic-v5/anytls/http/socks5, snell psk
	UUID     string `json:"uuid,omitempty"`     // vmess/vless/tuic
	Cipher   string `json:"cipher,omitempty"`   // ss/ssr/vmess security
	AlterID  int    `json:"alterId,omitempty"`  // vmess
	Flow     string `json:"flow,omitempty"`     // vless
	Version  int    `json:"version,omitempty"`  // snell / tuic version

	// Shadowsocks plugin, e.g. "obfs" with {"mode":"http","host":"x"} or "v2ray-plugin" / "shadow-tls".
	Plugin     string            `json:"plugin,omitempty"`
	PluginOpts map[string]string `json:"pluginOpts,omitempty"`

	// SSR
	Protocol      string `json:"protocol,omitempty"`
	ProtocolParam string `json:"protocolParam,omitempty"`
	Obfs          string `json:"obfs,omitempty"` // also hysteria2 obfs type (salamander), snell obfs mode
	ObfsParam     string `json:"obfsParam,omitempty"`
	// ObfsPassword: hysteria2 salamander password; ObfsHost: snell obfs host.
	ObfsPassword string `json:"obfsPassword,omitempty"`
	ObfsHost     string `json:"obfsHost,omitempty"`

	// Hysteria / TUIC
	UpMbps            int    `json:"upMbps,omitempty"`
	DownMbps          int    `json:"downMbps,omitempty"`
	CongestionControl string `json:"congestionControl,omitempty"`
	UDPRelayMode      string `json:"udpRelayMode,omitempty"`
	HopInterval       int    `json:"hopInterval,omitempty"`
	Token             string `json:"token,omitempty"` // tuic v4

	TLS       *TLS           `json:"tls,omitempty"`
	Transport *Transport     `json:"transport,omitempty"`
	WireGuard *WireGuardOpts `json:"wireguard,omitempty"`

	// Metadata (not part of the endpoint definition).
	Source     string   `json:"source,omitempty"`     // source id
	SelfHosted bool     `json:"selfHosted,omitempty"` // from a self-hosted source
	Tags       []string `json:"tags,omitempty"`
	Probe      *Probe   `json:"probe,omitempty"` // latest test result summary
}

// Key returns a stable identity hash of the endpoint (ignores name/metadata),
// used for de-duplication and for persisting per-node test results.
func (n *Node) Key() string {
	cred := n.Password + "|" + n.UUID + "|" + n.Token
	if n.WireGuard != nil {
		cred += "|" + n.WireGuard.PrivateKey
	}
	h := sha1.Sum([]byte(fmt.Sprintf("%s|%s|%d|%s", n.Type, strings.ToLower(n.Server), n.Port, cred)))
	return hex.EncodeToString(h[:8])
}

// TransportType returns the normalized transport type ("tcp" when unset).
func (n *Node) TransportType() string {
	if n.Transport == nil || n.Transport.Type == "" {
		return "tcp"
	}
	return n.Transport.Type
}

// TLSEnabled reports whether the node uses TLS (or REALITY).
func (n *Node) TLSEnabled() bool {
	return n.TLS != nil && (n.TLS.Enabled || n.TLS.Reality())
}

// Validate checks the minimum fields for the node type.
func (n *Node) Validate() error {
	if n.Name == "" {
		return fmt.Errorf("empty name")
	}
	if n.Type != WireGuard && (n.Server == "" || n.Port <= 0 || n.Port > 65535) {
		return fmt.Errorf("%s: invalid server/port", n.Name)
	}
	switch n.Type {
	case SS:
		if n.Cipher == "" || n.Password == "" {
			return fmt.Errorf("%s: ss requires cipher and password", n.Name)
		}
	case SSR:
		if n.Cipher == "" || n.Password == "" || n.Protocol == "" || n.Obfs == "" {
			return fmt.Errorf("%s: ssr requires cipher/password/protocol/obfs", n.Name)
		}
	case VMess, VLESS:
		if n.UUID == "" {
			return fmt.Errorf("%s: %s requires uuid", n.Name, n.Type)
		}
	case Trojan, Hysteria2, AnyTLS, Snell:
		if n.Password == "" {
			return fmt.Errorf("%s: %s requires password", n.Name, n.Type)
		}
	case TUIC:
		if n.Token == "" && (n.UUID == "" || n.Password == "") {
			return fmt.Errorf("%s: tuic requires token (v4) or uuid+password (v5)", n.Name)
		}
	case WireGuard:
		if n.WireGuard == nil || n.WireGuard.PrivateKey == "" || len(n.WireGuard.Peers) == 0 {
			return fmt.Errorf("%s: wireguard requires private key and a peer", n.Name)
		}
	case Hysteria, HTTP, SOCKS5:
	default:
		return fmt.Errorf("%s: unknown type %q", n.Name, n.Type)
	}
	return nil
}

func BoolPtr(b bool) *bool { return &b }

// Probe is the latest test summary for a node. It is attached when nodes are
// loaded for filtering and display, and is not part of the stored node.
type Probe struct {
	Latency     int                `json:"latency"` // ms; 0 = unreachable
	TestedAt    int64              `json:"testedAt"`
	ExitCountry string             `json:"exitCountry,omitempty"`
	Scores      map[string]float64 `json:"scores,omitempty"`   // exit IP safety 0–100 per source, higher is safer
	Unlocked    []string           `json:"unlocked,omitempty"` // service IDs fully available
	Partial     []string           `json:"partial,omitempty"`  // service IDs partially available
	Flags       []string           `json:"flags,omitempty"`    // exit IP labels, e.g. 机房, 滥用
}
