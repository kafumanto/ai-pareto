package domain

import "errors"

// Goal converts a finite scalar value into a finite loss where lower is better.
type Goal interface {
	// Expression returns the stable canonical representation of the goal.
	Expression() string
	// Loss returns the finite normalized loss for value or an error.
	Loss(value float64) (float64, error)
}

// Objective combines one value expression with one loss goal.
type Objective struct {
	// Value computes the raw scalar value to optimize.
	Value ValueExpression
	// Goal converts the raw scalar value into normalized loss.
	Goal Goal
}

// NewObjective returns an objective that evaluates value and applies goal.
func NewObjective(value ValueExpression, goal Goal) Objective {
	return Objective{Value: value, Goal: goal}
}

// Evaluate returns the raw value and normalized loss without modifying record.
func (objective Objective) Evaluate(record Record) (float64, float64, error) {
	// Reject an incomplete objective before calling either contract.
	if objective.Value == nil {
		return 0, 0, errors.New("objective value expression is nil")
	}
	if objective.Goal == nil {
		return 0, 0, errors.New("objective goal is nil")
	}

	// Evaluate the source value first so goal errors cannot hide expression errors.
	value, err := objective.Value.Evaluate(record)
	if err != nil {
		return 0, 0, err
	}

	// Apply the goal to the raw value and preserve the input record unchanged.
	loss, err := objective.Goal.Loss(value)
	if err != nil {
		return 0, 0, err
	}

	return value, loss, nil
}
