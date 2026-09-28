package domain

import "fmt"

// metricExpression reads one named source-native metric from a record.
type metricExpression struct {
	// name is the source-native metric name to read.
	name string
}

// NewMetricExpression returns a value expression that reads name from a record.
func NewMetricExpression(name string) ValueExpression {
	return metricExpression{name: name}
}

// Expression returns the metric name as the canonical expression.
func (expression metricExpression) Expression() string {
	return expression.name
}

// RequiredMetrics returns the one metric required by the expression.
func (expression metricExpression) RequiredMetrics() []string {
	return []string{expression.name}
}

// Evaluate returns the finite metric value or an error when the metric is absent or non-finite.
func (expression metricExpression) Evaluate(record Record) (float64, error) {
	// Check map presence separately so a present zero value is accepted.
	value, present := record.Metrics[expression.name]
	if !present {
		return 0, fmt.Errorf("metric %q: %w", expression.name, ErrMissingMetric)
	}

	// Reject NaN and both infinities because optimization requires finite values.
	if err := validateFinite(value); err != nil {
		return 0, fmt.Errorf("metric %q: %w", expression.name, err)
	}

	return value, nil
}
