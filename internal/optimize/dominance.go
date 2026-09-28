package optimize

// Dominates reports whether loss vector a is strictly better than loss vector b.
//
// Callers must provide equal-length vectors containing finite values. The
// function does not validate that precondition. It compares exact float64
// values, requires no greater loss in every objective, and requires a strict
// improvement in at least one objective.
func Dominates(a, b []float64) bool {
	strictlyBetter := false

	// Compare validated objectives in order without rounding or normalization.
	for index, aLoss := range a {
		bLoss := b[index]
		if aLoss > bLoss {
			return false
		}
		if aLoss < bLoss {
			strictlyBetter = true
		}
	}

	return strictlyBetter
}
