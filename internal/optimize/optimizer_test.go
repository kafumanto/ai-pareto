package optimize

import (
	"reflect"
	"testing"

	"github.com/kafumanto/ai-pareto/internal/domain"
)

/**
 * TestRequestEffectiveRanks defaults omitted ranks and preserves explicit order.
 * Expected: an empty request returns [1], while [3, 1] remains [3, 1].
 * [SPEC] openspec/changes/establish-portable-domain-contracts/specs/portable-domain-contracts/spec.md, heading "Requirement: Optimizer integration contract".
 */
func TestRequestEffectiveRanks(t *testing.T) {
	if got := (Request{}).EffectiveRanks(); !reflect.DeepEqual(got, []int{1}) {
		t.Fatalf("EffectiveRanks() = %#v, want [1]", got)
	}

	request := Request{Ranks: []int{3, 1}}
	got := request.EffectiveRanks()
	if !reflect.DeepEqual(got, []int{3, 1}) {
		t.Fatalf("EffectiveRanks() = %#v, want [3 1]", got)
	}
	got[0] = 99
	if !reflect.DeepEqual(request.Ranks, []int{3, 1}) {
		t.Fatalf("EffectiveRanks() exposed request ranks: %#v", request.Ranks)
	}
}

/**
 * TestResultPreservesOriginalRecords checks that result records retain domain identity and values.
 * Expected: a result can carry the original records without loss-vector projection.
 * [SPEC] openspec/changes/establish-portable-domain-contracts/specs/portable-domain-contracts/spec.md, heading "Requirement: Optimizer integration contract".
 */
func TestResultPreservesOriginalRecords(t *testing.T) {
	record := domain.Record{ID: "variant-1", Name: "Model", Metrics: map[string]float64{"score": 4}}
	result := Result{Records: []domain.Record{record}, Rank: 1}
	if !reflect.DeepEqual(result.Records[0], record) {
		t.Fatalf("Result.Records[0] = %#v, want %#v", result.Records[0], record)
	}
}
