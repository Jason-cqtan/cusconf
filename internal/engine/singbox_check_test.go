package engine

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/cusconf/cusconf/internal/capability"
)

// TestSingBoxCheck runs `sing-box check` on generated configs when SINGBOX_BIN
// points to a sing-box binary (built with tags with_quic,with_wireguard,with_utls,with_clash_api).
func TestSingBoxCheck(t *testing.T) {
	bin := os.Getenv("SINGBOX_BIN")
	if bin == "" {
		t.Skip("SINGBOX_BIN not set")
	}
	nodes := loadNodes(t)
	for _, n := range nodes {
		if n.TLS.Reality() {
			n.TLS.RealityPubKey = "SNGj1y3PTH4bvnfTxOtd8tsn7ppf6IU7XXdHhtrrTGs" // well-formed 32-byte key
		}
	}
	for _, v := range []string{"1.12.0", "1.11.0"} {
		res, err := Generate(Request{Profile: fixtureNodes(t), Nodes: nodes, Target: capability.Target{Client: capability.SingBox, Version: v}, Mirror: fakeMirror})
		if err != nil {
			t.Fatal(err)
		}
		f := filepath.Join(t.TempDir(), "config.json")
		if err := os.WriteFile(f, []byte(res.Content), 0o644); err != nil {
			t.Fatal(err)
		}
		if out, err := exec.Command(bin, "check", "-c", f).CombinedOutput(); err != nil {
			t.Errorf("sing-box %s check: %v\n%s", v, err, out)
		}
	}
}
