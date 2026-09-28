package domain

// minGoal minimizes the scalar value directly.
type minGoal struct{}

// NewMinGoal returns a goal that minimizes the scalar value.
func NewMinGoal() Goal {
	return minGoal{}
}

// Expression returns the canonical minimum-goal expression.
func (minGoal) Expression() string {
	return "min"
}

// Loss returns value when value is finite.
func (minGoal) Loss(value float64) (float64, error) {
	// Reject non-finite input because the loss vector must remain finite.
	if err := validateFinite(value); err != nil {
		return 0, err
	}

	return value, nil
}
