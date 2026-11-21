package profile

import (
	"fmt"

	"github.com/cusconf/cusconf/internal/node"
)

// IconBase serves the Koolson/Qure color icon set, widely used for policy groups.
const IconBase = "https://raw.githubusercontent.com/Koolson/Qure/master/IconSet/Color/"

// BM7 returns a rule set backed by blackmatrix7/ios_rule_script, which publishes
// the same lists for Surge (.list) and Clash/Stash (.yaml, classical).
func BM7(name string) RuleSet {
	base := "https://raw.githubusercontent.com/blackmatrix7/ios_rule_script/master/rule/"
	return RuleSet{
		Name:        name,
		Behavior:    "classical",
		SurgeURL:    fmt.Sprintf("%sSurge/%s/%s.list", base, name, name),
		StashURL:    fmt.Sprintf("%sClash/%s/%s.yaml", base, name, name),
		StashFormat: "yaml",
		Interval:    86400,
	}
}

type regionPreset struct{ code, name, icon string }

var regionPresets = []regionPreset{
	{"HK", "香港", "Hong_Kong.png"},
	{"TW", "台湾", "Taiwan.png"},
	{"JP", "日本", "Japan.png"},
	{"SG", "新加坡", "Singapore.png"},
	{"US", "美国", "United_States.png"},
}

// Default returns a starter profile with common groups and rules.
func Default() Profile {
	p := Profile{Name: "默认配置", Filter: Filter{Exclude: node.InfoPattern}}
	regionNames := make([]string, 0, len(regionPresets))
	for _, r := range regionPresets {
		regionNames = append(regionNames, r.name)
	}
	main := Group{Name: "节点选择", Type: Select, Icon: IconBase + "Proxy.png",
		Members: append([]string{"自动选择"}, regionNames...), Nodes: &Filter{}}
	auto := Group{Name: "自动选择", Type: Smart, Icon: IconBase + "Auto.png", Nodes: &Filter{}, Interval: 600, Tolerance: 50}
	p.Groups = append(p.Groups, main, auto)

	// Only the final group offers DIRECT; the others always go through a proxy.
	app := func(name, icon string) Group {
		return Group{Name: name, Type: Select, Icon: IconBase + icon,
			Members: append([]string{"节点选择", "自动选择"}, regionNames...)}
	}
	p.Groups = append(p.Groups,
		app("OpenAI", "ChatGPT.png"),
		app("Netflix", "Netflix.png"),
		app("YouTube", "YouTube.png"),
		app("Telegram", "Telegram.png"),
		app("Apple", "Apple.png"),
		app("Microsoft", "Microsoft.png"),
		Group{Name: "漏网之鱼", Type: Select, Icon: IconBase + "Final.png", Members: []string{"节点选择", "DIRECT"}},
	)
	for _, r := range regionPresets {
		p.Groups = append(p.Groups, Group{Name: r.name, Type: URLTest, Icon: IconBase + r.icon,
			Nodes: &Filter{Regions: []string{r.code}}, Interval: 600, Tolerance: 50})
	}

	p.RuleSets = []RuleSet{BM7("OpenAI"), BM7("Netflix"), BM7("YouTube"), BM7("Telegram"), BM7("Apple"), BM7("Microsoft"), BM7("Global"), BM7("ChinaMax")}
	p.Rules = []Rule{
		{Type: "RULE-SET", Value: "OpenAI", Policy: "OpenAI"},
		{Type: "RULE-SET", Value: "Netflix", Policy: "Netflix"},
		{Type: "RULE-SET", Value: "YouTube", Policy: "YouTube"},
		{Type: "RULE-SET", Value: "Telegram", Policy: "Telegram"},
		{Type: "RULE-SET", Value: "Apple", Policy: "Apple"},
		{Type: "RULE-SET", Value: "Microsoft", Policy: "Microsoft"},
		{Type: "RULE-SET", Value: "Global", Policy: "节点选择"},
		{Type: "RULE-SET", Value: "ChinaMax", Policy: "DIRECT"},
		{Type: "GEOIP", Value: "CN", Policy: "DIRECT"},
		{Type: "FINAL", Policy: "漏网之鱼"},
	}
	return p
}
