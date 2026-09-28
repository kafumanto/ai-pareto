package domain

import (
	"errors"
	"math"
	"reflect"
	"testing"
)

/**
 * TestGoalsEvaluateExactLosses verifies minimum identity and maximum negation.
 * Expected: both goals accept 4 and return finite losses 4 and -4.
 * [SPEC] openspec/changes/establish-portable-domain-contracts/specs/portable-domain-contracts/spec.md, heading "Requirement: Goals and objectives".
 */
func TestGoalsEvaluateExactLosses(t *testing.T) {
	tests := []struct {
		name string
		goal Goal
		want float64
	}{
		{name: "min", goal: NewMinGoal(), want: 4},
		{name: "max", goal: NewMaxGoal(), want: -4},
	}

	// Check each canonical goal with the same finite scalar value.
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			loss, err := test.goal.Loss(4)
			if err != nil {
				t.Fatalf("Loss() error = %v", err)
			}
			if loss != test.want {
				t.Fatalf("Loss() = %v, want %v", loss, test.want)
			}
		})
	}
}

/**
 * TestGoalsRejectNonFiniteValues rejects non-finite values before loss evaluation.
 * Expected: minimum and maximum goals return errors for NaN and both infinities.
 * [SPEC] openspec/changes/establish-portable-domain-contracts/specs/portable-domain-contracts/spec.md, heading "Requirement: Goals and objectives".
 */
func TestGoalsRejectNonFiniteValues(t *testing.T) {
	values := []struct {
		name  string
		value float64
	}{
		{name: "nan", value: math.NaN()},
		{name: "positive infinity", value: math.Inf(1)},
		{name: "negative infinity", value: math.Inf(-1)},
	}

	// Apply every invalid value to both supported goals.
	for _, value := range values {
		for _, goal := range []Goal{NewMinGoal(), NewMaxGoal()} {
			_, err := goal.Loss(value.value)
			if !errors.Is(err, ErrNonFiniteValue) {
				t.Fatalf("Loss(%s) error = %v, want %v", value.name, err, ErrNonFiniteValue)
			}
		}
	}
}

/**
 * TestObjectiveEvaluatePreservesRecord validates expression-first objective evaluation.
 * Expected: the raw value and loss are returned, and evaluation errors leave the record unchanged.
 * [SPEC] openspec/changes/establish-portable-domain-contracts/specs/portable-domain-contracts/spec.md, heading "Requirement: Goals and objectives".
 */
func TestObjectiveEvaluatePreservesRecord(t *testing.T) {
	record := Record{ID: "variant-1", Metrics: map[string]float64{"score": 4}}
	original := record
	objective := NewObjective(NewMetricExpression("score"), NewMaxGoal())

	value, loss, err := objective.Evaluate(record)
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}
	if value != 4 || loss != -4 {
		t.Fatalf("Evaluate() = (%v, %v), want (4, -4)", value, loss)
	}
	if !reflect.DeepEqual(record, original) {
		t.Fatalf("Evaluate() mutated record: got %#v, want %#v", record, original)
	}

	invalid := NewObjective(NewMetricExpression("missing"), NewMinGoal())
	if _, _, err := invalid.Evaluate(record); err == nil {
		t.Fatal("Evaluate() error = nil for missing metric")
	}
}
