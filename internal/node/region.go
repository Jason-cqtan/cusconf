package node

import (
	"regexp"
	"strings"
)

type regionDef struct {
	Code string
	Name string
	re   *regexp.Regexp
}

// Regions lists recognized regions in detection order.
var Regions = []regionDef{
	{"HK", "香港", nil}, {"TW", "台湾", nil}, {"JP", "日本", nil}, {"SG", "新加坡", nil},
	{"US", "美国", nil}, {"KR", "韩国", nil}, {"GB", "英国", nil}, {"DE", "德国", nil},
	{"FR", "法国", nil}, {"NL", "荷兰", nil}, {"CA", "加拿大", nil}, {"AU", "澳大利亚", nil},
	{"RU", "俄罗斯", nil}, {"IN", "印度", nil}, {"TR", "土耳其", nil}, {"AR", "阿根廷", nil},
	{"MY", "马来西亚", nil}, {"TH", "泰国", nil}, {"VN", "越南", nil}, {"PH", "菲律宾", nil},
	{"CN", "中国", nil},
}

var regionPatterns = map[string]string{
	"HK": `🇭🇰|香港|港|\bHK\b|Hong\s?Kong|HKG`,
	"TW": `🇹🇼|台湾|台灣|台北|\bTW\b|Taiwan|TPE`,
	"JP": `🇯🇵|日本|东京|東京|大阪|\bJP\b|Japan|Tokyo|Osaka`,
	"SG": `🇸🇬|新加坡|狮城|\bSG\b|Singapore`,
	"US": `🇺🇸|美国|美國|洛杉矶|圣何塞|西雅图|纽约|\bUS\b|\bUSA\b|United\s?States|America|Los\s?Angeles|San\s?Jose|Seattle`,
	"KR": `🇰🇷|韩国|韓國|首尔|\bKR\b|Korea|Seoul`,
	"GB": `🇬🇧|英国|伦敦|\bUK\b|\bGB\b|United\s?Kingdom|London`,
	"DE": `🇩🇪|德国|法兰克福|\bDE\b|Germany|Frankfurt`,
	"FR": `🇫🇷|法国|巴黎|\bFR\b|France|Paris`,
	"NL": `🇳🇱|荷兰|阿姆斯特丹|\bNL\b|Netherlands|Amsterdam`,
	"CA": `🇨🇦|加拿大|\bCA\b|Canada`,
	"AU": `🇦🇺|澳大利亚|澳洲|悉尼|\bAU\b|Australia|Sydney`,
	"RU": `🇷🇺|俄罗斯|莫斯科|\bRU\b|Russia|Moscow`,
	"IN": `🇮🇳|印度|孟买|\bIN\b|India|Mumbai`,
	"TR": `🇹🇷|土耳其|伊斯坦布尔|\bTR\b|Turkey|Türkiye|Istanbul`,
	"AR": `🇦🇷|阿根廷|\bAR\b|Argentina`,
	"MY": `🇲🇾|马来西亚|吉隆坡|\bMY\b|Malaysia`,
	"TH": `🇹🇭|泰国|曼谷|\bTH\b|Thailand`,
	"VN": `🇻🇳|越南|\bVN\b|Vietnam`,
	"PH": `🇵🇭|菲律宾|\bPH\b|Philippines`,
	"CN": `🇨🇳|中国|回国|\bCN\b|China`,
}

func init() {
	for i := range Regions {
		Regions[i].re = regexp.MustCompile(`(?i)` + regionPatterns[Regions[i].Code])
	}
}

// Region detects a region code from the node name (or a "region:XX" tag).
// Returns "" when unknown.
func Region(n *Node) string {
	for _, t := range n.Tags {
		if c, ok := strings.CutPrefix(t, "region:"); ok {
			return c
		}
	}
	for _, r := range Regions {
		if r.re.MatchString(n.Name) {
			return r.Code
		}
	}
	return ""
}
