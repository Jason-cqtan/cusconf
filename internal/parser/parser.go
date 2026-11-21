// Package parser converts subscription content of various formats into Node IR.
//
// Supported inputs:
//   - Share-link (URI) lists, plain or base64 encoded
//   - Clash / mihomo / Stash YAML (proxies:)
//   - Surge / Surge-compatible [Proxy] sections (incl. [WireGuard x] sections)
//   - sing-box JSON (outbounds)
package parser

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"regexp"
	"strings"

	"github.com/cusconf/cusconf/internal/node"
)

type Format string

const (
	FormatURI     Format = "uri"
	FormatClash   Format = "clash"
	FormatSurge   Format = "surge"
	FormatSingBox Format = "sing-box"
)

// Result of parsing a subscription.
type Result struct {
	Format Format
	Nodes  []*node.Node
	// Errors are per-entry problems; parsing continues past them.
	Errors []error
}

var surgeLineRe = regexp.MustCompile(`(?m)^\s*[^=#;\[\s][^=]*=\s*(ss|ssr|vmess|vless|trojan|hysteria2|tuic|tuic-v5|snell|anytls|wireguard|http|https|socks5|socks5-tls|custom)\s*,`)

// Parse detects the format and parses the content.
func Parse(content []byte) Result {
	content = bytes.TrimPrefix(content, []byte("\xef\xbb\xbf"))
	text := strings.TrimSpace(string(content))
	if text == "" {
		return Result{Format: FormatURI}
	}

	var r Result
	switch {
	case strings.HasPrefix(text, "{"):
		r = parseSingBox(text)
	case regexp.MustCompile(`(?m)^proxies\s*:`).MatchString(text):
		r = parseClash(text)
	case surgeLineRe.MatchString(text):
		r = parseSurge(text)
	default:
		if !strings.Contains(text, "://") {
			if dec, ok := decodeBase64(text); ok {
				return Parse([]byte(dec))
			}
		}
		r = parseURIList(text)
	}
	r.Nodes = finalize(r.Nodes, &r.Errors)
	return r
}

// finalize validates nodes and makes names unique.
func finalize(nodes []*node.Node, errs *[]error) []*node.Node {
	out := nodes[:0]
	seen := map[string]int{}
	for _, n := range nodes {
		n.Name = strings.TrimSpace(n.Name)
		if n.Name == "" {
			n.Name = fmt.Sprintf("%s %s:%d", n.Type, n.Server, n.Port)
		}
		if err := n.Validate(); err != nil {
			*errs = append(*errs, err)
			continue
		}
		if c, ok := seen[n.Name]; ok {
			seen[n.Name] = c + 1
			n.Name = fmt.Sprintf("%s %d", n.Name, c+1)
		} else {
			seen[n.Name] = 1
		}
		out = append(out, n)
	}
	return out
}

func parseURIList(text string) Result {
	r := Result{Format: FormatURI}
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "//") {
			continue
		}
		n, err := ParseURI(line)
		if err != nil {
			r.Errors = append(r.Errors, err)
			continue
		}
		r.Nodes = append(r.Nodes, n)
	}
	return r
}

// decodeBase64 tries std / url-safe, padded / raw encodings.
func decodeBase64(s string) (string, bool) {
	s = strings.Map(func(r rune) rune {
		if r == '\n' || r == '\r' || r == ' ' || r == '\t' {
			return -1
		}
		return r
	}, s)
	for _, enc := range []*base64.Encoding{base64.StdEncoding, base64.RawStdEncoding, base64.URLEncoding, base64.RawURLEncoding} {
		if b, err := enc.DecodeString(s); err == nil {
			return string(b), true
		}
	}
	// tolerate wrong padding
	trimmed := strings.TrimRight(s, "=")
	for _, enc := range []*base64.Encoding{base64.RawStdEncoding, base64.RawURLEncoding} {
		if b, err := enc.DecodeString(trimmed); err == nil {
			return string(b), true
		}
	}
	return "", false
}
