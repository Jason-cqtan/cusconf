package tester

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/cusconf/cusconf/internal/node"
	"github.com/cusconf/cusconf/internal/parser"
)

func TestLookup(t *testing.T) {
	var doc any
	json.Unmarshal([]byte(`{"status":"ok","1.2.3.4":{"risk":66,"proxy":"yes"},"a":{"b":[{"c":"x"}]},"s":"0.0039 (Very Low)"}`), &doc)
	if v, _ := number(lookup(doc, "{ip}.risk", "1.2.3.4")); v != 66 {
		t.Errorf("ip segment: %v", v)
	}
	if !truthy(lookup(doc, "{ip}.proxy", "1.2.3.4")) {
		t.Error("truthy yes")
	}
	if str(lookup(doc, "a.b.0.c", "")) != "x" {
		t.Error("array path")
	}
	if v, ok := number(lookup(doc, "s", "")); !ok || v != 0.0039 {
		t.Errorf("leading number: %v", v)
	}
}

func TestIPSourceQuery(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"ip":"9.9.9.9","geo":{"cc":"de"},"score":0.42,"flags":{"vpn":true,"tor":false}}`))
	}))
	defer srv.Close()
	src := IPSource{Name: "t", URL: srv.URL + "/{ip}", IP: "ip", Country: "geo.cc", Score: "score", ScoreScale: 100,
		Flags: map[string]string{"VPN": "flags.vpn", "Tor": "flags.tor"}}
	r := src.Query(context.Background(), nil, "9.9.9.9", 5e9)
	if r.Error != "" || r.Country != "DE" || r.Score == nil || *r.Score != 58 || !slices.Equal(r.Flags, []string{"VPN"}) {
		t.Errorf("%+v score=%v", r, r.Score)
	}
	src.HigherIsSafer = true
	if r := src.Query(context.Background(), nil, "9.9.9.9", 5e9); *r.Score != 42 {
		t.Errorf("higher-is-safer score: %v", *r.Score)
	}
	if r := src.Query(context.Background(), nil, "", 5e9); r.Error == "" {
		t.Error("expected error without exit IP")
	}
}

// Every protocol the parser understands must be accepted by mihomo.
// Needs the mihomo executable (CUSCONF_MIHOMO or PATH); skipped without it.
func TestToMihomoParses(t *testing.T) {
	if _, err := MihomoPath(); err != nil {
		t.Skip(err)
	}
	var nodes []*node.Node
	for _, f := range []string{"../parser/testdata/uri.txt", "../parser/testdata/clash.yaml", "../parser/testdata/surge.conf"} {
		b, _ := readFile(f)
		for _, n := range parser.Parse(b).Nodes {
			if (n.Type == node.WireGuard && n.WireGuard.PrivateKey == "priv") || (n.TLS.Reality() && len(n.TLS.RealityPubKey) < 40) {
				continue // fixture keys are placeholders, not real keys
			}
			nodes = append(nodes, n)
		}
	}
	core, err := StartCore(context.Background(), nodes)
	if err != nil {
		t.Fatal(err)
	}
	defer core.Close()
	for _, n := range nodes {
		if _, err := core.Proxy(n); err != nil {
			t.Errorf("%s (%s): %v", n.Name, n.Type, err)
		}
	}
}

func TestProbe(t *testing.T) {
	r := Result{Latency: 80, Exit: &Exit{Country: "US", Scores: map[string]float64{"IPPure": 88}}, Services: map[string]ServiceResult{
		"netflix": {Status: Partial}, "chatgpt": {Status: Available}, "claude": {Status: Available}, "gemini": {Status: Unavailable},
	}}
	p := r.Probe()
	if p.ExitCountry != "US" || p.Scores["IPPure"] != 88 || !slices.Equal(p.Unlocked, []string{"chatgpt", "claude"}) || !slices.Equal(p.Partial, []string{"netflix"}) {
		t.Errorf("%+v", p)
	}
}

func readFile(p string) ([]byte, error) { return os.ReadFile(p) }

func TestTestable(t *testing.T) {
	nodes := []*node.Node{
		{Name: "Traffic: 1 GB | 100 GB", Type: node.SS, Server: "a", Port: 1, Password: "p", Source: "s1"},
		{Name: "HK 1", Type: node.SS, Server: "a", Port: 1, Password: "p", Source: "s1"}, // same endpoint as the info node
		{Name: "JP 1", Type: node.SS, Server: "b", Port: 1, Password: "p", Source: "s2"},
		{Name: "JP 1 copy", Type: node.SS, Server: "b", Port: 1, Password: "p", Source: "s2"},
	}
	names := func(ns []*node.Node) (out []string) {
		for _, n := range ns {
			out = append(out, n.Name)
		}
		return
	}
	if got := names(Testable(nodes, Settings{})); !slices.Equal(got, []string{"HK 1", "JP 1"}) {
		t.Errorf("all sources: %v", got)
	}
	if got := names(Testable(nodes, Settings{Sources: []string{"s2"}})); !slices.Equal(got, []string{"JP 1"}) {
		t.Errorf("s2 only: %v", got)
	}
}

func TestUpgradeLegacyRisk(t *testing.T) {
	var r Result
	json.Unmarshal([]byte(`{"latency":50,"exit":{"country":"SG","risk":66,"score":34},"reports":[{"source":"p","risk":66},{"source":"q"},{"source":"n","score":92}]}`), &r)
	r.upgrade()
	if r.Exit.LegacyRisk != nil || r.Exit.LegacyScore != nil || r.Exit.Scores["p"] != 34 || r.Exit.Scores["n"] != 92 || len(r.Exit.Scores) != 2 || *r.Reports[0].Score != 34 {
		t.Errorf("%+v", r.Exit)
	}
	s := Settings{IPSources: []IPSource{{Name: "x", URL: "u", LegacyRisk: "a.b", LegacyRiskScale: 100}}}
	s.normalize()
	if s.IPSources[0].Score != "a.b" || s.IPSources[0].ScoreScale != 100 || s.IPSources[0].LegacyRisk != "" {
		t.Errorf("%+v", s.IPSources[0])
	}
}

func TestNewServicesEnabledByDefault(t *testing.T) {
	// Settings saved before the new checks existed, with gemini switched off.
	s := Settings{Services: []string{"netflix", "disney", "youtube", "chatgpt", "claude", "tiktok"}}
	s.normalize()
	for _, id := range []string{"spotify", "hbomax", "nintendo"} {
		if !slices.Contains(s.Services, id) {
			t.Errorf("%s should be enabled", id)
		}
	}
	if slices.Contains(s.Services, "gemini") {
		t.Error("gemini was disabled by the user and must stay off")
	}
	// Once known, turning a check off sticks.
	s.Services = slices.DeleteFunc(s.Services, func(id string) bool { return id == "spotify" })
	s.normalize()
	if slices.Contains(s.Services, "spotify") {
		t.Error("spotify should stay off")
	}
}

func TestProxycheckV3AndAPIErrors(t *testing.T) {
	body := `{"status":"ok","108.165.65.119":{"network":{"asn":"AS149440","provider":"Evoxt Sdn. Bhd.","type":"Hosting"},
		"location":{"country_code":"MY","city_name":"Kuala Lumpur"},
		"detections":{"proxy":false,"vpn":false,"hosting":true,"tor":false,"anonymous":false,"compromised":false,"risk":33}}}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "denied") {
			w.Write([]byte(`{"status":"denied","message":"100 queries exhausted"}`))
			return
		}
		w.Write([]byte(body))
	}))
	defer srv.Close()
	src := proxycheckV3()
	src.URL = srv.URL + "/v3/{ip}"
	r := src.Query(context.Background(), nil, "108.165.65.119", 5e9)
	if r.Error != "" || r.Score == nil || *r.Score != 67 || r.Country != "MY" || !slices.Equal(r.Flags, []string{"机房"}) {
		t.Errorf("%+v score=%v", r, r.Score)
	}
	src.URL = srv.URL + "/denied/{ip}"
	if r := src.Query(context.Background(), nil, "1.2.3.4", 5e9); r.Error != "100 queries exhausted" || r.Score != nil {
		t.Errorf("denied: %+v", r)
	}
	// Saved settings with the old v2 default move to v3.
	s := Settings{IPSources: []IPSource{{Name: "proxycheck.io", Enabled: false, URL: oldProxycheckURL, Score: "{ip}.risk"}}}
	s.normalize()
	if got := s.IPSources[0]; got.URL != "https://proxycheck.io/v3/{ip}" || got.Score != "{ip}.detections.risk" || got.Enabled {
		t.Errorf("migrated: %+v", got)
	}
}

func TestNewIPSourcesAddedByDefault(t *testing.T) {
	// Settings saved with only the original two sources, one of them customised.
	s := Settings{IPSources: []IPSource{{Name: "ip-api.com", URL: "http://x"}, {Name: "my own", URL: "http://y"}}}
	s.normalize()
	var names []string
	for _, src := range s.IPSources {
		names = append(names, src.Name)
	}
	if !slices.Equal(names, []string{"ip-api.com", "my own", "IPPure", "Net.Coffee"}) {
		t.Errorf("sources: %v", names) // proxycheck.io was known and removed by the user: stays removed
	}
	s.IPSources = s.IPSources[:3] // user removes Net.Coffee
	s.normalize()
	if len(s.IPSources) != 3 {
		t.Errorf("removed default came back: %d", len(s.IPSources))
	}
}

func TestNetCoffeeTrustScore(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"ip":"179.65.141.47","is_datacenter":false,"isResidential":true,"is_vpn":false,"countryCode":"jp","company_name":"SpaceX","asn":14593,"trust_score":100}`))
	}))
	defer srv.Close()
	var src IPSource
	for _, d := range DefaultIPSources() {
		if d.Name == "Net.Coffee" {
			src = d
		}
	}
	src.URL = srv.URL + "/api/iprisk/{ip}"
	r := src.Query(context.Background(), nil, "179.65.141.72", 5e9)
	if r.Error != "" || *r.Score != 100 || r.Country != "JP" || !slices.Equal(r.Flags, []string{"住宅"}) || r.IP != "179.65.141.72" {
		t.Errorf("%+v", r)
	}
}

func TestBuiltinDirectionFixed(t *testing.T) {
	// A saved Net.Coffee entry without the flag (or IPPure with it) is corrected from the built-in definition.
	s := Settings{IPSources: []IPSource{{Name: "Net.Coffee", URL: "u"}, {Name: "IPPure", URL: "v", HigherIsSafer: true}, {Name: "custom", URL: "w", HigherIsSafer: true}}}
	s.normalize()
	if !s.IPSources[0].HigherIsSafer || s.IPSources[1].HigherIsSafer || !s.IPSources[2].HigherIsSafer {
		t.Errorf("%+v", s.IPSources[:3])
	}
}

// A node mihomo rejects reports its own error; the others still load.
func TestCoreDropsBadNode(t *testing.T) {
	if _, err := MihomoPath(); err != nil {
		t.Skip(err)
	}
	good := &node.Node{Name: "good", Type: node.SS, Server: "1.2.3.4", Port: 8388, Cipher: "aes-128-gcm", Password: "x"}
	bad := &node.Node{Name: "bad", Type: node.VLESS, Server: "1.2.3.4", Port: 443, UUID: "b831b1f8-5a5e-4b57-a03e-1234567890ab",
		TLS: &node.TLS{Enabled: true, SNI: "a.com", RealityPubKey: "not-a-key"}}
	core, err := StartCore(context.Background(), []*node.Node{bad, good})
	if err != nil {
		t.Fatal(err)
	}
	defer core.Close()
	if _, err := core.Proxy(bad); err == nil || !strings.Contains(err.Error(), "REALITY") {
		t.Errorf("bad node: %v", err)
	}
	if _, err := core.Proxy(good); err != nil {
		t.Errorf("good node: %v", err)
	}
}
