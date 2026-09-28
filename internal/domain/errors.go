package domain

import "errors"

var (
	// ErrMissingMetric indicates that a required source-native metric is absent.
	ErrMissingMetric = errors.New("required metric is missing")
	// ErrNonFiniteValue indicates that a metric or objective value is NaN or infinite.
	ErrNonFiniteValue = errors.New("value is non-finite")
)
