package rewrite

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/request"

	"github.com/miekg/dns"
)

type ttlResponseRule struct {
	TTL uint32
}

func (r *ttlResponseRule) RewriteResponse(rr dns.RR) {
	rr.Header().Ttl = r.TTL
}

type ttlRuleBase struct {
	nextAction string
	response   ttlResponseRule
}

func newTTLRuleBase(nextAction string, ttl uint32) ttlRuleBase {
	return ttlRuleBase{
		nextAction: nextAction,
		response:   ttlResponseRule{TTL: ttl},
	}
}

func (rule *ttlRuleBase) responseRule(match bool) (ResponseRules, Result) {
	if match {
		return ResponseRules{&rule.response}, RewriteDone
	}
	return nil, RewriteIgnored
}

// Mode returns the processing nextAction
func (rule *ttlRuleBase) Mode() string { return rule.nextAction }

type exactTTLRule struct {
	ttlRuleBase
	From string
}

type prefixTTLRule struct {
	ttlRuleBase
	Prefix string
}

type suffixTTLRule struct {
	ttlRuleBase
	Suffix string
}

type substringTTLRule struct {
	ttlRuleBase
	Substring string
}

type regexTTLRule struct {
	ttlRuleBase
	Pattern *regexp.Regexp
}

// Rewrite rewrites the current request based upon exact match of the name
// in the question section of the request.
func (rule *exactTTLRule) Rewrite(ctx context.Context, state request.Request) (ResponseRules, Result) {
	return rule.responseRule(rule.From == state.Name())
}

// Rewrite rewrites the current request when the name begins with the matching string.
func (rule *prefixTTLRule) Rewrite(ctx context.Context, state request.Request) (ResponseRules, Result) {
	return rule.responseRule(strings.HasPrefix(state.Name(), rule.Prefix))
}

// Rewrite rewrites the current request when the name ends with the matching string.
func (rule *suffixTTLRule) Rewrite(ctx context.Context, state request.Request) (ResponseRules, Result) {
	return rule.responseRule(strings.HasSuffix(state.Name(), rule.Suffix))
}

// Rewrite rewrites the current request based upon partial match of the
// name in the question section of the request.
func (rule *substringTTLRule) Rewrite(ctx context.Context, state request.Request) (ResponseRules, Result) {
	return rule.responseRule(strings.Contains(state.Name(), rule.Substring))
}

// Rewrite rewrites the current request when the name in the question
// section of the request matches a regular expression.
func (rule *regexTTLRule) Rewrite(ctx context.Context, state request.Request) (ResponseRules, Result) {
	return rule.responseRule(len(rule.Pattern.FindStringSubmatch(state.Name())) != 0)
}

// newTTLRule creates a name matching rule based on exact, partial, or regex match
func newTTLRule(nextAction string, args ...string) (Rule, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("too few (%d) arguments for a ttl rule", len(args))
	}
	var s string
	if len(args) == 2 {
		s = args[1]
	}
	if len(args) == 3 {
		s = args[2]
	}
	ttl, valid := isValidTTL(s)
	if !valid {
		return nil, fmt.Errorf("invalid TTL '%s' for a ttl rule", s)
	}
	if len(args) == 3 {
		switch strings.ToLower(args[0]) {
		case ExactMatch:
			return &exactTTLRule{
				newTTLRuleBase(nextAction, ttl),
				plugin.Name(args[1]).Normalize(),
			}, nil
		case PrefixMatch:
			return &prefixTTLRule{
				newTTLRuleBase(nextAction, ttl),
				plugin.Name(args[1]).Normalize(),
			}, nil
		case SuffixMatch:
			return &suffixTTLRule{
				newTTLRuleBase(nextAction, ttl),
				plugin.Name(args[1]).Normalize(),
			}, nil
		case SubstringMatch:
			return &substringTTLRule{
				newTTLRuleBase(nextAction, ttl),
				plugin.Name(args[1]).Normalize(),
			}, nil
		case RegexMatch:
			regexPattern, err := regexp.Compile(args[1])
			if err != nil {
				return nil, fmt.Errorf("invalid regex pattern in a ttl rule: %s", args[1])
			}
			return &regexTTLRule{
				newTTLRuleBase(nextAction, ttl),
				regexPattern,
			}, nil
		default:
			return nil, fmt.Errorf("ttl rule supports only exact, prefix, suffix, substring, and regex name matching")
		}
	}
	if len(args) > 3 {
		return nil, fmt.Errorf("many few arguments for a ttl rule")
	}
	return &exactTTLRule{
		newTTLRuleBase(nextAction, ttl),
		plugin.Name(args[0]).Normalize(),
	}, nil
}

// validTTL returns true if v is valid TTL value.
func isValidTTL(v string) (uint32, bool) {
	i, err := strconv.Atoi(v)
	if err != nil {
		return uint32(0), false
	}
	if i > 2147483647 {
		return uint32(0), false
	}
	if i < 0 {
		return uint32(0), false
	}
	return uint32(i), true
}
