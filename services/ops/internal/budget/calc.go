package budget

// Pure calculation functions for Budget / RAPBS module.
// Strictly integer math, money in rupiah, rates in basis points (10000 bp = 100%).

// AbsorptionBP calculates budget absorption in basis points (e.g. 8500 = 85.00%).
// Returns 0 if plannedIDR <= 0.
func AbsorptionBP(actualIDR, plannedIDR int64) int {
	if plannedIDR <= 0 {
		if actualIDR > 0 {
			return 10000 // 100% or more if spending without budget
		}
		return 0
	}
	return int((actualIDR * 10000) / plannedIDR)
}

// Commitment sums commitments from unfulfilled PRs and unfulfilled POs.
func Commitment(openPRs, openPOs int64) int64 {
	c := openPRs + openPOs
	if c < 0 {
		return 0
	}
	return c
}

// Remaining calculates remaining available budget: planned - actual - commitment.
func Remaining(plannedIDR, actualIDR, commitmentIDR int64) int64 {
	return plannedIDR - actualIDR - commitmentIDR
}

// CheckLineBudget determines if a request exceeds the budget line and determines the enforcement policy.
// Tolerance is in basis points of plannedIDR (e.g., 500 bp = 5% tolerance).
// lockRule can be "warn", "hard_stop", "approval_required".
// Returned policy is "none", "warn", "extra_approval", or "reject".
func CheckLineBudget(plannedIDR, actualIDR, commitmentIDR, requestIDR int64, toleranceBP int, lockRule string) (over bool, policy string) {
	if plannedIDR <= 0 {
		// No budget planned at all for this line
		switch lockRule {
		case "hard_stop":
			return true, "reject"
		case "approval_required":
			return true, "extra_approval"
		default:
			return true, "warn"
		}
	}

	totalUsed := actualIDR + commitmentIDR + requestIDR
	toleranceAmount := (plannedIDR * int64(toleranceBP)) / 10000
	maxAllowed := plannedIDR + toleranceAmount

	if totalUsed <= plannedIDR {
		return false, "none"
	}

	// Exceeded planned budget
	if totalUsed <= maxAllowed {
		// Within tolerance
		if lockRule == "hard_stop" {
			return true, "warn"
		}
		return true, "warn"
	}

	// Exceeded budget + tolerance
	switch lockRule {
	case "hard_stop":
		return true, "reject"
	case "approval_required":
		return true, "extra_approval"
	default: // "warn"
		return true, "warn"
	}
}

// CrossedWarn checks if actual spending has crossed the warning threshold (e.g., 8000 bp = 80%).
func CrossedWarn(actualIDR, plannedIDR int64, warnThresholdBP int) bool {
	if plannedIDR <= 0 || warnThresholdBP <= 0 {
		return false
	}
	currBP := AbsorptionBP(actualIDR, plannedIDR)
	return currBP >= warnThresholdBP
}
