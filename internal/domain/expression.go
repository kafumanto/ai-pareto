package domain

// ValueExpression converts a record into one finite scalar value.
//
// Implementations report a stable canonical expression and the source-native
// metric names required for evaluation. Evaluation must not mutate record.
type ValueExpression interface {
	// Expression returns the stable canonical representation of the expression.
	Expression() string
	// RequiredMetrics returns unique source-native metric names in expression order.
	RequiredMetrics() []string
	// Evaluate returns the finite scalar value for record or an evaluation error.
	Evaluate(record Record) (float64, error)
}

// UnionRequiredMetrics returns metric names once in their first expression appearance order.
func UnionRequiredMetrics(expressions ...ValueExpression) []string {
	seen := make(map[string]struct{})
	required := make([]string, 0)

	// Process expressions from left to right so callers retain objective order.
	for _, expression := range expressions {
		if expression == nil {
			continue
		}
		for _, name := range expression.RequiredMetrics() {
			if _, exists := seen[name]; exists {
				continue
			}
			seen[name] = struct{}{}
			required = append(required, name)
		}
	}

	return required
}
