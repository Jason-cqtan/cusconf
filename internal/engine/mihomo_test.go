package engine

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/cusconf/cusconf/internal/capability"
	"github.com/cusconf/cusconf/internal/tester"
)

// TestMihomoParses checks the generated Clash Meta config with `mihomo -t`.
// Needs the mihomo executable (CUSCONF_MIHOMO or PATH); skipped without it.
func TestMihomoParses(t *testing.T) {
	bin, err := tester.MihomoPath()
	if err != nil {
		t.Skip(err)
	}
	nodes := loadNodes(t)
	for _, n := range nodes {
		if n.TLS.Reality() {
			n.TLS.RealityPubKey = "SNGj1y3PTH4bvnfTxOtd8tsn7ppf6IU7XXdHhtrrTGs" // well-formed 32-byte key
		}
	}
	p := fixtureNodes(t)
	rules := p.Rules[:0]
	for _, r := range p.Rules {
		if r.Type != "GEOIP" { // would need the MMDB download
			rules = append(rules, r)
		}
	}
	p.Rules = append(logicalRules(), rules...) // AND / OR / NOT, with a process regex and no-resolve inside
	res, err := Generate(Request{Profile: p, Nodes: nodes, Target: capability.Target{Client: capability.ClashMeta}, Mirror: fakeMirror})
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(cfg, []byte(res.Content), 0o600); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command(bin, "-d", dir, "-f", cfg, "-t").CombinedOutput(); err != nil {
		t.Fatalf("mihomo rejected the config: %v\n%s", err, out)
	}
}
