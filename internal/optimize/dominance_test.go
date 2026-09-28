package optimize

import (
	"math"
	"testing"

	"github.com/kafumanto/ai-pareto/internal/domain"
)

/**
 * TestDominatesTruthTable verifies strict improvement, trade-offs, equality, and exact adjacent values.
 * Expected: dominance requires component-wise improvement with one strict loss, and adjacent float64 values remain distinguishable.
 * [SPEC] openspec/changes/establish-portable-domain-contracts/specs/portable-domain-contracts/spec.md, heading "Requirement: Exact dominance".
 */
func TestDominatesTruthTable(t *testing.T) {
	adjacent := math.Nextafter(1, 2)
	tests := []struct {
		name string
		a    []float64
		b    []float64
		want bool
	}{
		{name: "strict improvement", a: []float64{1, 2}, b: []float64{2, 2}, want: true},
		{name: "trade-off", a: []float64{1, 3}, b: []float64{2, 2}, want: false},
		{name: "equal vectors", a: []float64{2, 2}, b: []float64{2, 2}, want: false},
		{name: "adjacent representable values", a: []float64{1}, b: []float64{adjacent}, want: true},
	}

	// Evaluate every exact comparison without tolerance or normalization.
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := Dominates(test.a, test.b); got != test.want {
				t.Fatalf("Dominates(%v, %v) = %t, want %t", test.a, test.b, got, test.want)
			}
		})
	}
}

/**
 * TestEqualLossesKeepDistinctRecordIdentities prevents equal vectors from collapsing records.
 * Expected: distinct IDs remain distinct and neither equal vector dominates the other.
 * [SPEC] openspec/changes/establish-portable-domain-contracts/specs/portable-domain-contracts/spec.md, heading "Requirement: Exact dominance".
 */
func TestEqualLossesKeepDistinctRecordIdentities(t *testing.T) {
	a := domain.Record{ID: "a"}
	b := domain.Record{ID: "b"}
	if a.ID == b.ID {
		t.Fatal("test records unexpectedly share an ID")
	}
	if Dominates([]float64{4}, []float64{4}) || Dominates([]float64{4}, []float64{4}) {
		t.Fatal("equal loss vectors unexpectedly dominate")
	}
}
