package render

import (
	"strings"

	"github.com/cusconf/cusconf/internal/profile"
)

// Cond is one condition of a rule as a client writes it: TYPE,VALUE[,no-resolve].
type Cond struct {
	Type, Value string
	NoResolve   bool
}

// String is the condition inside a logical rule, e.g. "IP-CIDR,10.0.0.0/8,no-resolve".
func (c Cond) String() string {
	s := c.Type + "," + c.Value
	if c.NoResolve {
		s += ",no-resolve"
	}
	return s
}

// LogicalRule writes an AND / OR / NOT rule without its policy, in the syntax Surge,
// Loon, Stash and mihomo share: AND,((TYPE,VALUE),(OR,((TYPE,VALUE),(TYPE,VALUE)))).
// cond converts one plain condition for the client, or returns ok=false after adding
// its own warning. A rule with any condition the client cannot express is skipped as a
// whole (dropping a condition would change what it matches), with a warning.
func LogicalRule(c *Config, r profile.Rule, w *Warnings, cond func(profile.Rule) (Cond, bool)) (string, bool) {
	name := r.Expr()
	if !c.Has("rule.logical") {
		w.Add("rule", name, "logical rules %s; skipped", c.Req("rule.logical"))
		return "", false
	}
	expr, ok := logicalExpr(r, cond)
	if !ok {
		w.Add("rule", name, "a condition cannot be written for %s; logical rule skipped", c.Matrix.Clients[c.Target.Client].Label)
		return "", false
	}
	return expr, true
}

func logicalExpr(r profile.Rule, cond func(profile.Rule) (Cond, bool)) (string, bool) {
	parts := make([]string, 0, len(r.Rules))
	for _, s := range r.Rules {
		var inner string
		if profile.IsLogical(s.Type) {
			e, ok := logicalExpr(s, cond)
			if !ok {
				return "", false
			}
			inner = e
		} else {
			cd, ok := cond(s)
			if !ok {
				return "", false
			}
			inner = cd.String()
		}
		parts = append(parts, "("+inner+")")
	}
	if len(parts) == 0 {
		return "", false
	}
	return strings.ToUpper(r.Type) + ",(" + strings.Join(parts, ",") + ")", true
}
