package rewrite

import (
	"github.com/coredns/caddy"
	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"
)

func init() { plugin.Register("rewrite", setup) }

func setup(c *caddy.Controller) error {
	rewrites, err := rewriteParse(c)
	if err != nil {
		return plugin.Error("rewrite", err)
	}

	dnsserver.GetConfig(c).AddPlugin(func(next plugin.Handler) plugin.Handler {
		return Rewrite{Next: next, Rules: rewrites}
	})

	return nil
}

func rewriteParse(c *caddy.Controller) ([]Rule, error) {
	var rules []Rule

	for c.Next() {
		args := c.RemainingArgs()
		if len(args) < 2 {
			// Handles rules out of nested instructions, i.e. the ones enclosed in curly brackets
			for c.NextBlock() {
				args = append(args, c.Val())
			}
		}
		rule, err := newRule(args...)
		if err != nil {
			return nil, err
		}
		rules = append(rules, rule)
	}
	return rules, nil
}
