package scheduling

import (
	"errors"
	"math"
	"slices"
	"testing"
	"time"

	"github.com/HYMIO-LLC/gpu-intelligence/operator/internal/telemetry"
)

var testNow = time.Date(2026, 11, 17, 12, 0, 0, 0, time.UTC)

const (
	nodeA = "node-a"
	nodeB = "node-b"
	gpu1  = "GPU-1"
)

func validRequest() Request {
	return Request{
		MaxTemperatureCelsius: 80,
		MaxUtilizationPercent: 90,
		MaxTelemetryAge:       2 * time.Minute,
	}
}

// gpu builds a candidate whose readings were observed at testNow.
func gpu(node, uuid string, temp, util float64) Candidate {
	return Candidate{Sample: telemetry.Sample{
		NodeName:           node,
		GPUUUID:            uuid,
		TemperatureCelsius: &telemetry.Reading{Value: temp, ObservedAt: testNow},
		UtilizationPercent: &telemetry.Reading{Value: util, ObservedAt: testNow},
	}}
}

func withTemp(c Candidate, r *telemetry.Reading) Candidate {
	c.Sample.TemperatureCelsius = r
	return c
}

func withUtil(c Candidate, r *telemetry.Reading) Candidate {
	c.Sample.UtilizationPercent = r
	return c
}

func eligibleIDs(cs []Candidate) []string {
	ids := make([]string, 0, len(cs))
	for _, c := range cs {
		ids = append(ids, c.Sample.NodeName+"/"+c.Sample.GPUUUID)
	}
	return ids
}

func TestRequestValidate(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*Request)
		wantErr bool
	}{
		{name: "valid", mutate: func(*Request) {}},
		{name: "temperature at lower bound", mutate: func(r *Request) { r.MaxTemperatureCelsius = 1 }},
		{name: "temperature at upper bound", mutate: func(r *Request) { r.MaxTemperatureCelsius = 120 }},
		{name: "temperature below range", mutate: func(r *Request) { r.MaxTemperatureCelsius = 0.5 }, wantErr: true},
		{name: "temperature above range", mutate: func(r *Request) { r.MaxTemperatureCelsius = 120.1 }, wantErr: true},
		{name: "temperature NaN", mutate: func(r *Request) { r.MaxTemperatureCelsius = math.NaN() }, wantErr: true},
		{name: "temperature +Inf", mutate: func(r *Request) { r.MaxTemperatureCelsius = math.Inf(1) }, wantErr: true},
		{name: "utilization at upper bound", mutate: func(r *Request) { r.MaxUtilizationPercent = 100 }},
		{name: "utilization zero", mutate: func(r *Request) { r.MaxUtilizationPercent = 0 }, wantErr: true},
		{name: "utilization negative", mutate: func(r *Request) { r.MaxUtilizationPercent = -1 }, wantErr: true},
		{name: "utilization above 100", mutate: func(r *Request) { r.MaxUtilizationPercent = 100.5 }, wantErr: true},
		{name: "utilization NaN", mutate: func(r *Request) { r.MaxUtilizationPercent = math.NaN() }, wantErr: true},
		{name: "utilization -Inf", mutate: func(r *Request) { r.MaxUtilizationPercent = math.Inf(-1) }, wantErr: true},
		{name: "max age zero", mutate: func(r *Request) { r.MaxTelemetryAge = 0 }, wantErr: true},
		{name: "max age negative", mutate: func(r *Request) { r.MaxTelemetryAge = -time.Second }, wantErr: true},
		{name: "zero request", mutate: func(r *Request) { *r = Request{} }, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := validRequest()
			tt.mutate(&req)

			err := req.Validate()
			if (err != nil) != tt.wantErr {
				t.Fatalf("Validate() error = %v, wantErr = %v", err, tt.wantErr)
			}
			if err != nil && !errors.Is(err, ErrInvalidRequest) {
				t.Fatalf("Validate() error = %v, want wrapping ErrInvalidRequest", err)
			}
		})
	}
}

func TestFilterInvalidRequest(t *testing.T) {
	req := validRequest()
	req.MaxTelemetryAge = 0

	result, err := Filter([]Candidate{gpu(nodeA, gpu1, 60, 10)}, req, testNow)
	if !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("Filter() error = %v, want ErrInvalidRequest", err)
	}
	if len(result.Eligible) != 0 || len(result.Rejections) != 0 {
		t.Fatalf("Filter() result = %+v, want empty on error", result)
	}
}

func TestFilterSingleCandidate(t *testing.T) {
	base := gpu(nodeA, gpu1, 60, 50)
	req := validRequest()

	tests := []struct {
		name       string
		candidate  Candidate
		wantReason string // empty means eligible
	}{
		{name: "healthy", candidate: base},
		{name: "temperature equals limit", candidate: gpu(nodeA, gpu1, 80, 50)},
		{name: "utilization equals limit", candidate: gpu(nodeA, gpu1, 60, 90)},
		{name: "zero utilization", candidate: gpu(nodeA, gpu1, 60, 0)},
		{
			name: "age equals max age",
			candidate: withTemp(base, &telemetry.Reading{
				Value: 60, ObservedAt: testNow.Add(-2 * time.Minute),
			}),
		},
		{
			name: "memory reading absent is allowed",
			candidate: func() Candidate {
				c := base
				c.Sample.MemoryUsedMiB = nil
				return c
			}(),
		},
		{
			name:       "missing node name",
			candidate:  gpu("", gpu1, 60, 50),
			wantReason: ReasonInvalidIdentity,
		},
		{
			name:       "missing GPU UUID",
			candidate:  gpu(nodeA, "", 60, 50),
			wantReason: ReasonInvalidIdentity,
		},
		{
			name:       "missing temperature",
			candidate:  withTemp(base, nil),
			wantReason: ReasonMissingTelemetry,
		},
		{
			name:       "missing utilization",
			candidate:  withUtil(base, nil),
			wantReason: ReasonMissingTelemetry,
		},
		{
			name:       "temperature NaN",
			candidate:  withTemp(base, &telemetry.Reading{Value: math.NaN(), ObservedAt: testNow}),
			wantReason: ReasonInvalidTelemetry,
		},
		{
			name:       "temperature +Inf",
			candidate:  withTemp(base, &telemetry.Reading{Value: math.Inf(1), ObservedAt: testNow}),
			wantReason: ReasonInvalidTelemetry,
		},
		{
			name:       "utilization -Inf",
			candidate:  withUtil(base, &telemetry.Reading{Value: math.Inf(-1), ObservedAt: testNow}),
			wantReason: ReasonInvalidTelemetry,
		},
		{
			name:       "utilization above 100",
			candidate:  withUtil(base, &telemetry.Reading{Value: 101, ObservedAt: testNow}),
			wantReason: ReasonInvalidTelemetry,
		},
		{
			name:       "utilization negative",
			candidate:  withUtil(base, &telemetry.Reading{Value: -1, ObservedAt: testNow}),
			wantReason: ReasonInvalidTelemetry,
		},
		{
			name:       "temperature below absolute zero",
			candidate:  withTemp(base, &telemetry.Reading{Value: -300, ObservedAt: testNow}),
			wantReason: ReasonInvalidTelemetry,
		},
		{
			name:       "zero timestamp",
			candidate:  withTemp(base, &telemetry.Reading{Value: 60}),
			wantReason: ReasonInvalidTelemetry,
		},
		{
			name: "invalid optional memory reading",
			candidate: func() Candidate {
				c := base
				c.Sample.MemoryUsedMiB = &telemetry.Reading{Value: math.NaN(), ObservedAt: testNow}
				return c
			}(),
			wantReason: ReasonInvalidTelemetry,
		},
		{
			name: "temperature from the future",
			candidate: withTemp(base, &telemetry.Reading{
				Value: 60, ObservedAt: testNow.Add(time.Nanosecond),
			}),
			wantReason: ReasonFutureTelemetry,
		},
		{
			name: "utilization from the future",
			candidate: withUtil(base, &telemetry.Reading{
				Value: 50, ObservedAt: testNow.Add(time.Minute),
			}),
			wantReason: ReasonFutureTelemetry,
		},
		{
			name: "temperature stale",
			candidate: withTemp(base, &telemetry.Reading{
				Value: 60, ObservedAt: testNow.Add(-2*time.Minute - time.Nanosecond),
			}),
			wantReason: ReasonStaleTelemetry,
		},
		{
			name: "utilization stale",
			candidate: withUtil(base, &telemetry.Reading{
				Value: 50, ObservedAt: testNow.Add(-time.Hour),
			}),
			wantReason: ReasonStaleTelemetry,
		},
		{
			name: "stale but cool still rejected",
			candidate: withTemp(base, &telemetry.Reading{
				Value: 20, ObservedAt: testNow.Add(-10 * time.Minute),
			}),
			wantReason: ReasonStaleTelemetry,
		},
		{
			name:       "overheated",
			candidate:  gpu(nodeA, gpu1, 80.01, 50),
			wantReason: ReasonTemperatureExceeded,
		},
		{
			name:       "overloaded",
			candidate:  gpu(nodeA, gpu1, 60, 90.01),
			wantReason: ReasonUtilizationExceeded,
		},
		{
			name:       "overheated and overloaded reports temperature first",
			candidate:  gpu(nodeA, gpu1, 95, 99),
			wantReason: ReasonTemperatureExceeded,
		},
		{
			name:       "invalid identity reported before missing telemetry",
			candidate:  withTemp(gpu("", "", 60, 50), nil),
			wantReason: ReasonInvalidIdentity,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := Filter([]Candidate{tt.candidate}, req, testNow)
			if err != nil {
				t.Fatalf("Filter() unexpected error: %v", err)
			}

			if tt.wantReason == "" {
				if len(result.Eligible) != 1 || len(result.Rejections) != 0 {
					t.Fatalf("Filter() = %+v, want candidate eligible", result)
				}
				return
			}

			if len(result.Eligible) != 0 || len(result.Rejections) != 1 {
				t.Fatalf("Filter() = %+v, want one rejection", result)
			}
			got := result.Rejections[0]
			want := Rejection{
				NodeName: tt.candidate.Sample.NodeName,
				GPUUUID:  tt.candidate.Sample.GPUUUID,
				Reason:   tt.wantReason,
			}
			if got != want {
				t.Fatalf("Rejection = %+v, want %+v", got, want)
			}
		})
	}
}

func TestFilterEmptyInput(t *testing.T) {
	for _, candidates := range [][]Candidate{nil, {}} {
		result, err := Filter(candidates, validRequest(), testNow)
		if err != nil {
			t.Fatalf("Filter(%v) unexpected error: %v", candidates, err)
		}
		if len(result.Eligible) != 0 || len(result.Rejections) != 0 {
			t.Fatalf("Filter(%v) = %+v, want empty result", candidates, result)
		}
	}
}

func TestFilterDuplicateGPUIdentity(t *testing.T) {
	stale := withTemp(gpu(nodeA, gpu1, 60, 10), &telemetry.Reading{
		Value: 60, ObservedAt: testNow.Add(-time.Hour),
	})

	candidates := []Candidate{
		gpu(nodeA, gpu1, 60, 10),
		stale,                    // same identity, conflicting snapshot
		gpu(nodeB, gpu1, 60, 10), // same UUID on another node is distinct
		gpu("", "", 60, 10),
		gpu("", "", 60, 10), // empty identities are invalid, not duplicates
	}

	result, err := Filter(candidates, validRequest(), testNow)
	if err != nil {
		t.Fatalf("Filter() unexpected error: %v", err)
	}

	if got, want := eligibleIDs(result.Eligible), []string{"node-b/GPU-1"}; !slices.Equal(got, want) {
		t.Fatalf("Eligible = %v, want %v", got, want)
	}

	want := []Rejection{
		{NodeName: "", GPUUUID: "", Reason: ReasonInvalidIdentity},
		{NodeName: "", GPUUUID: "", Reason: ReasonInvalidIdentity},
		{NodeName: nodeA, GPUUUID: gpu1, Reason: ReasonDuplicateGPU},
		{NodeName: nodeA, GPUUUID: gpu1, Reason: ReasonDuplicateGPU},
	}
	if !slices.Equal(result.Rejections, want) {
		t.Fatalf("Rejections = %+v, want %+v", result.Rejections, want)
	}
}

func TestFilterMixedSetSortedAndOrderIndependent(t *testing.T) {
	candidates := []Candidate{
		gpu("node-c", "GPU-2", 60, 10),
		gpu(nodeA, "GPU-9", 95, 10), // overheated
		gpu(nodeB, gpu1, 60, 95),    // overloaded
		gpu(nodeA, "GPU-2", 70, 30),
		withUtil(gpu(nodeB, "GPU-0", 60, 10), nil), // missing
		gpu(nodeA, "GPU-10", 50, 50),
	}

	wantEligible := []string{"node-a/GPU-10", "node-a/GPU-2", "node-c/GPU-2"}
	wantRejections := []Rejection{
		{NodeName: nodeA, GPUUUID: "GPU-9", Reason: ReasonTemperatureExceeded},
		{NodeName: nodeB, GPUUUID: "GPU-0", Reason: ReasonMissingTelemetry},
		{NodeName: nodeB, GPUUUID: gpu1, Reason: ReasonUtilizationExceeded},
	}

	for _, perm := range [][]Candidate{
		candidates,
		reversed(candidates),
		rotated(candidates, 2),
		rotated(reversed(candidates), 3),
	} {
		snapshot := slices.Clone(perm)

		result, err := Filter(perm, validRequest(), testNow)
		if err != nil {
			t.Fatalf("Filter() unexpected error: %v", err)
		}
		if got := eligibleIDs(result.Eligible); !slices.Equal(got, wantEligible) {
			t.Fatalf("Eligible = %v, want %v", got, wantEligible)
		}
		if !slices.Equal(result.Rejections, wantRejections) {
			t.Fatalf("Rejections = %+v, want %+v", result.Rejections, wantRejections)
		}
		if !slices.EqualFunc(perm, snapshot, func(a, b Candidate) bool {
			return a.Sample.NodeName == b.Sample.NodeName && a.Sample.GPUUUID == b.Sample.GPUUUID
		}) {
			t.Fatalf("Filter() modified its input slice")
		}
	}
}

func reversed(cs []Candidate) []Candidate {
	out := slices.Clone(cs)
	slices.Reverse(out)
	return out
}

func rotated(cs []Candidate, n int) []Candidate {
	n %= len(cs)
	return append(slices.Clone(cs[n:]), cs[:n]...)
}
