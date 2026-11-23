package render

import (
	"strings"

	"github.com/cusconf/cusconf/internal/capability"
)

// ProtocolValues are the PROTOCOL rule values the editor offers (Surge names, plus
// TLS and MTPROTO from Stash). Each client takes the subset in protocolMap.
var ProtocolValues = []string{"HTTP", "HTTPS", "TLS", "TCP", "UDP", "QUIC", "STUN", "DOH", "DOH3", "DOQ", "MTPROTO"}

// protocolMap maps a PROTOCOL value to the client's rule as "TYPE,VALUE".
// Sources: manual.nssurge.com (rule), nsloon.app/docs/Rule/protocol_rule,
// stash.wiki/rules/rule-types, wiki.metacubex.one (NETWORK only),
// sing-box.sagernet.org route rule (network / sniffed protocol).
var protocolMap = map[capability.Client]map[string]string{
	capability.Surge: {
		"HTTP": "PROTOCOL,HTTP", "HTTPS": "PROTOCOL,HTTPS", "TLS": "PROTOCOL,HTTPS", "TCP": "PROTOCOL,TCP", "UDP": "PROTOCOL,UDP",
		"QUIC": "PROTOCOL,QUIC", "STUN": "PROTOCOL,STUN", "DOH": "PROTOCOL,DOH", "DOH3": "PROTOCOL,DOH3", "DOQ": "PROTOCOL,DOQ",
	},
	capability.Loon: {
		"HTTP": "PROTOCOL,HTTP", "HTTPS": "PROTOCOL,HTTPS", "TLS": "PROTOCOL,HTTPS", "TCP": "PROTOCOL,TCP", "UDP": "PROTOCOL,UDP",
		"QUIC": "PROTOCOL,QUIC", "STUN": "PROTOCOL,STUN",
	},
	capability.Stash: {
		"HTTP": "PROTOCOL,HTTP", "HTTPS": "PROTOCOL,TLS", "TLS": "PROTOCOL,TLS", "TCP": "PROTOCOL,TCP", "UDP": "PROTOCOL,UDP",
		"QUIC": "PROTOCOL,QUIC", "STUN": "PROTOCOL,STUN", "MTPROTO": "PROTOCOL,MTPROTO",
	},
	// mihomo has no protocol sniffing rule, only NETWORK.
	capability.ClashMeta: {"TCP": "NETWORK,tcp", "UDP": "NETWORK,udp"},
	capability.FlClash:   {"TCP": "NETWORK,tcp", "UDP": "NETWORK,udp"},
	// sing-box: TCP/UDP match the network, the rest the sniffed protocol (the route sniffs first).
	capability.SingBox: {
		"TCP": "NETWORK,tcp", "UDP": "NETWORK,udp", "HTTP": "PROTOCOL,http", "HTTPS": "PROTOCOL,tls", "TLS": "PROTOCOL,tls",
		"QUIC": "PROTOCOL,quic", "STUN": "PROTOCOL,stun",
	},
}

// ProtocolRule converts a PROTOCOL rule value for the target client. It returns the
// rule type and value to write, or ok=false after adding a warning when the client
// cannot express it.
func ProtocolRule(c *Config, value string, w *Warnings) (typ, val string, ok bool) {
	v := strings.ToUpper(strings.TrimSpace(value))
	name := "PROTOCOL," + v
	if !c.Has("rule.protocol") {
		w.Add("rule", name, "PROTOCOL rules %s; skipped", c.Req("rule.protocol"))
		return "", "", false
	}
	out, found := protocolMap[c.Target.Client][v]
	if !found {
		w.Add("rule", name, "protocol %s cannot be matched by %s; skipped", v, c.Matrix.Clients[c.Target.Client].Label)
		return "", "", false
	}
	typ, val, _ = strings.Cut(out, ",")
	return typ, val, true
}
