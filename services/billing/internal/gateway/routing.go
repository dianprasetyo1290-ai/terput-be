package gateway

import (
	"fmt"
	"sort"

	"skci/billing/internal/model"
)

// DefaultProvider is the provider T6 always used; with no routing rule a
// method goes there when it is enabled (T6 compatibility).
const DefaultProvider = "nicepay"

// ProviderInfo is the routing view of a payment_providers row.
type ProviderInfo struct {
	Code         string
	Enabled      bool
	Capabilities []string
}

// Rule is an active payment_routing_rules row.
type Rule struct {
	ID              string
	MethodKind      string
	MethodCode      string // "" = every method of the kind
	Primary         string
	Fallback        string // "" = none
	FailoverEnabled bool
}

// MethodRef is the method being paid with.
type MethodRef struct {
	Kind string
	Code string
}

// Decision is where a new transaction goes. Fallback is only set when the
// rule allows failover and the fallback can serve the method.
// FailoverFrom is set when the primary was already unusable at routing time.
type Decision struct {
	RuleID       string `json:"rule_id,omitempty"`
	Primary      string `json:"primary"`
	Fallback     string `json:"fallback,omitempty"`
	FailoverFrom string `json:"failover_from,omitempty"`
}

func usable(providers map[string]ProviderInfo, code, kind string) bool {
	p, ok := providers[code]
	return ok && p.Enabled && HasCapability(p.Capabilities, kind)
}

// Route picks the provider for a method (FR-150-002, BR-PAY-03, AC-150-01).
// A rule for the exact method code beats a rule for the whole kind. Without
// a rule the method goes to nicepay if it can serve it. A target that is
// disabled or lacks the capability yields ErrNotRouted, unless failover is
// on and the fallback can take it (FR-150-003).
func Route(rules []Rule, method MethodRef, providers map[string]ProviderInfo) (Decision, error) {
	var kindRule, codeRule *Rule
	for i := range rules {
		r := &rules[i]
		if r.MethodKind != method.Kind {
			continue
		}
		if r.MethodCode == "" {
			kindRule = r
		} else if r.MethodCode == method.Code {
			codeRule = r
		}
	}
	rule := codeRule
	if rule == nil {
		rule = kindRule
	}
	if rule == nil {
		if usable(providers, DefaultProvider, method.Kind) {
			return Decision{Primary: DefaultProvider}, nil
		}
		return Decision{}, ErrNotRouted
	}
	fallbackOK := rule.FailoverEnabled && rule.Fallback != "" && rule.Fallback != rule.Primary &&
		usable(providers, rule.Fallback, method.Kind)
	switch {
	case usable(providers, rule.Primary, method.Kind):
		d := Decision{RuleID: rule.ID, Primary: rule.Primary}
		if fallbackOK {
			d.Fallback = rule.Fallback
		}
		return d, nil
	case fallbackOK:
		return Decision{RuleID: rule.ID, Primary: rule.Fallback, FailoverFrom: rule.Primary}, nil
	}
	return Decision{}, ErrNotRouted
}

// ShouldFailover reports whether a failed charge at d.Primary may be
// retried at d.Fallback: only outages (timeout, network, 5xx), only when the
// rule allowed it and the fallback is a different, capable provider.
func ShouldFailover(err error, d Decision) bool {
	return err != nil && d.Fallback != "" && d.Fallback != d.Primary && IsTransient(err)
}

// ProviderCode resolves the method code to send to provider: an explicit
// mapping wins; nicepay falls back to the legacy payment_methods.provider_code;
// any other provider without a mapping is an error.
func ProviderCode(method model.GatewayMethod, provider string, mappings map[string]string) (string, error) {
	if c, ok := mappings[provider]; ok && c != "" {
		return c, nil
	}
	if provider == DefaultProvider && method.ProviderCode != "" {
		return method.ProviderCode, nil
	}
	return "", fmt.Errorf("kode metode %s untuk provider %s belum dipetakan", method.Code, provider)
}

// ProviderTx is a successful transaction tagged with its provider.
type ProviderTx struct {
	Provider string
	model.ReconcileTx
}

// ProviderItem is a settlement item tagged with its batch's provider.
type ProviderItem struct {
	Provider string
	model.ReconcileItem
}

// ReconcileGroup is the input of model.Reconcile for one provider.
type ReconcileGroup struct {
	Provider string
	Txs      []model.ReconcileTx
	Items    []model.ReconcileItem
}

// GroupForReconcile splits transactions and settlement items per provider
// so a transaction of provider A is never matched with B's settlement
// (FR-150-004). Groups are sorted by provider code.
func GroupForReconcile(txs []ProviderTx, items []ProviderItem) []ReconcileGroup {
	byProvider := map[string]*ReconcileGroup{}
	get := func(p string) *ReconcileGroup {
		g, ok := byProvider[p]
		if !ok {
			g = &ReconcileGroup{Provider: p}
			byProvider[p] = g
		}
		return g
	}
	for _, t := range txs {
		g := get(t.Provider)
		g.Txs = append(g.Txs, t.ReconcileTx)
	}
	for _, it := range items {
		g := get(it.Provider)
		g.Items = append(g.Items, it.ReconcileItem)
	}
	out := make([]ReconcileGroup, 0, len(byProvider))
	for _, g := range byProvider {
		out = append(out, *g)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Provider < out[j].Provider })
	return out
}
