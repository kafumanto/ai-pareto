package domain

import "math"

// isFinite reports whether value is neither NaN nor positive or negative infinity.
func isFinite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}

// validateFinite returns ErrNonFiniteValue when value is NaN or infinite.
func validateFinite(value float64) error {
	if isFinite(value) {
		return nil
	}

	return ErrNonFiniteValue
}
