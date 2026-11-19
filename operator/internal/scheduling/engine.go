package scheduling

import "time"

// DefaultEngine is the Phase 1 decision engine: Filter, then Score,
// then a deterministic selection. It is stateless and safe for
// concurrent use.
//
// It returns a recommendation only. It does not reserve GPUs,
// bind Pods, or modify Kubernetes objects.
type DefaultEngine struct{}

var _ Engine = DefaultEngine{}

// NewEngine returns the Phase 1 decision engine.
func NewEngine() DefaultEngine {
	return DefaultEngine{}
}

// Select recommends one GPU for a single-GPU workload.
//
// An invalid request returns an error wrapping ErrInvalidRequest.
// When no candidate is eligible, Found is false, the selection fields
// keep their zero values, and the result is not an error.
//
// The highest score wins. Scores are compared exactly; equal scores
// are broken by NodeName, then GPUUUID, ascending. The same inputs
// produce the same Decision regardless of candidate order.
//
// Rejections lists only candidates removed by Filter. Eligible GPUs
// that lose on score are not rejections.
func (DefaultEngine) Select(
	candidates []Candidate,
	request Request,
	now time.Time,
) (Decision, error) {
	filtered, err := Filter(candidates, request, now)
	if err != nil {
		return Decision{}, err
	}

	decision := Decision{Rejections: filtered.Rejections}

	// Eligible is already sorted by NodeName, GPUUUID, so a strict
	// greater-than keeps the first candidate among equal scores.
	for _, c := range filtered.Eligible {
		score := scoreCandidate(c, request)
		if decision.Found && score <= decision.Score {
			continue
		}

		decision.Found = true
		decision.SelectedNode = c.Sample.NodeName
		decision.SelectedGPU = c.Sample.GPUUUID
		decision.Score = score
	}

	return decision, nil
}
