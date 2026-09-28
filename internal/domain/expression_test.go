package domain

import (
	"errors"
	"math"
	"reflect"
	"testing"
)

/**
 * TestMetricExpressionEvaluate covers finite lookup, missing values, and non-finite rejection.
 * Expected: valid values are returned, invalid values return errors, and the record remains unchanged.
 * [SPEC] openspec/changes/establish-portable-domain-contracts/specs/portable-domain-contracts/spec.md, heading "Requirement: Metric value expression".
 */
func TestMetricExpressionEvaluate(t *testing.T) {
	record := Record{
		ID:         "variant-1",
		Name:       "Model",
		Source:     "source",
		Provider:   "provider",
		Metrics:    map[string]float64{"score": 4, "nan": math.NaN(), "inf": math.Inf(1)},
		Attributes: map[string]string{"effort": "high"},
	}
	original := record

	tests := []struct {
		name       string
		expression ValueExpression
		want       float64
		wantError  error
	}{
		{name: "finite", expression: NewMetricExpression("score"), want: 4},
		{name: "missing", expression: NewMetricExpression("missing"), wantError: ErrMissingMetric},
		{name: "nan", expression: NewMetricExpression("nan"), wantError: ErrNonFiniteValue},
		{name: "positive infinity", expression: NewMetricExpression("inf"), wantError: ErrNonFiniteValue},
	}

	// Evaluate each supported input class using the same unchanged record.
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			value, err := test.expression.Evaluate(record)
			if !errors.Is(err, test.wantError) {
				t.Fatalf("Evaluate() error = %v, want %v", err, test.wantError)
			}
			if test.wantError == nil && value != test.want {
				t.Fatalf("Evaluate() = %v, want %v", value, test.want)
			}
		})
	}

	if !reflect.DeepEqual(record, original) {
		t.Fatalf("Evaluate() mutated record: got %#v, want %#v", record, original)
	}
}

/**
 * TestUnionRequiredMetrics preserves unique metric names in first-appearance order.
 * Expected: overlapping requirements appear once and later expressions cannot reorder them.
 * [SPEC] openspec/changes/establish-portable-domain-contracts/specs/portable-domain-contracts/spec.md, heading "Requirement: Metric value expression".
 */
func TestUnionRequiredMetrics(t *testing.T) {
	got := UnionRequiredMetrics(
		NewMetricExpression("latency"),
		NewMetricExpression("quality"),
		NewMetricExpression("latency"),
		NewMetricExpression("cost"),
	)
	want := []string{"latency", "quality", "cost"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("UnionRequiredMetrics() = %#v, want %#v", got, want)
	}
}
