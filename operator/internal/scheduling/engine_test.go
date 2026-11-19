package scheduling

import (
	"errors"
	"math"
	"slices"
	"testing"
	"time"

	"github.com/HYMIO-LLC/gpu-intelligence/operator/internal/telemetry"
)

func TestSelect(t *testing.T) {
	stale := withTemp(gpu(nodeB, "GPU-2", 30, 0), &telemetry.Reading{
		Value: 30, ObservedAt: testNow.Add(-time.Hour),
	})

	tests := []struct {
		name           string
		candidates     []Candidate
		wantFound      bool
		wantNode       string
		wantGPU        string
		wantScore      float64
		wantRejections []Rejection
	}{
		{
			name:       "single eligible GPU",
			candidates: []Candidate{gpu(nodeA, gpu1, 60, 50)},
			wantFound:  true,
			wantNode:   nodeA,
			wantGPU:    gpu1,
			wantScore:  0.7*50 + 0.3*25,
		},
		{
			name: "least utilized wins",
			candidates: []Candidate{
				gpu(nodeA, gpu1, 60, 80),
				gpu(nodeB, gpu1, 60, 10),
				gpu("node-c", gpu1, 60, 40),
			},
			wantFound: true,
			wantNode:  nodeB,
			wantGPU:   gpu1,
			wantScore: 0.7*90 + 0.3*25,
		},
		{
			name: "cooler wins at equal utilization",
			candidates: []Candidate{
				gpu(nodeA, gpu1, 70, 30),
				gpu(nodeB, gpu1, 40, 30),
			},
			wantFound: true,
			wantNode:  nodeB,
			wantGPU:   gpu1,
			wantScore: 0.7*70 + 0.3*50,
		},
		{
			name: "rejected GPU never wins even with best raw values",
			candidates: []Candidate{
				stale,
				gpu(nodeA, gpu1, 79, 89),
			},
			wantFound: true,
			wantNode:  nodeA,
			wantGPU:   gpu1,
			wantScore: Score(79, 89, 80),
			wantRejections: []Rejection{
				{NodeName: nodeB, GPUUUID: "GPU-2", Reason: ReasonStaleTelemetry},
			},
		},
		{
			name: "exact tie breaks by node name",
			candidates: []Candidate{
				gpu(nodeB, gpu1, 60, 50),
				gpu(nodeA, "GPU-9", 60, 50),
			},
			wantFound: true,
			wantNode:  nodeA,
			wantGPU:   "GPU-9",
			wantScore: 0.7*50 + 0.3*25,
		},
		{
			name: "exact tie on same node breaks by GPU UUID",
			candidates: []Candidate{
				gpu(nodeA, "GPU-2", 60, 50),
				gpu(nodeA, gpu1, 60, 50),
			},
			wantFound: true,
			wantNode:  nodeA,
			wantGPU:   gpu1,
			wantScore: 0.7*50 + 0.3*25,
		},
		{
			name: "near tie is decided by score, not name",
			candidates: []Candidate{
				gpu(nodeA, gpu1, 60, 50),
				gpu("node-z", gpu1, 60, 49.999999),
			},
			wantFound: true,
			wantNode:  "node-z",
			wantGPU:   gpu1,
			wantScore: Score(60, 49.999999, 80),
		},
		{
			name:       "no candidates",
			candidates: nil,
			wantFound:  false,
		},
		{
			name: "all candidates rejected",
			candidates: []Candidate{
				gpu(nodeA, gpu1, 95, 10),
				gpu(nodeB, gpu1, 60, 99),
				withTemp(gpu("node-c", gpu1, 60, 10), nil),
			},
			wantFound: false,
			wantRejections: []Rejection{
				{NodeName: nodeA, GPUUUID: gpu1, Reason: ReasonTemperatureExceeded},
				{NodeName: nodeB, GPUUUID: gpu1, Reason: ReasonUtilizationExceeded},
				{NodeName: "node-c", GPUUUID: gpu1, Reason: ReasonMissingTelemetry},
			},
		},
	}

	req := validRequest()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewEngine().Select(tt.candidates, req, testNow)
			if err != nil {
				t.Fatalf("Select() unexpected error: %v", err)
			}

			if got.Found != tt.wantFound ||
				got.SelectedNode != tt.wantNode ||
				got.SelectedGPU != tt.wantGPU ||
				math.Abs(got.Score-tt.wantScore) > 1e-9 {
				t.Fatalf("Select() = {Found:%v Node:%q GPU:%q Score:%v}, want {Found:%v Node:%q GPU:%q Score:%v}",
					got.Found, got.SelectedNode, got.SelectedGPU, got.Score,
					tt.wantFound, tt.wantNode, tt.wantGPU, tt.wantScore)
			}

			if !slices.Equal(got.Rejections, tt.wantRejections) {
				t.Fatalf("Rejections = %+v, want %+v", got.Rejections, tt.wantRejections)
			}
		})
	}
}

func TestSelectZeroScoreIsStillSelected(t *testing.T) {
	req := validRequest()
	req.MaxUtilizationPercent = 100

	got, err := NewEngine().Select(
		[]Candidate{gpu(nodeA, gpu1, 80, 100)}, req, testNow,
	)
	if err != nil {
		t.Fatalf("Select() unexpected error: %v", err)
	}
	if !got.Found || got.SelectedGPU != gpu1 || got.Score != 0 {
		t.Fatalf("Select() = %+v, want GPU-1 selected with score 0", got)
	}
}

func TestSelectInvalidRequest(t *testing.T) {
	req := validRequest()
	req.MaxTemperatureCelsius = math.NaN()

	got, err := NewEngine().Select(
		[]Candidate{gpu(nodeA, gpu1, 60, 50)}, req, testNow,
	)
	if !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("Select() error = %v, want ErrInvalidRequest", err)
	}
	if got.Found || got.SelectedNode != "" || got.SelectedGPU != "" ||
		got.Score != 0 || got.Rejections != nil {
		t.Fatalf("Select() = %+v, want zero Decision on error", got)
	}
}

func TestSelectNotFoundKeepsZeroSelection(t *testing.T) {
	got, err := NewEngine().Select(
		[]Candidate{gpu(nodeA, gpu1, 99, 99)}, validRequest(), testNow,
	)
	if err != nil {
		t.Fatalf("Select() unexpected error: %v", err)
	}
	if got.Found || got.SelectedNode != "" || got.SelectedGPU != "" || got.Score != 0 {
		t.Fatalf("Select() = %+v, want zero selection fields when not found", got)
	}
	if len(got.Rejections) != 1 {
		t.Fatalf("Rejections = %+v, want 1", got.Rejections)
	}
}

func TestSelectOrderIndependent(t *testing.T) {
	candidates := []Candidate{
		gpu(nodeB, gpu1, 60, 50), // ties with node-a/GPU-1
		gpu(nodeA, gpu1, 60, 50),
		gpu(nodeA, "GPU-2", 95, 0), // overheated
		gpu("node-c", gpu1, 70, 70),
		withUtil(gpu("node-c", "GPU-2", 60, 0), nil), // missing
	}

	want, err := NewEngine().Select(candidates, validRequest(), testNow)
	if err != nil {
		t.Fatalf("Select() unexpected error: %v", err)
	}
	if want.SelectedNode != nodeA || want.SelectedGPU != gpu1 {
		t.Fatalf("Select() picked %s/%s, want node-a/GPU-1", want.SelectedNode, want.SelectedGPU)
	}

	count := 0
	forEachPermutation(candidates, func(perm []Candidate) {
		count++
		got, err := NewEngine().Select(perm, validRequest(), testNow)
		if err != nil {
			t.Fatalf("Select() unexpected error: %v", err)
		}
		if got.Found != want.Found ||
			got.SelectedNode != want.SelectedNode ||
			got.SelectedGPU != want.SelectedGPU ||
			got.Score != want.Score ||
			!slices.Equal(got.Rejections, want.Rejections) {
			t.Fatalf("permutation %d: Select() = %+v, want %+v", count, got, want)
		}
	})

	if count != 120 {
		t.Fatalf("checked %d permutations, want 120", count)
	}
}

func TestSelectConcurrent(t *testing.T) {
	candidates := []Candidate{
		gpu(nodeA, gpu1, 60, 50),
		gpu(nodeB, gpu1, 40, 20),
		gpu("node-c", gpu1, 95, 10),
	}
	engine := NewEngine()

	done := make(chan Decision)
	for range 16 {
		go func() {
			d, _ := engine.Select(candidates, validRequest(), testNow)
			done <- d
		}()
	}
	for range 16 {
		if d := <-done; d.SelectedNode != nodeB {
			t.Errorf("concurrent Select() picked %q, want node-b", d.SelectedNode)
		}
	}
}

// forEachPermutation calls fn with every ordering of cs (Heap's algorithm).
func forEachPermutation(cs []Candidate, fn func([]Candidate)) {
	a := slices.Clone(cs)
	var generate func(k int)
	generate = func(k int) {
		if k <= 1 {
			fn(slices.Clone(a))
			return
		}
		for i := range k - 1 {
			generate(k - 1)
			if k%2 == 0 {
				a[i], a[k-1] = a[k-1], a[i]
			} else {
				a[0], a[k-1] = a[k-1], a[0]
			}
		}
		generate(k - 1)
	}
	generate(len(a))
}
