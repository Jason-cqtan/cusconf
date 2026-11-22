//go:build smoke

package tester

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/cusconf/cusconf/internal/store"
)

// Manual check against real nodes: DB=path/to/cusconf.db NAMES=香港,美国 go test -tags smoke -run Smoke -v ./internal/tester/
func TestSmoke(t *testing.T) {
	st, err := store.Open(os.Getenv("DB"))
	if err != nil {
		t.Fatal(err)
	}
	nodes, _ := st.AllNodes()
	want := strings.Split(os.Getenv("NAMES"), ",")
	for _, w := range want {
		for _, n := range nodes {
			if !strings.Contains(n.Name, w) {
				continue
			}
			r := Test(context.Background(), n, DefaultSettings(), Checks{IP: true, Services: true})
			b, _ := json.MarshalIndent(r, "", "  ")
			t.Logf("%s", b)
			break
		}
	}
}
