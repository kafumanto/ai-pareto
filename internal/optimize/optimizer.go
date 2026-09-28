// Package optimize defines exact comparison and optimizer integration contracts.
package optimize

import (
	"context"

	"github.com/kafumanto/ai-pareto/internal/domain"
)

// Request contains the original records, ordered objectives, and requested ranks for one computation.
type Request struct {
	// Records contains the original source records that the optimizer must preserve in results.
	Records []domain.Record
	// Objectives contains the ordered scalar objectives used to form loss vectors.
	Objectives []domain.Objective
	// Ranks contains requested ranks in caller order; an empty slice means rank one.
	Ranks []int
}

// EffectiveRanks returns rank one for omitted ranks or a copy of the requested rank order.
func (request Request) EffectiveRanks() []int {
	if len(request.Ranks) == 0 {
		return []int{1}
	}

	// Copy explicit ranks so callers cannot mutate the request through the returned slice.
	ranks := make([]int, len(request.Ranks))
	copy(ranks, request.Ranks)
	return ranks
}

// Result contains the original records selected for one requested rank.
type Result struct {
	// Records contains original input records rather than loss-only projections.
	Records []domain.Record
	// Rank identifies the requested rank represented by Records.
	Rank int
}

// Optimizer computes rank-tagged results for a request.
//
// Implementations must accept a context, periodically check cancellation, and
// return a context error when cancellation stops computation.
type Optimizer interface {
	// Compute returns rank-tagged results or an error, including context cancellation.
	Compute(ctx context.Context, request Request) ([]Result, error)
}
