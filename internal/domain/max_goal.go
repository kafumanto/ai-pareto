package domain

// maxGoal maximizes the scalar value by negating it into a loss.
type maxGoal struct{}

// NewMaxGoal returns a goal that maximizes the scalar value.
func NewMaxGoal() Goal {
	return maxGoal{}
}

// Expression returns the canonical maximum-goal expression.
func (maxGoal) Expression() string {
	return "max"
}

// Loss returns the negated value when negation remains finite.
func (maxGoal) Loss(value float64) (float64, error) {
	// Reject non-finite input because the loss vector must remain finite.
	if err := validateFinite(value); err != nil {
		return 0, err
	}

	// Negate only after validating the input so the operation remains exact.
	return -value, nil
}
