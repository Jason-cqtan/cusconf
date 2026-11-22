package tester

import (
	"context"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
)

// Service availability is detected heuristically from public pages/APIs, the way
// tools like RegionRestrictionCheck do. Sites change; when a check starts
// misbehaving, adjust it here.

type ServiceStatus string

const (
	Available   ServiceStatus = "yes"
	Partial     ServiceStatus = "partial" // e.g. Netflix originals only
	Unavailable ServiceStatus = "no"
	Unknown     ServiceStatus = "unknown" // blocked by a captcha / WAF, or an unexpected page
	Failed      ServiceStatus = "error"   // network error
)

type ServiceResult struct {
	Status ServiceStatus `json:"status"`
	Region string        `json:"region,omitempty"`
	Note   string        `json:"note,omitempty"`
}

type Service struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	check func(ctx context.Context, e *env) ServiceResult
}

// env is shared by the checks of one node.
type env struct {
	c    *http.Client
	once sync.Once
	loc  string
}

// exitLoc is the exit country as Cloudflare sees it (cached per node).
func (e *env) exitLoc(ctx context.Context) string {
	e.once.Do(func() { e.loc = traceLoc(ctx, e.c, "https://www.cloudflare.com/cdn-cgi/trace") })
	return e.loc
}

// unsupportedAI are regions where the major AI services (OpenAI, Anthropic, Google Gemini)
// are not offered. Used where a service cannot be probed directly.
var unsupportedAI = map[string]bool{"CN": true, "HK": true, "MO": true, "RU": true, "BY": true, "IR": true, "KP": true, "CU": true, "SY": true}

func byRegion(region string) ServiceResult {
	switch {
	case region == "":
		return ServiceResult{Status: Unknown, Note: "无法确定出口地区"}
	case unsupportedAI[region]:
		return ServiceResult{Status: Unavailable, Region: region, Note: "按出口地区推断"}
	}
	return ServiceResult{Status: Available, Region: region, Note: "按出口地区推断"}
}

// Services lists the built-in checks.
var Services = []Service{
	{ID: "netflix", Name: "Netflix", check: checkNetflix},
	{ID: "disney", Name: "Disney+", check: checkDisney},
	{ID: "youtube", Name: "YouTube Premium", check: checkYouTube},
	{ID: "chatgpt", Name: "ChatGPT", check: checkChatGPT},
	{ID: "claude", Name: "Claude", check: checkClaude},
	{ID: "gemini", Name: "Gemini", check: checkGemini},
	{ID: "tiktok", Name: "TikTok", check: checkTikTok},
	{ID: "spotify", Name: "Spotify", check: checkSpotify},
	{ID: "hbomax", Name: "HBO Max", check: checkHBOMax},
	{ID: "primevideo", Name: "Prime Video", check: checkPrimeVideo},
	{ID: "hulu", Name: "Hulu", check: checkHulu},
	{ID: "steam", Name: "Steam", check: checkSteam},
	{ID: "paypal", Name: "PayPal", check: checkPayPal},
	{ID: "nintendo", Name: "任天堂", check: checkNintendo},
}

func ServiceByID(id string) (Service, bool) {
	for _, s := range Services {
		if s.ID == id {
			return s, true
		}
	}
	return Service{}, false
}

const browserUA = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36"

type page struct {
	status int
	url    string // final URL after redirects
	body   string
}

func fetch(ctx context.Context, c *http.Client, url string, headers ...string) (page, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return page{}, err
	}
	req.Header.Set("User-Agent", browserUA)
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	resp, err := c.Do(req)
	if err != nil {
		return page{}, err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	return page{status: resp.StatusCode, url: resp.Request.URL.String(), body: string(b)}, nil
}

func failed(err error) ServiceResult {
	note := shortErr(err)
	if note == "EOF" || strings.Contains(note, "connection reset") {
		note = "连接被节点断开（可能被节点屏蔽）"
	}
	return ServiceResult{Status: Failed, Note: note}
}

func match(re *regexp.Regexp, s string) string {
	if m := re.FindStringSubmatch(s); m != nil {
		return strings.ToUpper(m[1])
	}
	return ""
}

// traceLoc reads the visitor country from a Cloudflare /cdn-cgi/trace endpoint.
func traceLoc(ctx context.Context, c *http.Client, url string) string {
	p, err := fetch(ctx, c, url)
	if err != nil {
		return ""
	}
	return match(regexp.MustCompile(`(?m)^loc=([A-Z]{2})`), p.body)
}

var (
	netflixPathRe   = regexp.MustCompile(`netflix\.com/([a-z]{2})(?:-[a-z]{2})?/title/`)
	netflixRegionRe = regexp.MustCompile(`"requestCountry":\{"id":"([A-Z]{2})"`)
)

// Netflix: two licensed (non-original) titles. Both 404 means only originals are available.
func checkNetflix(ctx context.Context, e *env) ServiceResult {
	c := e.c
	a, err := fetch(ctx, c, "https://www.netflix.com/title/81280792")
	if err != nil {
		return failed(err)
	}
	b, err := fetch(ctx, c, "https://www.netflix.com/title/70143836")
	if err != nil {
		return failed(err)
	}
	region := match(netflixPathRe, a.url)
	if region == "" {
		region = match(netflixRegionRe, a.body)
	}
	switch {
	case a.status == 403 || b.status == 403:
		return ServiceResult{Status: Unavailable, Note: "访问被拒绝"}
	case a.status == 404 && b.status == 404:
		return ServiceResult{Status: Partial, Region: region, Note: "仅自制剧"}
	case a.status == 200 || b.status == 200:
		if region == "" {
			region = "US"
		}
		return ServiceResult{Status: Available, Region: region}
	}
	return ServiceResult{Status: Unknown, Note: "HTTP " + itoa(a.status)}
}

func checkDisney(ctx context.Context, e *env) ServiceResult {
	c := e.c
	p, err := fetch(ctx, c, "https://www.disneyplus.com/")
	if err != nil {
		return failed(err)
	}
	switch {
	case strings.Contains(p.url, "unavailable"):
		return ServiceResult{Status: Unavailable}
	case p.status == 403:
		return ServiceResult{Status: Unavailable, Note: "访问被拒绝"}
	case p.status == 200:
		return ServiceResult{Status: Available, Region: e.exitLoc(ctx), Note: "仅检测首页可访问"}
	}
	return ServiceResult{Status: Unknown, Note: "HTTP " + itoa(p.status)}
}

func checkYouTube(ctx context.Context, e *env) ServiceResult {
	c := e.c
	p, err := fetch(ctx, c, "https://www.youtube.com/premium", "Cookie", "CONSENT=YES+cb; SOCS=CAI")
	if err != nil {
		return failed(err)
	}
	region := match(regexp.MustCompile(`"INNERTUBE_CONTEXT_GL"\s*:\s*"([A-Z]{2})"`), p.body)
	if region == "" {
		region = match(regexp.MustCompile(`"contentRegion"\s*:\s*"([A-Z]{2})"`), p.body)
	}
	switch {
	case strings.Contains(p.url, "google.cn") || strings.Contains(p.body, "www.google.cn"):
		return ServiceResult{Status: Unavailable, Region: "CN"}
	case strings.Contains(p.body, "Premium is not available in your country"):
		return ServiceResult{Status: Unavailable, Region: region}
	case strings.Contains(p.body, "ad-free") || strings.Contains(p.body, "YouTube Premium"):
		return ServiceResult{Status: Available, Region: region}
	}
	return ServiceResult{Status: Unknown, Region: region, Note: "HTTP " + itoa(p.status)}
}

func checkChatGPT(ctx context.Context, e *env) ServiceResult {
	c := e.c
	p, err := fetch(ctx, c, "https://api.openai.com/compliance/cookie_requirements", "Authorization", "Bearer null")
	if err != nil {
		return failed(err)
	}
	region := traceLoc(ctx, c, "https://chatgpt.com/cdn-cgi/trace")
	if strings.Contains(p.body, "unsupported_country") {
		return ServiceResult{Status: Unavailable, Region: region, Note: "地区不支持"}
	}
	if p.status >= 500 {
		return ServiceResult{Status: Unknown, Region: region, Note: "HTTP " + itoa(p.status)}
	}
	return ServiceResult{Status: Available, Region: region}
}

// Claude's site sits behind a Cloudflare challenge for non-browser clients, so
// availability follows the region Anthropic sees (its Cloudflare trace).
func checkClaude(ctx context.Context, e *env) ServiceResult {
	c := e.c
	p, err := fetch(ctx, c, "https://claude.ai/cdn-cgi/trace")
	if err != nil {
		return failed(err)
	}
	return byRegion(match(regexp.MustCompile(`(?m)^loc=([A-Z]{2})`), p.body))
}

// Gemini's page no longer exposes a reliable availability marker; infer from the exit region.
func checkGemini(ctx context.Context, e *env) ServiceResult {
	return byRegion(e.exitLoc(ctx))
}

func checkTikTok(ctx context.Context, e *env) ServiceResult {
	c := e.c
	p, err := fetch(ctx, c, "https://www.tiktok.com/")
	if err != nil {
		return failed(err)
	}
	if region := match(regexp.MustCompile(`"region"\s*:\s*"([A-Z]{2})"`), p.body); region != "" {
		if region == "CN" {
			return ServiceResult{Status: Unavailable, Region: region}
		}
		return ServiceResult{Status: Available, Region: region}
	}
	if p.status == 403 || strings.Contains(p.url, "notfound") {
		return ServiceResult{Status: Unavailable}
	}
	if p.status == 200 && len(p.body) < 5000 {
		// A short page without region data is usually an anti-bot challenge, not a block.
		return ServiceResult{Status: Unknown, Note: "返回验证页，无法判断"}
	}
	return ServiceResult{Status: Unknown, Note: "HTTP " + itoa(p.status)}
}

func itoa(i int) string { return strconv.Itoa(i) }

func checkSpotify(ctx context.Context, e *env) ServiceResult {
	p, err := fetch(ctx, e.c, "https://www.spotify.com/")
	if err != nil {
		return failed(err)
	}
	switch {
	case strings.Contains(p.url, "select-your-country") || strings.Contains(p.url, "unavailable") || p.status == 403:
		return ServiceResult{Status: Unavailable}
	case p.status < 400:
		return ServiceResult{Status: Available, Region: e.exitLoc(ctx)}
	}
	return ServiceResult{Status: Unknown, Note: "HTTP " + itoa(p.status)}
}

var (
	hboMarketsRe = regexp.MustCompile(`defaultLang\\?":\{([^}]*)\}`)
	hboKeyRe     = regexp.MustCompile(`([a-z]{2})\\?":`)
)

// HBO Max embeds its list of markets in the page (geoData.brand.defaultLang keys);
// availability is whether the exit country is one of them.
func checkHBOMax(ctx context.Context, e *env) ServiceResult {
	p, err := fetch(ctx, e.c, "https://www.hbomax.com/")
	if err != nil {
		return failed(err)
	}
	if p.status == 403 {
		return ServiceResult{Status: Unavailable, Note: "访问被拒绝"}
	}
	m := hboMarketsRe.FindStringSubmatch(p.body)
	if m == nil {
		return ServiceResult{Status: Unknown, Note: "未找到地区列表"}
	}
	region := e.exitLoc(ctx)
	if region == "" {
		return ServiceResult{Status: Unknown, Note: "无法确定出口地区"}
	}
	for _, k := range hboKeyRe.FindAllStringSubmatch(m[1], -1) {
		if strings.EqualFold(k[1], region) {
			return ServiceResult{Status: Available, Region: region}
		}
	}
	return ServiceResult{Status: Unavailable, Region: region, Note: "不在服务地区"}
}

func checkPrimeVideo(ctx context.Context, e *env) ServiceResult {
	p, err := fetch(ctx, e.c, "https://www.primevideo.com/")
	if err != nil {
		return failed(err)
	}
	region := match(regexp.MustCompile(`"currentTerritory":"([A-Z]{2})"`), p.body)
	switch {
	case strings.Contains(p.body, `"isServiceRestricted":true`):
		return ServiceResult{Status: Unavailable, Region: region}
	case region != "":
		return ServiceResult{Status: Available, Region: region}
	}
	return ServiceResult{Status: Unknown, Note: "HTTP " + itoa(p.status)}
}

// Hulu serves the same welcome page everywhere; it only streams in the US.
func checkHulu(ctx context.Context, e *env) ServiceResult {
	region := e.exitLoc(ctx)
	switch region {
	case "":
		return ServiceResult{Status: Unknown, Note: "无法确定出口地区"}
	case "US":
		return ServiceResult{Status: Available, Region: region, Note: "仅限美国，按出口地区推断"}
	}
	return ServiceResult{Status: Unavailable, Region: region, Note: "仅限美国，按出口地区推断"}
}

// Steam works almost everywhere; the store currency shows which regional pricing applies.
func checkSteam(ctx context.Context, e *env) ServiceResult {
	p, err := fetch(ctx, e.c, "https://store.steampowered.com/app/761830/")
	if err != nil {
		return failed(err)
	}
	cur := match(regexp.MustCompile(`"priceCurrency" content="([A-Z]{3})"`), p.body)
	if p.status == 200 && cur != "" {
		return ServiceResult{Status: Available, Region: e.exitLoc(ctx), Note: "商店货币 " + cur}
	}
	if p.status == 403 {
		return ServiceResult{Status: Unavailable, Note: "访问被拒绝"}
	}
	return ServiceResult{Status: Unknown, Note: "HTTP " + itoa(p.status)}
}

func checkPayPal(ctx context.Context, e *env) ServiceResult {
	p, err := fetch(ctx, e.c, "https://www.paypal.com/")
	if err != nil {
		return failed(err)
	}
	region := match(regexp.MustCompile(`paypal\.com/([a-z]{2})/`), p.url)
	if region == "" {
		region = match(regexp.MustCompile(`"country":"([A-Z]{2})"`), p.body)
	}
	switch {
	case p.status == 403:
		return ServiceResult{Status: Unavailable, Note: "访问被拒绝"}
	case p.status < 400:
		return ServiceResult{Status: Available, Region: region}
	}
	return ServiceResult{Status: Unknown, Note: "HTTP " + itoa(p.status)}
}

// Nintendo redirects its site to the visitor's regional store (/us/, /jp/, /hk/ ...).
func checkNintendo(ctx context.Context, e *env) ServiceResult {
	p, err := fetch(ctx, e.c, "https://www.nintendo.com/")
	if err != nil {
		return failed(err)
	}
	region := match(regexp.MustCompile(`nintendo\.com/([a-z]{2})(?:[/-]|$)`), p.url)
	if strings.Contains(p.url, "nintendo.com.cn") || strings.Contains(p.url, "nintendoswitch.com.cn") {
		region = "CN"
	}
	if p.status < 400 {
		return ServiceResult{Status: Available, Region: region, Note: "官网区域"}
	}
	return ServiceResult{Status: Unknown, Note: "HTTP " + itoa(p.status)}
}
