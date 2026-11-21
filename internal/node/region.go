package node

import (
	"regexp"
	"strings"
)

// Region detection works on the node name, in this order:
//
//  1. A "region:XX" tag on the node (set explicitly, e.g. by a future IP lookup).
//  2. A flag emoji (any country: the two regional-indicator letters are the ISO code).
//     🇨🇳 is not trusted on its own, since airports often put it on Taiwan nodes.
//  3. The left-most keyword from the table below (Chinese / English names, cities,
//     or a stand-alone uppercase code such as "HK01" or "[US]"). The longer match wins a tie,
//     so "印度尼西亚" is Indonesia, not India.

type regionDef struct {
	Code  string
	Name  string
	words string // case-insensitive alternatives
	codes string // case-sensitive codes that must not touch other letters
	re    *regexp.Regexp
}

// Regions lists keyword-detectable regions. Flag emoji cover every other country.
// Codes are omitted where they collide with common words or units (GB, IT, IS, NO, ID, AT, BE, MO...).
var Regions = []regionDef{
	{Code: "HK", Name: "香港", words: `香港|港|Hong\s?Kong|HKG`, codes: "HK"},
	{Code: "MO", Name: "澳门", words: `澳门|澳門|Macau|Macao`},
	{Code: "TW", Name: "台湾", words: `台湾|台灣|台北|新北|彰化|Taiwan|Taipei|TPE`, codes: "TW"},
	{Code: "JP", Name: "日本", words: `日本|东京|東京|大阪|埼玉|Japan|Tokyo|Osaka`, codes: "JP"},
	{Code: "KR", Name: "韩国", words: `韩国|韓國|首尔|首爾|春川|Korea|Seoul`, codes: "KR"},
	{Code: "SG", Name: "新加坡", words: `新加坡|狮城|獅城|Singapore`, codes: "SG"},
	{Code: "US", Name: "美国", words: `美国|美國|洛杉矶|洛杉磯|圣何塞|聖何塞|西雅图|纽约|紐約|硅谷|芝加哥|达拉斯|凤凰城|United\s?States|America|Los\s?Angeles|San\s?Jose|Seattle|Silicon\s?Valley|Chicago|Dallas|New\s?York`, codes: "US|USA"},
	{Code: "CA", Name: "加拿大", words: `加拿大|多伦多|温哥华|蒙特利尔|Canada|Toronto|Vancouver|Montreal`, codes: "CA"},
	{Code: "GB", Name: "英国", words: `英国|英國|伦敦|倫敦|United\s?Kingdom|Britain|London`, codes: "UK"},
	{Code: "DE", Name: "德国", words: `德国|德國|法兰克福|Germany|Frankfurt`, codes: "DE"},
	{Code: "FR", Name: "法国", words: `法国|法國|巴黎|France|Paris`, codes: "FR"},
	{Code: "NL", Name: "荷兰", words: `荷兰|荷蘭|阿姆斯特丹|Netherlands|Amsterdam`, codes: "NL"},
	{Code: "IT", Name: "意大利", words: `意大利|義大利|米兰|罗马|Italy|Milan|Rome`},
	{Code: "ES", Name: "西班牙", words: `西班牙|马德里|Spain|Madrid`, codes: "ES"},
	{Code: "PT", Name: "葡萄牙", words: `葡萄牙|里斯本|Portugal|Lisbon`, codes: "PT"},
	{Code: "CH", Name: "瑞士", words: `瑞士|苏黎世|Switzerland|Zurich`, codes: "CH"},
	{Code: "AT", Name: "奥地利", words: `奥地利|奧地利|维也纳|Austria|Vienna`},
	{Code: "BE", Name: "比利时", words: `比利时|比利時|布鲁塞尔|Belgium|Brussels`},
	{Code: "IE", Name: "爱尔兰", words: `爱尔兰|愛爾蘭|都柏林|Ireland|Dublin`, codes: "IE"},
	{Code: "SE", Name: "瑞典", words: `瑞典|斯德哥尔摩|Sweden|Stockholm`, codes: "SE"},
	{Code: "NO", Name: "挪威", words: `挪威|奥斯陆|Norway|Oslo`},
	{Code: "FI", Name: "芬兰", words: `芬兰|芬蘭|赫尔辛基|Finland|Helsinki`, codes: "FI"},
	{Code: "DK", Name: "丹麦", words: `丹麦|丹麥|哥本哈根|Denmark|Copenhagen`, codes: "DK"},
	{Code: "IS", Name: "冰岛", words: `冰岛|冰島|Iceland`},
	{Code: "PL", Name: "波兰", words: `波兰|波蘭|华沙|Poland|Warsaw`, codes: "PL"},
	{Code: "CZ", Name: "捷克", words: `捷克|布拉格|Czech|Prague`, codes: "CZ"},
	{Code: "HU", Name: "匈牙利", words: `匈牙利|布达佩斯|Hungary|Budapest`, codes: "HU"},
	{Code: "RO", Name: "罗马尼亚", words: `罗马尼亚|羅馬尼亞|Romania|Bucharest`, codes: "RO"},
	{Code: "BG", Name: "保加利亚", words: `保加利亚|Bulgaria|Sofia`, codes: "BG"},
	{Code: "GR", Name: "希腊", words: `希腊|希臘|雅典|Greece|Athens`, codes: "GR"},
	{Code: "MD", Name: "摩尔多瓦", words: `摩尔多瓦|摩爾多瓦|Moldova|Chisinau`, codes: "MD"},
	{Code: "UA", Name: "乌克兰", words: `乌克兰|烏克蘭|基辅|Ukraine|Kyiv|Kiev`, codes: "UA"},
	{Code: "RU", Name: "俄罗斯", words: `俄罗斯|俄羅斯|莫斯科|伯力|新西伯利亚|Russia|Moscow|Khabarovsk|Novosibirsk`, codes: "RU"},
	{Code: "TR", Name: "土耳其", words: `土耳其|伊斯坦布尔|Turkey|Türkiye|Istanbul`, codes: "TR"},
	{Code: "IL", Name: "以色列", words: `以色列|Israel|Tel\s?Aviv`, codes: "IL"},
	{Code: "AE", Name: "阿联酋", words: `阿联酋|阿聯酋|迪拜|杜拜|UAE|Dubai|Emirates`, codes: "AE"},
	{Code: "SA", Name: "沙特", words: `沙特|Saudi`},
	{Code: "IN", Name: "印度", words: `印度|孟买|孟買|班加罗尔|India|Mumbai|Bangalore`, codes: "IN"},
	{Code: "PK", Name: "巴基斯坦", words: `巴基斯坦|Pakistan`, codes: "PK"},
	{Code: "BD", Name: "孟加拉", words: `孟加拉|Bangladesh`, codes: "BD"},
	{Code: "KZ", Name: "哈萨克斯坦", words: `哈萨克斯坦|哈薩克|Kazakhstan|Almaty`, codes: "KZ"},
	{Code: "MN", Name: "蒙古", words: `蒙古|Mongolia`, codes: "MN"},
	{Code: "TH", Name: "泰国", words: `泰国|泰國|曼谷|Thailand|Bangkok`, codes: "TH"},
	{Code: "VN", Name: "越南", words: `越南|胡志明|河内|Vietnam|Hanoi`, codes: "VN"},
	{Code: "MY", Name: "马来西亚", words: `马来西亚|馬來西亞|大马|吉隆坡|Malaysia|Kuala\s?Lumpur`, codes: "MY"},
	{Code: "ID", Name: "印度尼西亚", words: `印度尼西亚|印尼|雅加达|Indonesia|Jakarta`},
	{Code: "PH", Name: "菲律宾", words: `菲律宾|菲律賓|马尼拉|Philippines|Manila`, codes: "PH"},
	{Code: "KH", Name: "柬埔寨", words: `柬埔寨|金边|Cambodia|Phnom\s?Penh`, codes: "KH"},
	{Code: "AU", Name: "澳大利亚", words: `澳大利亚|澳大利亞|澳洲|悉尼|墨尔本|Australia|Sydney|Melbourne`, codes: "AU"},
	{Code: "NZ", Name: "新西兰", words: `新西兰|紐西蘭|奥克兰|New\s?Zealand|Auckland`, codes: "NZ"},
	{Code: "BR", Name: "巴西", words: `巴西|圣保罗|Brazil|Sao\s?Paulo`, codes: "BR"},
	{Code: "AR", Name: "阿根廷", words: `阿根廷|Argentina|Buenos\s?Aires`, codes: "AR"},
	{Code: "CL", Name: "智利", words: `智利|Chile|Santiago`, codes: "CL"},
	{Code: "MX", Name: "墨西哥", words: `墨西哥|Mexico`, codes: "MX"},
	{Code: "CO", Name: "哥伦比亚", words: `哥伦比亚|Colombia|Bogota`},
	{Code: "PE", Name: "秘鲁", words: `秘鲁|Peru|Lima`},
	{Code: "ZA", Name: "南非", words: `南非|约翰内斯堡|South\s?Africa|Johannesburg`, codes: "ZA"},
	{Code: "NG", Name: "尼日利亚", words: `尼日利亚|Nigeria|Lagos`, codes: "NG"},
	{Code: "EG", Name: "埃及", words: `埃及|开罗|Egypt|Cairo`, codes: "EG"},
	{Code: "CN", Name: "中国", words: `中国|回国|China|大陆|上海|北京|广州|深圳`, codes: "CN"},
}

func init() {
	for i := range Regions {
		r := &Regions[i]
		pat := `(?i:` + r.words + `)`
		if r.codes != "" {
			// The code is captured so its position excludes the boundary character.
			pat += `|(?:^|[^A-Za-z])(` + r.codes + `)(?:[^A-Za-z]|$)`
		}
		r.re = regexp.MustCompile(pat)
	}
}

// flagCode returns the ISO code of the first flag emoji in s ("" if none).
func flagCode(s string) string {
	var prev rune
	for _, r := range s {
		if r >= 0x1F1E6 && r <= 0x1F1FF {
			if prev != 0 {
				return string([]rune{'A' + prev - 0x1F1E6, 'A' + r - 0x1F1E6})
			}
			prev = r
			continue
		}
		prev = 0
	}
	return ""
}

// keywordRegion returns the region whose keyword appears left-most (longest on ties).
func keywordRegion(name string) string {
	best, bestPos, bestLen := "", -1, 0
	for _, r := range Regions {
		m := r.re.FindStringSubmatchIndex(name)
		if m == nil {
			continue
		}
		pos, end := m[0], m[1]
		if len(m) >= 4 && m[2] >= 0 { // matched the code alternative
			pos, end = m[2], m[3]
		}
		if bestPos < 0 || pos < bestPos || (pos == bestPos && end-pos > bestLen) {
			best, bestPos, bestLen = r.Code, pos, end-pos
		}
	}
	return best
}

// infoRe matches pseudo-nodes airports use to display traffic, expiry or notices.
var infoRe = regexp.MustCompile(InfoPattern)

// InfoPattern is the default regex for airport info pseudo-nodes.
const InfoPattern = `(?i)traffic|expire|剩余|到期|过期|官网|流量|重置|套餐|有效期|距离下次|网址|群组|频道|获取[:：]|更新[:：]`

// IsInfo reports whether the node looks like an airport info entry, not a real proxy.
func IsInfo(n *Node) bool { return infoRe.MatchString(n.Name) }

// Region detects an ISO 3166 region code for the node, or "" when unknown.
func Region(n *Node) string {
	for _, t := range n.Tags {
		if c, ok := strings.CutPrefix(t, "region:"); ok {
			return c
		}
	}
	flag := flagCode(n.Name)
	if flag != "" && flag != "CN" {
		return flag
	}
	if kw := keywordRegion(n.Name); kw != "" {
		return kw
	}
	return flag
}
