package engine

import (
	"strings"
	"testing"

	"github.com/cusconf/cusconf/internal/capability"
	"github.com/cusconf/cusconf/internal/profile"
)

func TestProtocolRules(t *testing.T) {
	genProto := func(target capability.Target) (string, func(name string) bool) {
		t.Helper()
		p := fixtureNodes(t)
		p.Rules = append([]profile.Rule{
			{Type: "PROTOCOL", Value: "STUN", Policy: "REJECT"},
			{Type: "PROTOCOL", Value: "UDP", Policy: "DIRECT"},
			{Type: "PROTOCOL", Value: "DOH", Policy: "DIRECT"},
		}, p.Rules...)
		res, err := Generate(Request{Profile: p, Nodes: loadNodes(t), Target: target})
		if err != nil {
			t.Fatal(err)
		}
		return res.Content, func(name string) bool {
			for _, w := range res.Warnings {
				if w.Name == name {
					return true
				}
			}
			return false
		}
	}

	cases := []struct {
		target       capability.Target
		want         []string
		skipped      []string
		notInContent []string
	}{
		{capability.Target{Client: capability.Surge}, []string{"PROTOCOL,STUN,REJECT", "PROTOCOL,UDP,DIRECT", "PROTOCOL,DOH,DIRECT"}, nil, nil},
		{capability.Target{Client: capability.Loon}, []string{"PROTOCOL,STUN,REJECT", "PROTOCOL,UDP,DIRECT"}, []string{"PROTOCOL,DOH"}, nil},
		{capability.Target{Client: capability.Loon, Platform: "ios", Version: "3.1.0"}, nil, []string{"PROTOCOL,STUN", "PROTOCOL,UDP"}, []string{"PROTOCOL,"}},
		{capability.Target{Client: capability.Stash}, []string{"PROTOCOL,STUN,REJECT", "PROTOCOL,UDP,DIRECT"}, []string{"PROTOCOL,DOH"}, nil},
		{capability.Target{Client: capability.ClashMeta}, []string{"NETWORK,udp,DIRECT"}, []string{"PROTOCOL,STUN", "PROTOCOL,DOH"}, []string{"PROTOCOL,"}},
		{capability.Target{Client: capability.SingBox}, []string{`"protocol": [`, `"stun"`, `"network": "udp"`}, []string{"PROTOCOL,DOH"}, nil},
		{capability.Target{Client: capability.QX}, nil, nil, []string{"PROTOCOL", "STUN"}},
	}
	for _, c := range cases {
		content, warned := genProto(c.target)
		for _, s := range c.want {
			if !strings.Contains(content, s) {
				t.Errorf("%s: missing %q", c.target, s)
			}
		}
		for _, n := range c.skipped {
			if !warned(n) {
				t.Errorf("%s: no warning for skipped %s", c.target, n)
			}
		}
		for _, s := range c.notInContent {
			if strings.Contains(content, s) {
				t.Errorf("%s: unexpected %q in output", c.target, s)
			}
		}
	}
}
