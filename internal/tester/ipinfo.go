package tester

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

// IPSource describes an HTTP API that reports information about an IP.
//
// Every source is called through the node. If URL contains "{ip}", it is filled
// with the exit IP found by an earlier source (for APIs that look up a given IP);
// otherwise the API reports on the address it sees, the node's exit IP. Going
// through the node keeps the tests independent of this machine's DNS and proxy
// setup, and the lookup sites never see this machine's own IP.
//
// Fields are dot paths into the JSON response ("location.country_code",
// "data.0.score"); a "{ip}" segment stands for the exit IP ("{ip}.risk").
//
// Score is read as a number (a leading number in a string also works, e.g.
// "0.0039 (Very Low)") and multiplied by ScoreScale to land on 0–100. Most APIs
// report risk (higher = worse); it is turned into a safety score (100 - risk)
// unless HigherIsSafer says the API already reports safety. Flags are paths whose
// truthy values become labels, e.g. {"datacenter": "is_datacenter"}.
type IPSource struct {
	Name          string            `json:"name"`
	Enabled       bool              `json:"enabled"`
	URL           string            `json:"url"`
	Headers       map[string]string `json:"headers,omitempty"`
	IP            string            `json:"ip,omitempty"`      // path of the IP
	Country       string            `json:"country,omitempty"` // path of the ISO country code
	City          string            `json:"city,omitempty"`
	ISP           string            `json:"isp,omitempty"`
	ASN           string            `json:"asn,omitempty"`
	Score         string            `json:"score,omitempty"` // path of the score
	ScoreScale    float64           `json:"scoreScale,omitempty"`
	HigherIsSafer bool              `json:"higherIsSafer,omitempty"`

	// Read from settings saved before scores became safety scores.
	LegacyRisk      string            `json:"risk,omitempty"`
	LegacyRiskScale float64           `json:"riskScale,omitempty"`
	Flags           map[string]string `json:"flags,omitempty"` // label -> path
}

// DefaultIPSources are free endpoints that need no key. Adjust or add your own in settings.
func DefaultIPSources() []IPSource {
	return []IPSource{
		{
			Name: "ip-api.com", Enabled: true,
			URL: "http://ip-api.com/json/?fields=status,message,countryCode,city,isp,as,query,proxy,hosting,mobile",
			IP:  "query", Country: "countryCode", City: "city", ISP: "isp", ASN: "as",
			Flags: map[string]string{"代理": "proxy", "机房": "hosting", "移动网络": "mobile"},
		},
		proxycheckV3(),
		{
			// IPPure reports on the caller, so it is queried through the node. fraudScore is a risk score.
			Name: "IPPure", Enabled: true,
			URL: "https://my.ippure.com/v1/info",
			IP:  "ip", Country: "countryCode", City: "city", ISP: "asOrganization", ASN: "asn",
			Score: "fraudScore",
			Flags: map[string]string{"住宅": "isResidential", "广播": "isBroadcast"},
		},
		{
			// Net.Coffee (ip.net.coffee) has no public API; this is the endpoint its page
			// calls to show the IP's properties. trust_score is already "higher is safer".
			Name: "Net.Coffee", Enabled: true,
			URL:     "https://ip.net.coffee/api/iprisk/{ip}",
			Country: "countryCode", City: "city", ISP: "company_name", ASN: "asn",
			Score: "trust_score", HigherIsSafer: true,
			Flags: map[string]string{
				"住宅": "isResidential", "机房": "is_datacenter", "VPN": "is_vpn", "代理": "is_proxy",
				"Tor": "is_tor", "滥用": "is_abuser", "爬虫": "is_crawler", "移动网络": "is_mobile",
			},
		},
	}
}

// IPReport is what one source said about the exit IP.
type IPReport struct {
	Source  string   `json:"source"`
	IP      string   `json:"ip,omitempty"`
	Country string   `json:"country,omitempty"`
	City    string   `json:"city,omitempty"`
	ISP     string   `json:"isp,omitempty"`
	ASN     string   `json:"asn,omitempty"`
	Score   *float64 `json:"score,omitempty"` // safety, 0–100, higher is safer
	Flags   []string `json:"flags,omitempty"`
	Error   string   `json:"error,omitempty"`

	LegacyRisk *float64 `json:"risk,omitempty"` // results stored before safety scores
}

func (s IPSource) Query(ctx context.Context, via *http.Client, ip string, timeout time.Duration) IPReport {
	r := IPReport{Source: s.Name}
	url := s.URL
	client := via
	if client == nil { // unit tests: query directly
		client = &http.Client{Timeout: timeout}
	}
	if strings.Contains(url, "{ip}") {
		if ip == "" {
			r.Error = "未获取到出口 IP（经节点查询的来源都失败了）"
			return r
		}
		url = strings.ReplaceAll(url, "{ip}", ip)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		r.Error = err.Error()
		return r
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0 Safari/537.36")
	req.Header.Set("Accept", "application/json")
	for k, v := range s.Headers {
		req.Header.Set(k, v)
	}
	resp, err := client.Do(req)
	if err != nil {
		r.Error = shortErr(err)
		return r
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		r.Error = fmt.Sprintf("HTTP %d", resp.StatusCode)
		var doc any
		if json.Unmarshal(body, &doc) == nil {
			if m, ok := doc.(map[string]any); ok && str(m["message"]) != "" {
				r.Error += "：" + str(m["message"])
			}
		}
		return r
	}
	var doc any
	if err := json.Unmarshal(body, &doc); err != nil {
		r.Error = "invalid JSON"
		return r
	}
	if msg := apiError(doc); msg != "" {
		r.Error = msg
		return r
	}
	get := func(path string) any { return lookup(doc, path, ip) }
	r.IP = or(str(get(s.IP)), ip)
	r.Country = strings.ToUpper(str(get(s.Country)))
	r.City = str(get(s.City))
	r.ISP = str(get(s.ISP))
	r.ASN = str(get(s.ASN))
	if s.Score != "" {
		if f, ok := number(get(s.Score)); ok {
			if s.ScoreScale != 0 {
				f *= s.ScoreScale
			}
			if !s.HigherIsSafer {
				f = 100 - f
			}
			f = max(0, min(100, float64(int(f*10+0.5))/10))
			r.Score = &f
		}
	}
	for label, path := range s.Flags {
		if truthy(get(path)) {
			r.Flags = append(r.Flags, label)
		}
	}
	slices.Sort(r.Flags)
	return r
}

// lookup walks a dot path ("a.b.0.c") through decoded JSON. A "{ip}" segment is
// replaced by ip (which itself contains dots, so it cannot be written literally).
func lookup(doc any, path, ip string) any {
	if path == "" {
		return nil
	}
	cur := doc
	for _, p := range strings.Split(path, ".") {
		if p == "{ip}" {
			p = ip
		}
		switch v := cur.(type) {
		case map[string]any:
			cur = v[p]
		case []any:
			i, err := strconv.Atoi(p)
			if err != nil || i < 0 || i >= len(v) {
				return nil
			}
			cur = v[i]
		default:
			return nil
		}
	}
	return cur
}

func str(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	default:
		return fmt.Sprint(x)
	}
}

var leadingNumber = regexp.MustCompile(`^\s*(-?\d+(\.\d+)?)`)

func number(v any) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, true
	case string:
		if m := leadingNumber.FindStringSubmatch(x); m != nil {
			f, err := strconv.ParseFloat(m[1], 64)
			return f, err == nil
		}
	}
	return 0, false
}

func truthy(v any) bool {
	switch x := v.(type) {
	case bool:
		return x
	case float64:
		return x != 0
	case string:
		s := strings.ToLower(x)
		return s == "true" || s == "yes" || s == "1"
	}
	return false
}

// shortErr trims wrapped network errors to their last, most specific part.
func shortErr(err error) string {
	s := err.Error()
	if strings.Contains(s, "deadline exceeded") || strings.Contains(s, "Client.Timeout") {
		return "请求超时"
	}
	if i := strings.LastIndex(s, ": "); i >= 0 && len(s)-i < 80 {
		return s[i+2:]
	}
	if len(s) > 120 {
		return s[:120] + "…"
	}
	return s
}

// proxycheckV3 uses proxycheck.io's v3 API (the risk model its website shows).
// Without a key it allows 100 queries per day per server IP; a free key raises that
// to 1,000: append "?key=..." to the URL.
func proxycheckV3() IPSource {
	return IPSource{
		Name: "proxycheck.io", Enabled: true,
		URL:     "https://proxycheck.io/v3/{ip}",
		Country: "{ip}.location.country_code", City: "{ip}.location.city_name",
		ISP: "{ip}.network.provider", ASN: "{ip}.network.asn",
		Score: "{ip}.detections.risk", // risk 0–100; shown as 100 - risk
		Flags: map[string]string{
			"代理": "{ip}.detections.proxy", "VPN": "{ip}.detections.vpn", "机房": "{ip}.detections.hosting",
			"Tor": "{ip}.detections.tor", "匿名": "{ip}.detections.anonymous", "已被入侵": "{ip}.detections.compromised",
		},
	}
}

// oldProxycheckURL is the v2 default shipped earlier; settings still using it are moved to v3.
const oldProxycheckURL = "https://proxycheck.io/v2/{ip}?vpn=1&asn=1&risk=1"

// apiError reports an API-level failure in an HTTP 200 response, e.g.
// {"status":"denied","message":"100 queries exhausted"} or ip-api's {"status":"fail"}.
func apiError(doc any) string {
	m, ok := doc.(map[string]any)
	if !ok {
		return ""
	}
	switch strings.ToLower(str(m["status"])) {
	case "denied", "error", "fail", "failed":
		if msg := str(m["message"]); msg != "" {
			return msg
		}
		return str(m["status"])
	}
	return ""
}
