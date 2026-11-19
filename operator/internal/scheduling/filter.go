package scheduling

import (
	"cmp"
	"errors"
	"fmt"
	"math"
	"slices"
	"time"

	"github.com/HYMIO-LLC/gpu-intelligence/operator/internal/telemetry"
)

// Accepted temperature limit range in degrees Celsius.
// It matches the GPUHealthPolicy MaxTemperatureCelsius schema.
const (
	MinTemperatureLimitCelsius = 1
	MaxTemperatureLimitCelsius = 120
)

// Rejection reasons. Each rejected candidate receives exactly one
// reason: the first failed check in the order documented on Filter.
const (
	ReasonInvalidIdentity     = "InvalidIdentity"
	ReasonDuplicateGPU        = "DuplicateGPU"
	ReasonMissingTelemetry    = "MissingTelemetry"
	ReasonInvalidTelemetry    = "InvalidTelemetry"
	ReasonFutureTelemetry     = "FutureTelemetry"
	ReasonStaleTelemetry      = "StaleTelemetry"
	ReasonTemperatureExceeded = "TemperatureExceeded"
	ReasonUtilizationExceeded = "UtilizationExceeded"
)

// ErrInvalidRequest is wrapped by every Request validation error.
var ErrInvalidRequest = errors.New("invalid scheduling request")

// Validate reports whether the request limits are usable.
//
// MaxTemperatureCelsius must be finite and within [1, 120].
// MaxUtilizationPercent must be finite and within (0, 100].
// MaxTelemetryAge must be positive.
func (r Request) Validate() error {
	if math.IsNaN(r.MaxTemperatureCelsius) ||
		math.IsInf(r.MaxTemperatureCelsius, 0) ||
		r.MaxTemperatureCelsius < MinTemperatureLimitCelsius ||
		r.MaxTemperatureCelsius > MaxTemperatureLimitCelsius {
		return fmt.Errorf(
			"%w: MaxTemperatureCelsius must be within [%d, %d], got %v",
			ErrInvalidRequest,
			MinTemperatureLimitCelsius,
			MaxTemperatureLimitCelsius,
			r.MaxTemperatureCelsius,
		)
	}

	if math.IsNaN(r.MaxUtilizationPercent) ||
		math.IsInf(r.MaxUtilizationPercent, 0) ||
		r.MaxUtilizationPercent <= 0 ||
		r.MaxUtilizationPercent > 100 {
		return fmt.Errorf(
			"%w: MaxUtilizationPercent must be within (0, 100], got %v",
			ErrInvalidRequest,
			r.MaxUtilizationPercent,
		)
	}

	if r.MaxTelemetryAge <= 0 {
		return fmt.Errorf(
			"%w: MaxTelemetryAge must be positive, got %v",
			ErrInvalidRequest,
			r.MaxTelemetryAge,
		)
	}

	return nil
}

// FilterResult splits candidates into eligible and rejected sets.
// Both slices are sorted by NodeName, then GPUUUID.
type FilterResult struct {
	Eligible   []Candidate
	Rejections []Rejection
}

// Filter removes candidates that cannot safely receive the workload.
//
// It fails closed: missing, invalid, stale or future-dated telemetry
// is rejected, never treated as healthy or idle. Checks run in order:
//
//  1. Identity: NodeName and GPUUUID are non-empty.
//  2. Uniqueness: every copy of a duplicated (NodeName, GPUUUID)
//     is rejected, because conflicting snapshots cannot be trusted.
//  3. Required readings: temperature and utilization are present.
//  4. Validity: telemetry.Sample.Validate passes (finite values,
//     physical ranges, non-zero timestamps).
//  5. Freshness: each required reading has 0 <= now-ObservedAt
//     <= MaxTelemetryAge.
//  6. Limits: temperature <= MaxTemperatureCelsius and utilization
//     <= MaxUtilizationPercent. Values equal to a limit pass.
//
// MemoryUsedMiB is not required and is never interpreted as free
// capacity. An invalid request returns an error; an empty candidate
// list returns an empty result. The input slice is not modified.
func Filter(
	candidates []Candidate,
	request Request,
	now time.Time,
) (FilterResult, error) {
	if err := request.Validate(); err != nil {
		return FilterResult{}, err
	}

	type gpuKey struct{ node, gpu string }
	counts := make(map[gpuKey]int, len(candidates))
	for _, c := range candidates {
		counts[gpuKey{c.Sample.NodeName, c.Sample.GPUUUID}]++
	}

	var result FilterResult
	for _, c := range candidates {
		duplicate := counts[gpuKey{c.Sample.NodeName, c.Sample.GPUUUID}] > 1
		reason := rejectReason(c.Sample, duplicate, request, now)

		if reason != "" {
			result.Rejections = append(result.Rejections, Rejection{
				NodeName: c.Sample.NodeName,
				GPUUUID:  c.Sample.GPUUUID,
				Reason:   reason,
			})
			continue
		}
		result.Eligible = append(result.Eligible, c)
	}

	slices.SortStableFunc(result.Eligible, compareCandidates)
	slices.SortStableFunc(result.Rejections, func(a, b Rejection) int {
		return cmp.Or(
			cmp.Compare(a.NodeName, b.NodeName),
			cmp.Compare(a.GPUUUID, b.GPUUUID),
			cmp.Compare(a.Reason, b.Reason),
		)
	})

	return result, nil
}

// rejectReason returns the first failed check, or "" when eligible.
func rejectReason(
	s telemetry.Sample,
	duplicate bool,
	request Request,
	now time.Time,
) string {
	if s.NodeName == "" || s.GPUUUID == "" {
		return ReasonInvalidIdentity
	}

	if duplicate {
		return ReasonDuplicateGPU
	}

	if s.TemperatureCelsius == nil || s.UtilizationPercent == nil {
		return ReasonMissingTelemetry
	}

	if err := s.Validate(); err != nil {
		return ReasonInvalidTelemetry
	}

	for _, r := range []*telemetry.Reading{
		s.TemperatureCelsius,
		s.UtilizationPercent,
	} {
		age := now.Sub(r.ObservedAt)
		if age < 0 {
			return ReasonFutureTelemetry
		}
		if age > request.MaxTelemetryAge {
			return ReasonStaleTelemetry
		}
	}

	if s.TemperatureCelsius.Value > request.MaxTemperatureCelsius {
		return ReasonTemperatureExceeded
	}
	if s.UtilizationPercent.Value > request.MaxUtilizationPercent {
		return ReasonUtilizationExceeded
	}

	return ""
}

// compareCandidates orders candidates by NodeName, then GPUUUID.
func compareCandidates(a, b Candidate) int {
	return cmp.Or(
		cmp.Compare(a.Sample.NodeName, b.Sample.NodeName),
		cmp.Compare(a.Sample.GPUUUID, b.Sample.GPUUUID),
	)
}
