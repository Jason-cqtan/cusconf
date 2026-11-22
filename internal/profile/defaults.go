package profile

import (
	"slices"

	"github.com/cusconf/cusconf/internal/node"
)

// IconBase serves the Koolson/Qure color icon set, widely used for policy groups.
const IconBase = "https://raw.githubusercontent.com/Koolson/Qure/master/IconSet/Color/"

// ACL4SSR returns a rule set from ACL4SSR/ACL4SSR. Its classical .list files work for
// both Surge (RULE-SET) and Stash (rule-provider, format text). path is under Clash/.
func ACL4SSR(name, path string) RuleSet {
	url := "https://raw.githubusercontent.com/ACL4SSR/ACL4SSR/master/Clash/" + path
	return RuleSet{Name: name, Behavior: "classical", SurgeURL: url, StashURL: url, StashFormat: "text", Interval: 86400}
}

// mainRegions lead the region list, in this order.
var mainRegions = []struct{ code, icon string }{
	{"HK", "Hong_Kong.png"}, {"TW", "Taiwan.png"}, {"JP", "Japan.png"}, {"SG", "Singapore.png"}, {"US", "United_States.png"},
}

// OtherGroup collects airport nodes outside the main regions (and nodes without a known region).
const OtherGroup = "其他国家"

// RegionGroups builds one hidden url-test group per main region (HK, TW, JP, SG, US)
// that has nodes, plus a hidden "其他国家" select group for everything else when
// there is anything else. "其他国家" excludes the main regions rather than listing
// the other countries, so countries that appear later still land somewhere.
//
// counts maps region code to airport node count; with no nodes yet, every group is created.
func RegionGroups(counts map[string]int) []Group {
	all := len(counts) == 0
	var codes []string
	for _, r := range mainRegions {
		codes = append(codes, r.code)
	}
	var out []Group
	for _, r := range mainRegions {
		if all || counts[r.code] > 0 {
			out = append(out, Group{Name: node.RegionName(r.code), Type: URLTest, Icon: IconBase + r.icon, Hidden: true,
				Nodes: &Filter{Regions: []string{r.code}}, Interval: 600, Tolerance: 50})
		}
	}
	other := all
	for c, n := range counts {
		if n > 0 && !slices.Contains(codes, c) {
			other = true
		}
	}
	if other {
		out = append(out, Group{Name: OtherGroup, Type: Select, Icon: IconBase + "Global.png", Hidden: true,
			Nodes: &Filter{ExcludeRegions: codes}})
	}
	return out
}

// SelfHostedGroup holds the nodes of self-hosted sources.
const SelfHostedGroup = "自建"

// Names of the preset groups.
const (
	GlobalGroup = "全局代理"
	FinalGroup  = "漏网之鱼"
)

// Default returns a starter profile with common groups and rules.
// counts (region code -> airport node count) shapes the per-country groups;
// selfHosted is the number of self-hosted nodes (a "自建" group is added when > 0).
//
// Group order: the preset groups (全局代理, 漏网之鱼), then 自建, then the regions.
func Default(counts map[string]int, selfHosted int) Profile {
	p := Profile{Name: "默认配置", Filter: Filter{Exclude: node.InfoPattern}, General: DefaultGeneral()}
	regions := RegionGroups(counts)

	// 全局代理 tests the main regions' groups.
	global := Group{Name: GlobalGroup, Type: URLTest, Icon: IconBase + "Proxy.png", Interval: 600, Tolerance: 50}
	for _, g := range regions {
		if g.Name != OtherGroup {
			global.Members = append(global.Members, g.Name)
		}
	}
	p.Groups = append(p.Groups,
		global,
		Group{Name: FinalGroup, Type: Fallback, Icon: IconBase + "Final.png", Members: []string{GlobalGroup, "DIRECT"}, Interval: 600},
	)
	if selfHosted > 0 {
		p.Groups = append(p.Groups, Group{Name: SelfHostedGroup, Type: Select, Icon: IconBase + "Static.png",
			Nodes: &Filter{NodeKind: "selfhosted"}})
	}
	p.Groups = append(p.Groups, regions...)

	// Rule sets follow the ACL4SSR_Online layout: LAN direct, the GFW proxy list, then China
	// domains / IPs direct. Like the groups, service rule sets come with the group templates.
	p.RuleSets = []RuleSet{
		ACL4SSR("LocalAreaNetwork", "LocalAreaNetwork.list"),
		ACL4SSR("ProxyGFWlist", "ProxyGFWlist.list"),
		ACL4SSR("ChinaDomain", "ChinaDomain.list"),
		ACL4SSR("ChinaCompanyIp", "ChinaCompanyIp.list"),
	}
	p.Rules = []Rule{
		{Type: "RULE-SET", Value: "LocalAreaNetwork", Policy: "DIRECT"},
		{Type: "DOMAIN-WILDCARD", Value: "*pcdn*.biliapi.net", Policy: "REJECT", Comment: "解决观看B 站卡顿"},
		{Type: "DOMAIN-SUFFIX", Value: "mcdn.bilivideo.cn", Policy: "REJECT"},
		{Type: "DOMAIN-SUFFIX", Value: "mcdn.bilivideo.com", Policy: "REJECT"},
		{Type: "DOMAIN-SUFFIX", Value: "szbdyd.com", Policy: "REJECT"},
		{Type: "DOMAIN-SUFFIX", Value: "mountaintoys.cn", Policy: "REJECT"},
		{Type: "RULE-SET", Value: "ProxyGFWlist", Policy: GlobalGroup},
		{Type: "RULE-SET", Value: "ChinaDomain", Policy: "DIRECT"},
		{Type: "RULE-SET", Value: "ChinaCompanyIp", Policy: "DIRECT", NoResolve: true},
		{Type: "GEOIP", Value: "CN", Policy: "DIRECT"},
		{Type: "FINAL", Policy: FinalGroup},
	}
	return p
}

// AliDoH is the domestic encrypted DNS used for China domains and node domains.
const AliDoH = "https://dns.alidns.com/dns-query"

// DefaultGeneral is the recommended [General] / [Host] setup against DNS hijacking and leaks:
//   - every DNS query on port 53 is taken over by Surge (enhanced mode);
//   - other domains use overseas DoH that follows the rules / proxy, so the ISP sees only
//     encrypted traffic and DNS leak tests show Cloudflare / Google;
//   - China domains use AliDNS DoH (right CDN answers), node and subscription domains too
//     (ServerDNS), so resolving a node never needs a node;
//   - DoH host names are pinned, so no plain DNS query is needed to reach them.
func DefaultGeneral() General {
	return General{
		DNS: []string{"223.5.5.5", "119.29.29.29"},
		SkipProxy: []string{
			"127.0.0.1", "192.168.0.0/16", "10.0.0.0/8", "172.16.0.0/12", "100.64.0.0/10", "17.0.0.0/8",
			"localhost", "*.local", "captive.apple.com", "*.crashlytics.com", "seed-sequoia.siri.apple.com", "sequoia.apple.com",
		},
		ServerDNS: AliDoH,
		Hosts: []Host{
			{Domain: "dns.alidns.com", Value: "223.5.5.5, 223.6.6.6", Comment: "加密 DNS 服务器固定地址，连接它们无需先做明文 DNS 查询"},
			{Domain: "cloudflare-dns.com", Value: "1.1.1.1, 1.0.0.1"},
			{Domain: "dns.google", Value: "8.8.8.8, 8.8.4.4"},
			{Domain: "*.cn", Value: "server:" + AliDoH, Comment: "国内域名用阿里加密 DNS，得到国内 CDN 的就近地址；与分流共用 ChinaDomain 列表"},
			{Domain: "RULE-SET:https://raw.githubusercontent.com/ACL4SSR/ACL4SSR/master/Clash/ChinaDomain.list", Value: "server:" + AliDoH},
		},
		Extra: map[string]string{"surge": DefaultSurgeExtra},
	}
}

// DefaultSurgeExtra is appended to Surge's [General]; edit it in the profile's General tab.
const DefaultSurgeExtra = `# ---- DNS 防劫持 ----
# 劫持所有 53 端口的 DNS 查询交给 Surge 处理（需开启增强模式）
hijack-dns = *:53
# 境外加密 DNS：运营商只能看到加密流量，DNS 检测站看到的是 Cloudflare / Google
encrypted-dns-server = https://cloudflare-dns.com/dns-query, https://dns.google/dns-query
# 加密 DNS 查询按规则 / 代理出站；国内域名与节点域名见 [Host] 中的阿里 DNS 映射
encrypted-dns-follow-outbound-mode = true
# 注：配置加密 DNS 后，上方 dns-server 只用于连通性检测和引导，不会在加密 DNS 失败时接管查询
# ---- 其它 ----
# 节点不支持 UDP 时直接拒绝，避免 UDP（如 QUIC）泄漏到直连
udp-policy-not-supported-behaviour = REJECT
# Fake IP 模式下需要真实 IP 的域名（游戏主机联机、系统连通性检测）
always-real-ip = *.srv.nintendo.net, *.stun.playstation.net, xbox.*.microsoft.com, *.xboxlive.com, *.msftncsi.com, *.msftconnecttest.com
`
