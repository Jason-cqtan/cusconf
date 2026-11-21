package node

import "testing"

func TestRegion(t *testing.T) {
	cases := map[string]string{
		"Traffic: 1.48 GB | 400 GB": "",
		"Expire: 2026-12-20":        "",
		"🇭🇰 香港实验性 IEPL 专线 1":        "HK",
		"台湾高级 IEPL 专线 1":            "TW",
		"HK01 | 0.5x":               "HK",
		"[US] Los Angeles":          "US",
		"plus us in my home":        "",
		"🇬🇧 英国 01":                  "GB",
		"UK-London":                 "GB",
		"日本 东京 02":                  "JP",
		"Singapore SG-3":            "SG",
	}
	for name, want := range cases {
		if got := Region(&Node{Name: name}); got != want {
			t.Errorf("%q: got %q want %q", name, got, want)
		}
	}
	if !IsInfo(&Node{Name: "Traffic: 1.48 GB | 400 GB"}) || !IsInfo(&Node{Name: "剩余流量：100G"}) || IsInfo(&Node{Name: "香港 01"}) || !IsInfo(&Node{Name: "获取: 2026-11-21 19:04:01"}) {
		t.Error("IsInfo")
	}
}
