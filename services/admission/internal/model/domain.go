// Package model holds the pure PPDB domain logic: application and payment
// state machines, the ordered discount engine, and the weighted ranking
// algorithm. Nothing in this package touches the database or the network so
// every rule here can be exercised by fast, deterministic unit tests.
package model

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

// ---------------------------------------------------------------------------
// Application lifecycle
// ---------------------------------------------------------------------------

// ApplicationStatus is the lifecycle state of a PPDB application. The set and
// order mirror docs/CONVENTIONS.md and the frontend union in
// web/src/services/admission/types.ts.
type ApplicationStatus string

const (
	StatusDraft           ApplicationStatus = "draft"
	StatusSubmitted       ApplicationStatus = "submitted"
	StatusDocumentReview  ApplicationStatus = "document_review"
	StatusEligibleForTest ApplicationStatus = "eligible_for_test"
	StatusTested          ApplicationStatus = "tested"
	StatusRanked          ApplicationStatus = "ranked"
	StatusOffered         ApplicationStatus = "offered"
	StatusWaitlisted      ApplicationStatus = "waitlisted"
	StatusRejected        ApplicationStatus = "rejected"
	StatusReregistration  ApplicationStatus = "reregistration"
	StatusConverted       ApplicationStatus = "converted"
	StatusWithdrawn       ApplicationStatus = "withdrawn"
	// StatusDropped is "gugur": the form fee (BR-PPDB-07) or the minimum
	// re-registration payment (BR-PPDB-08) was not met by its deadline.
	StatusDropped ApplicationStatus = "dropped"
)

// applicationTransitions is the single source of truth for allowed status
// changes. Terminal states (converted, withdrawn) have no outgoing edges;
// rejected applications stay in history and are corrected with a new event.
var applicationTransitions = map[ApplicationStatus]map[ApplicationStatus]bool{
	StatusDraft:           {StatusSubmitted: true, StatusWithdrawn: true},
	StatusSubmitted:       {StatusDocumentReview: true, StatusRejected: true, StatusWithdrawn: true, StatusDropped: true},
	StatusDocumentReview:  {StatusEligibleForTest: true, StatusRejected: true, StatusWithdrawn: true},
	StatusEligibleForTest: {StatusTested: true, StatusRejected: true, StatusWithdrawn: true},
	StatusTested:          {StatusRanked: true, StatusRejected: true, StatusWithdrawn: true},
	StatusRanked:          {StatusOffered: true, StatusWaitlisted: true, StatusRejected: true},
	StatusOffered:         {StatusReregistration: true, StatusWithdrawn: true, StatusDropped: true},
	StatusWaitlisted:      {StatusOffered: true, StatusRejected: true, StatusWithdrawn: true},
	StatusReregistration:  {StatusConverted: true, StatusWithdrawn: true, StatusDropped: true},
}

// CanTransition reports whether from -> to is a legal application transition.
func CanTransition(from, to ApplicationStatus) bool {
	return applicationTransitions[from][to]
}

// ReasonRequired reports whether reaching the target status demands an
// operator supplied reason (corrective/terminal actions).
func ReasonRequired(to ApplicationStatus) bool {
	switch to {
	case StatusRejected, StatusWithdrawn, StatusDropped:
		return true
	default:
		return false
	}
}

// ValidateTransition enforces the transition table and the reason requirement.
func ValidateTransition(from, to ApplicationStatus, reason string) error {
	if from == to {
		return fmt.Errorf("application is already %s", from)
	}
	if !CanTransition(from, to) {
		return fmt.Errorf("invalid application transition %s -> %s", from, to)
	}
	if ReasonRequired(to) && strings.TrimSpace(reason) == "" {
		return fmt.Errorf("a reason is required to move an application to %s", to)
	}
	return nil
}

// guardianTransitions are the transitions a guardian session is allowed to
// trigger; everything else is an admin/committee action.
var guardianTransitions = map[ApplicationStatus]bool{
	StatusSubmitted:      true,
	StatusWithdrawn:      true,
	StatusReregistration: true,
}

// GuardianCanTransition reports whether a guardian may perform from -> to.
func GuardianCanTransition(from, to ApplicationStatus) bool {
	return CanTransition(from, to) && guardianTransitions[to]
}

// ---------------------------------------------------------------------------
// Payment state machine
// ---------------------------------------------------------------------------

// PaymentStatus is the lifecycle of a payment submission.
type PaymentStatus string

const (
	PaymentSubmitted     PaymentStatus = "submitted"
	PaymentPendingReview PaymentStatus = "pending_review"
	PaymentVerified      PaymentStatus = "verified"
	PaymentRejected      PaymentStatus = "rejected"
	PaymentReversed      PaymentStatus = "reversed"
)

var paymentTransitions = map[PaymentStatus]map[PaymentStatus]bool{
	PaymentSubmitted:     {PaymentPendingReview: true, PaymentVerified: true, PaymentRejected: true},
	PaymentPendingReview: {PaymentVerified: true, PaymentRejected: true},
	PaymentVerified:      {PaymentReversed: true},
}

// ValidatePaymentTransition enforces the payment state machine. Verified
// payments may only be corrected by a reversal event, never deleted.
func ValidatePaymentTransition(from, to PaymentStatus) error {
	if from == to {
		return fmt.Errorf("payment is already %s", from)
	}
	if !paymentTransitions[from][to] {
		return fmt.Errorf("invalid payment transition %s -> %s", from, to)
	}
	return nil
}

// PaymentAction maps the admin review action names used by the frontend to a
// payment status.
func PaymentAction(action string) (PaymentStatus, error) {
	switch strings.ToLower(strings.TrimSpace(action)) {
	case "verify", "verified":
		return PaymentVerified, nil
	case "reject", "rejected":
		return PaymentRejected, nil
	default:
		return "", fmt.Errorf("unknown payment action %q", action)
	}
}

// Allocation is a partial or full settlement of a payment against one
// installment plan line.
type Allocation struct {
	InstallmentID string `json:"installment_id"`
	Amount        int64  `json:"amount"`
}

// ValidateAllocations enforces the invariant that a verified payment is fully
// allocated: every line is positive and the lines sum exactly to the payment.
func ValidateAllocations(paymentAmount int64, allocations []Allocation) error {
	if paymentAmount <= 0 {
		return errors.New("payment amount must be positive")
	}
	if len(allocations) == 0 {
		return errors.New("at least one allocation is required to verify a payment")
	}
	var sum int64
	for _, a := range allocations {
		if a.Amount <= 0 {
			return errors.New("allocation amount must be positive")
		}
		sum += a.Amount
	}
	if sum != paymentAmount {
		return fmt.Errorf("allocations total %d does not match payment amount %d", sum, paymentAmount)
	}
	return nil
}

// Outstanding computes the remaining balance after verified allocations and
// reversals: charge - verified allocations + reversal amounts.
func Outstanding(charge int64, verifiedAllocations, reversals []int64) (int64, error) {
	if charge < 0 {
		return 0, errors.New("charge must not be negative")
	}
	out := charge
	for _, v := range verifiedAllocations {
		if v <= 0 {
			return 0, errors.New("allocation must be positive")
		}
		out -= v
	}
	for _, v := range reversals {
		if v <= 0 {
			return 0, errors.New("reversal must be positive")
		}
		out += v
	}
	if out < 0 {
		return 0, errors.New("verified allocations exceed the charge")
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// Discount engine
// ---------------------------------------------------------------------------

// DiscountKind is the discount mode. Percent values are expressed in basis
// points (10000 == 100%) so no floating point arithmetic is ever used.
type DiscountKind string

const (
	DiscountFixed   DiscountKind = "fixed"
	DiscountPercent DiscountKind = "percent"
	percentScale                 = 10000
)

// DiscountCandidate is one discount considered while computing a charge.
type DiscountCandidate struct {
	ID        string
	Source    string // automatic | voucher | manual
	Kind      DiscountKind
	Value     int64
	Cap       int64 // 0 means "no per-rule cap"
	Priority  int
	CreatedAt time.Time
	// Exclusive marks a non-stackable promo: it is only ever applied alone.
	Exclusive bool
}

// DiscountStep is the immutable audit snapshot for one applied discount.
type DiscountStep struct {
	ID         string `json:"id"`
	Source     string `json:"source"`
	Label      string `json:"label"`
	Priority   int    `json:"priority"`
	BaseBefore int64  `json:"base_before"`
	Requested  int64  `json:"requested"`
	Applied    int64  `json:"applied"`
	BaseAfter  int64  `json:"base_after"`
}

// ApplyDiscounts stacks every candidate onto base in a deterministic order:
// ascending priority, then ascending created_at, then id as a stable
// tie-break. Each step records the running base, so the returned slice is a
// complete audit snapshot. The floor is zero: a discount can never make an
// item negative.
func ApplyDiscounts(base int64, in []DiscountCandidate) (int64, []DiscountStep, error) {
	if base < 0 {
		return 0, nil, errors.New("base amount must not be negative")
	}
	ordered := append([]DiscountCandidate(nil), in...)
	sort.SliceStable(ordered, func(i, j int) bool {
		if ordered[i].Priority != ordered[j].Priority {
			return ordered[i].Priority < ordered[j].Priority
		}
		if !ordered[i].CreatedAt.Equal(ordered[j].CreatedAt) {
			return ordered[i].CreatedAt.Before(ordered[j].CreatedAt)
		}
		return ordered[i].ID < ordered[j].ID
	})

	out := base
	steps := make([]DiscountStep, 0, len(ordered))
	for _, d := range ordered {
		if d.Value < 0 {
			return 0, nil, fmt.Errorf("discount %s has a negative value", d.ID)
		}
		var requested int64
		switch d.Kind {
		case DiscountFixed:
			requested = d.Value
		case DiscountPercent:
			if d.Value > percentScale {
				return 0, nil, fmt.Errorf("discount %s exceeds 100%%", d.ID)
			}
			requested = out * d.Value / percentScale
		default:
			return 0, nil, fmt.Errorf("discount %s has an invalid kind %q", d.ID, d.Kind)
		}
		applied := requested
		if d.Cap > 0 && applied > d.Cap {
			applied = d.Cap
		}
		if applied > out {
			applied = out
		}
		before := out
		out -= applied
		steps = append(steps, DiscountStep{
			ID:         d.ID,
			Source:     d.Source,
			Label:      d.Source,
			Priority:   d.Priority,
			BaseBefore: before,
			Requested:  requested,
			Applied:    applied,
			BaseAfter:  out,
		})
	}
	return out, steps, nil
}

// ApplyDiscountPolicy applies BR-PPDB-02 on top of ApplyDiscounts.
//
//   - mode "max" keeps only the single candidate with the largest discount.
//   - mode "stack" stacks every stackable candidate; an exclusive
//     (non-stackable) candidate is only ever applied alone, so the option with
//     the larger total wins (ties keep the stacked set).
//
// maxTotal > 0 caps the summed discount per applicant by trimming the latest
// steps.
func ApplyDiscountPolicy(base int64, in []DiscountCandidate, mode string, maxTotal int64) (int64, []DiscountStep, error) {
	if base < 0 {
		return 0, nil, errors.New("base amount must not be negative")
	}
	var options [][]DiscountCandidate
	if mode == "max" {
		for _, c := range in {
			options = append(options, []DiscountCandidate{c})
		}
	} else {
		var stack []DiscountCandidate
		for _, c := range in {
			if c.Exclusive {
				options = append(options, []DiscountCandidate{c})
			} else {
				stack = append(stack, c)
			}
		}
		options = append([][]DiscountCandidate{stack}, options...)
	}
	bestNet, bestSteps := base, []DiscountStep{}
	for _, option := range options {
		net, steps, err := ApplyDiscounts(base, option)
		if err != nil {
			return 0, nil, err
		}
		if net < bestNet {
			bestNet, bestSteps = net, steps
		}
	}
	if maxTotal <= 0 || base-bestNet <= maxTotal {
		return bestNet, bestSteps, nil
	}
	remaining := maxTotal
	running := base
	kept := make([]DiscountStep, 0, len(bestSteps))
	for _, s := range bestSteps {
		if remaining <= 0 {
			break
		}
		if s.Applied > remaining {
			s.Applied = remaining
		}
		remaining -= s.Applied
		s.BaseBefore = running
		running -= s.Applied
		s.BaseAfter = running
		kept = append(kept, s)
	}
	return running, kept, nil
}

// MinimumPayment is the BR-PPDB-04 threshold: pct percent of net, rounded up.
func MinimumPayment(net int64, pct int) int64 {
	if pct <= 0 || net <= 0 {
		return 0
	}
	if pct > 100 {
		pct = 100
	}
	return (net*int64(pct) + 99) / 100
}

// ---------------------------------------------------------------------------
// Ranking
// ---------------------------------------------------------------------------

// RankInput is one eligible application's final component scores.
type RankInput struct {
	ApplicationID      string
	RegistrationNumber string
	SubmittedAt        time.Time
	Scores             map[string]int64
}

// Formula is the compiled ranking formula. Weights are basis points and must
// total 10000. Minimums are per-component floors, MinimumTotal is the overall
// floor. TieBreak lists component codes consulted (in order, descending)
// before the deterministic fallbacks.
type Formula struct {
	Weights      map[string]int64
	Minimums     map[string]int64
	MinimumTotal int64
	TieBreak     []string
}

// RankEntry is one line of a ranking result.
type RankEntry struct {
	ApplicationID  string `json:"application_id"`
	Total          int64  `json:"total_score"`
	Rank           int    `json:"rank"`
	Recommendation string `json:"recommendation"`
}

// RecommendedDecision maps a rank to the default recommendation given the
// track quota.
func RecommendedDecision(rank, quota int) string {
	if rank <= quota {
		return "accepted"
	}
	return "waitlisted"
}

// Rank scores and orders eligible inputs, assigning ranks and recommendations
// against the quota. It is deterministic and never mutates its inputs: the
// same inputs always produce identical output, which is what makes a
// persisted ranking snapshot trustworthy.
func Rank(inputs []RankInput, f Formula, quota int) ([]RankEntry, error) {
	if quota < 0 {
		return nil, errors.New("quota must not be negative")
	}
	var weightSum int64
	minimums := make(map[string]int64, len(f.Weights))
	for code, weight := range f.Weights {
		if weight < 0 {
			return nil, errors.New("weights must not be negative")
		}
		weightSum += weight
		minimums[code] = f.Minimums[code]
	}
	if weightSum != percentScale {
		return nil, fmt.Errorf("weights must total 100%%, got %d basis points", weightSum)
	}

	eligible := make([]RankEntry, 0, len(inputs))
	byID := make(map[string]RankInput, len(inputs))
	for _, in := range inputs {
		var total int64
		ok := true
		for code, weight := range f.Weights {
			score, exists := in.Scores[code]
			if !exists || score < minimums[code] {
				ok = false
				break
			}
			total += score * weight / percentScale
		}
		if !ok || total < f.MinimumTotal {
			continue
		}
		eligible = append(eligible, RankEntry{ApplicationID: in.ApplicationID, Total: total})
		byID[in.ApplicationID] = in
	}

	sort.SliceStable(eligible, func(i, j int) bool {
		a, b := eligible[i], eligible[j]
		if a.Total != b.Total {
			return a.Total > b.Total
		}
		ai, bi := byID[a.ApplicationID], byID[b.ApplicationID]
		for _, code := range f.TieBreak {
			if ai.Scores[code] != bi.Scores[code] {
				return ai.Scores[code] > bi.Scores[code]
			}
		}
		if !ai.SubmittedAt.Equal(bi.SubmittedAt) {
			return ai.SubmittedAt.Before(bi.SubmittedAt)
		}
		return ai.RegistrationNumber < bi.RegistrationNumber
	})

	for i := range eligible {
		eligible[i].Rank = i + 1
		eligible[i].Recommendation = RecommendedDecision(i+1, quota)
	}
	return eligible, nil
}

// Component is one weighted element of a selection formula, as configured by
// an admin. Weight is stored in basis points.
type Component struct {
	Code    string `json:"code"`
	Name    string `json:"name"`
	Source  string `json:"source"`
	Weight  int64  `json:"weight"`
	Minimum int64  `json:"minimum"`
}

// FormulaSpec is the admin-editable form of a formula.
type FormulaSpec struct {
	Name         string      `json:"name"`
	Version      int         `json:"version"`
	Status       string      `json:"status"`
	MinimumTotal int64       `json:"minimum_total"`
	Components   []Component `json:"components"`
	TieBreakers  []string    `json:"tie_breakers"`
}

// Compile validates the admin configuration and produces the Rank formula.
func (s FormulaSpec) Compile() (Formula, error) {
	if strings.TrimSpace(s.Name) == "" {
		return Formula{}, errors.New("formula name is required")
	}
	if len(s.Components) == 0 {
		return Formula{}, errors.New("at least one selection component is required")
	}
	weights := map[string]int64{}
	minimums := map[string]int64{}
	var total int64
	for _, c := range s.Components {
		if strings.TrimSpace(c.Code) == "" {
			return Formula{}, errors.New("every component needs a code")
		}
		if c.Weight < 0 {
			return Formula{}, fmt.Errorf("component %s has a negative weight", c.Code)
		}
		if _, dup := weights[c.Code]; dup {
			return Formula{}, fmt.Errorf("duplicate component code %s", c.Code)
		}
		weights[c.Code] = c.Weight
		minimums[c.Code] = c.Minimum
		total += c.Weight
	}
	if total != percentScale {
		return Formula{}, fmt.Errorf("component weights must total 100%%, got %d basis points", total)
	}
	return Formula{Weights: weights, Minimums: minimums, MinimumTotal: s.MinimumTotal, TieBreak: s.TieBreakers}, nil
}

// Slug produces a stable component code from a human name when the admin did
// not supply one.
func Slug(name string) string {
	var b strings.Builder
	prevDash := false
	for _, r := range strings.ToLower(strings.TrimSpace(name)) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			prevDash = false
		default:
			if !prevDash && b.Len() > 0 {
				b.WriteByte('_')
				prevDash = true
			}
		}
	}
	return strings.Trim(b.String(), "_")
}
