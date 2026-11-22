// Package capability answers "does client X version V support feature F".
package capability

import (
	_ "embed"
	"fmt"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

//go:embed matrix.yaml
var matrixYAML []byte

type Client string

const (
	Surge     Client = "surge"
	Stash     Client = "stash"
	ClashMeta Client = "clash-meta" // mihomo core: Clash Verge Rev, Clash Meta for Android, …
	FlClash   Client = "flclash"    // FlClash app (mihomo core)
	SingBox   Client = "sing-box"
	Loon      Client = "loon"
	QX        Client = "qx" // Quantumult X
)

// Mihomo reports whether the client uses the mihomo (Clash Meta) config dialect.
func (c Client) Mihomo() bool { return c == ClashMeta || c == FlClash }

// Clash reports whether the client takes a Clash-style YAML config.
func (c Client) Clash() bool { return c == Stash || c.Mihomo() }

// Target identifies a concrete client build to generate for.
type Target struct {
	Client   Client `json:"client"`
	Platform string `json:"platform"` // ios | mac | all …
	Version  string `json:"version"`  // empty = latest
}

func (t Target) String() string { return fmt.Sprintf("%s-%s %s", t.Client, t.Platform, t.Version) }

type PlatformInfo struct {
	Label  string `yaml:"label" json:"label"`
	Order  int    `yaml:"order" json:"order"`
	Latest string `yaml:"latest" json:"latest"`
}

type ClientInfo struct {
	Label string `yaml:"label" json:"label"`
	Order int    `yaml:"order" json:"order"`
	Ext   string `yaml:"ext" json:"ext"` // file extension of the config
	// Import is a URL template that opens the client and imports the config; {url} and {name} are escaped.
	Import    string                    `yaml:"import" json:"import,omitempty"`
	Platforms map[string]PlatformInfo   `yaml:"platforms" json:"platforms"`
	Features  map[string]map[string]any `yaml:"features" json:"features"`
}

type Matrix struct {
	Clients map[Client]ClientInfo `yaml:"clients" json:"clients"`
}

var Default = mustLoad()

func mustLoad() *Matrix {
	var mx Matrix
	if err := yaml.Unmarshal(matrixYAML, &mx); err != nil {
		panic(err)
	}
	return &mx
}

// Validate normalizes the target and checks it exists.
func (mx *Matrix) Validate(t *Target) error {
	ci, ok := mx.Clients[t.Client]
	if !ok {
		return fmt.Errorf("unknown client %q", t.Client)
	}
	if t.Platform == "" {
		t.Platform = ci.DefaultPlatform()
	}
	p, ok := ci.Platforms[t.Platform]
	if !ok {
		return fmt.Errorf("unknown platform %q for %s", t.Platform, t.Client)
	}
	if t.Version == "" || t.Version == "latest" {
		t.Version = p.Latest
	}
	if !mx.Supports(*t, "min") {
		return fmt.Errorf("%s %s is too old: %s", p.Label, t.Version, mx.Requirement(*t, "min"))
	}
	return nil
}

// DefaultPlatform is "ios" when the client has it, else its first platform.
func (ci ClientInfo) DefaultPlatform() string {
	if _, ok := ci.Platforms["ios"]; ok {
		return "ios"
	}
	best, order := "", 0
	for k, p := range ci.Platforms {
		if best == "" || p.Order < order || (p.Order == order && k < best) {
			best, order = k, p.Order
		}
	}
	return best
}

// Supports reports whether target supports the feature. Unknown features are
// treated as supported (the matrix lists restrictions, not permissions).
func (mx *Matrix) Supports(t Target, feature string) bool {
	ci, ok := mx.Clients[t.Client]
	if !ok {
		return false
	}
	f, ok := ci.Features[feature]
	if !ok {
		return true
	}
	req, ok := f[t.Platform]
	if !ok {
		return true
	}
	s := fmt.Sprint(req)
	if s == "never" {
		return false
	}
	return CompareVersion(t.Version, s) >= 0
}

// Requirement returns a human readable requirement for messages, e.g. "Surge iOS 6.0.0+".
func (mx *Matrix) Requirement(t Target, feature string) string {
	ci := mx.Clients[t.Client]
	req := fmt.Sprint(ci.Features[feature][t.Platform])
	label := ci.Platforms[t.Platform].Label
	if req == "never" {
		return "not supported by " + label
	}
	return fmt.Sprintf("requires %s %s+", label, req)
}

// CompareVersion compares dotted numeric versions. Missing parts count as 0.
func CompareVersion(a, b string) int {
	pa, pb := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < len(pa) || i < len(pb); i++ {
		var x, y int
		if i < len(pa) {
			x, _ = strconv.Atoi(pa[i])
		}
		if i < len(pb) {
			y, _ = strconv.Atoi(pb[i])
		}
		if x != y {
			if x < y {
				return -1
			}
			return 1
		}
	}
	return 0
}
