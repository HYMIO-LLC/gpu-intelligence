package scheduling

import "math"

// Phase 1 scoring weights. They sum to 1, so scores stay in [0, 100].
//
// This is an initial, explainable heuristic. It has not been validated
// against real GPU workloads and does not represent available compute
// or memory capacity.
const (
	UtilizationWeight         = 0.7
	TemperatureHeadroomWeight = 0.3
)

// Score rates one GPU for a single-GPU workload. Higher is better.
//
//	score    = 0.7 × (100 − utilizationPercent)
//	         + 0.3 × headroom
//	headroom = clamp(100 × (maxTemp − temp) / maxTemp, 0, 100)
//
// Inputs are expected to come from a candidate that passed Filter.
// Both terms are clamped to [0, 100], so the result is always finite
// and within [0, 100]. Non-finite inputs or a non-positive
// maxTemperatureCelsius score 0, the lowest possible value.
func Score(
	temperatureCelsius float64,
	utilizationPercent float64,
	maxTemperatureCelsius float64,
) float64 {
	if !isFinite(temperatureCelsius) ||
		!isFinite(utilizationPercent) ||
		!isFinite(maxTemperatureCelsius) ||
		maxTemperatureCelsius <= 0 {
		return 0
	}

	idle := clamp(100-utilizationPercent, 0, 100)
	headroom := clamp(
		100*(maxTemperatureCelsius-temperatureCelsius)/maxTemperatureCelsius,
		0,
		100,
	)

	return UtilizationWeight*idle + TemperatureHeadroomWeight*headroom
}

// scoreCandidate scores a candidate that has passed Filter.
func scoreCandidate(c Candidate, request Request) float64 {
	return Score(
		c.Sample.TemperatureCelsius.Value,
		c.Sample.UtilizationPercent.Value,
		request.MaxTemperatureCelsius,
	)
}

func clamp(v, lo, hi float64) float64 {
	return math.Min(math.Max(v, lo), hi)
}

func isFinite(v float64) bool {
	return !math.IsNaN(v) && !math.IsInf(v, 0)
}
