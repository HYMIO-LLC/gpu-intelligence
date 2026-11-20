package scheduling

import (
	"cmp"
	"fmt"
	"math"
	"math/rand/v2"
	"slices"
	"testing"
	"time"

	"github.com/HYMIO-LLC/gpu-intelligence/operator/internal/telemetry"
)

// Simulation tests run Filter and Select together on in-memory,
// multi-node clusters. They do not depend on kind, Prometheus or
// the number of real nodes.

const (
	simNode1 = "gpu-node-1"
	simNode2 = "gpu-node-2"
	simNode3 = "gpu-node-3"
	simNode4 = "gpu-node-4"

	simGPU0 = "GPU-0"
	simGPU2 = "GPU-2"
	simGPU3 = "GPU-3"
)

// simGPU is one entry in a simulated cluster snapshot together with
// the outcome the fixture author expects under validRequest() at
// testNow.
type simGPU struct {
	candidate  Candidate
	wantReason string  // "" means eligible
	wantScore  float64 // meaningful only when eligible
}

// simCandidate builds a candidate whose readings are age old at testNow.
// A negative age produces future-dated telemetry.
func simCandidate(
	node string,
	uuid string,
	temp float64,
	util float64,
	age time.Duration,
) Candidate {
	at := testNow.Add(-age)
	return Candidate{Sample: telemetry.Sample{
		NodeName:           node,
		GPUUUID:            uuid,
		TemperatureCelsius: &telemetry.Reading{Value: temp, ObservedAt: at},
		UtilizationPercent: &telemetry.Reading{Value: util, ObservedAt: at},
	}}
}

// simulatedCluster returns a four-node, sixteen-GPU snapshot with one
// duplicated GPU entry (17 candidates in total).
//
// With validRequest() (80 °C, 90 %, 2m) at testNow, seven GPUs are
// eligible and gpu-node-3/GPU-1 wins with 71 after a tie-break against
// gpu-node-4/GPU-0. The most attractive raw values (idle and cool) all
// belong to GPUs that must be rejected.
func simulatedCluster() []simGPU {
	const fresh = 15 * time.Second

	return []simGPU{
		// gpu-node-1: busy training node.
		{simCandidate(simNode1, simGPU0, 72, 88, fresh), "", 0.7*12 + 0.3*10},
		{simCandidate(simNode1, gpu1, 76, 90, fresh), "", 0.7*10 + 0.3*5}, // utilization at limit
		{simCandidate(simNode1, simGPU2, 83, 60, fresh), ReasonTemperatureExceeded, 0},
		{simCandidate(simNode1, simGPU3, 70, 97, fresh), ReasonUtilizationExceeded, 0},

		// gpu-node-2: mixed inference node with telemetry gaps.
		{simCandidate(simNode2, simGPU0, 55, 35, 30*time.Second), "", 0.7*65 + 0.3*31.25},
		{simCandidate(simNode2, gpu1, 48, 20, 30*time.Second), "", 0.7*80 + 0.3*40},
		{withTemp(simCandidate(simNode2, simGPU2, 30, 0, fresh), nil), ReasonMissingTelemetry, 0},
		{simCandidate(simNode2, simGPU3, 41, 15, 5*time.Minute), ReasonStaleTelemetry, 0},

		// gpu-node-3: mostly idle, but the idlest GPUs have bad data.
		{simCandidate(simNode3, simGPU0, 35, 0, 3*time.Minute), ReasonStaleTelemetry, 0},
		{simCandidate(simNode3, gpu1, 40, 20, 10*time.Second), "", 0.7*80 + 0.3*50},
		{simCandidate(simNode3, simGPU2, 30, 5, -30*time.Second), ReasonFutureTelemetry, 0},
		{withUtil(simCandidate(simNode3, simGPU3, 35, 0, fresh), &telemetry.Reading{
			Value: math.NaN(), ObservedAt: testNow,
		}), ReasonInvalidTelemetry, 0},

		// gpu-node-4: boundary values and identity problems.
		{simCandidate(simNode4, simGPU0, 40, 20, 2*time.Minute), "", 0.7*80 + 0.3*50}, // age at limit, ties node-3
		{simCandidate(simNode4, gpu1, 80, 20, fresh), "", 0.7 * 80},                   // temperature at limit
		{simCandidate(simNode4, simGPU2, 25, 0, fresh), ReasonDuplicateGPU, 0},
		{simCandidate(simNode4, simGPU2, 25, 0, time.Minute), ReasonDuplicateGPU, 0},
		{simCandidate(simNode4, "", 30, 0, fresh), ReasonInvalidIdentity, 0},
	}
}

func simCandidates(gpus []simGPU) []Candidate {
	cs := make([]Candidate, 0, len(gpus))
	for _, g := range gpus {
		cs = append(cs, g.candidate)
	}
	return cs
}

func compareRejections(a, b Rejection) int {
	return cmp.Or(
		cmp.Compare(a.NodeName, b.NodeName),
		cmp.Compare(a.GPUUUID, b.GPUUUID),
		cmp.Compare(a.Reason, b.Reason),
	)
}

func TestSimulationClusterDecision(t *testing.T) {
	gpus := simulatedCluster()
	candidates := simCandidates(gpus)
	req := validRequest()

	var wantEligible []string
	var wantRejections []Rejection
	for _, g := range gpus {
		s := g.candidate.Sample
		if g.wantReason == "" {
			wantEligible = append(wantEligible, s.NodeName+"/"+s.GPUUUID)
			if got := scoreCandidate(g.candidate, req); math.Abs(got-g.wantScore) > 1e-9 {
				t.Errorf("score(%s/%s) = %v, want %v", s.NodeName, s.GPUUUID, got, g.wantScore)
			}
			continue
		}
		wantRejections = append(wantRejections, Rejection{
			NodeName: s.NodeName, GPUUUID: s.GPUUUID, Reason: g.wantReason,
		})
	}
	slices.Sort(wantEligible)
	slices.SortFunc(wantRejections, compareRejections)

	filtered, err := Filter(candidates, req, testNow)
	if err != nil {
		t.Fatalf("Filter() unexpected error: %v", err)
	}
	if got := eligibleIDs(filtered.Eligible); !slices.Equal(got, wantEligible) {
		t.Fatalf("Eligible = %v, want %v", got, wantEligible)
	}

	got, err := NewEngine().Select(candidates, req, testNow)
	if err != nil {
		t.Fatalf("Select() unexpected error: %v", err)
	}
	if !got.Found || got.SelectedNode != simNode3 || got.SelectedGPU != gpu1 ||
		math.Abs(got.Score-71) > 1e-9 {
		t.Fatalf("Select() = {Found:%v %s/%s Score:%v}, want gpu-node-3/GPU-1 with 71",
			got.Found, got.SelectedNode, got.SelectedGPU, got.Score)
	}
	if !slices.Equal(got.Rejections, wantRejections) {
		t.Fatalf("Rejections =\n%+v\nwant\n%+v", got.Rejections, wantRejections)
	}
	checkDecisionInvariants(t, candidates, req, testNow, got)
}

func TestSimulationScenarios(t *testing.T) {
	type scenario struct {
		name         string
		mutate       func([]Candidate) []Candidate
		request      func(*Request)
		evaluateAt   time.Time
		wantNode     string
		wantGPU      string
		wantScore    float64
		wantRejected int
	}

	tests := []scenario{
		{
			name:         "baseline",
			wantNode:     simNode3,
			wantGPU:      gpu1,
			wantScore:    71,
			wantRejected: 10,
		},
		{
			name: "winner overheats, tie partner takes over",
			mutate: func(cs []Candidate) []Candidate {
				return replaceGPU(cs, simCandidate(simNode3, gpu1, 85, 20, 10*time.Second))
			},
			wantNode:     simNode4,
			wantGPU:      simGPU0,
			wantScore:    71,
			wantRejected: 11,
		},
		{
			name:         "one second later the tie partner is stale",
			evaluateAt:   testNow.Add(time.Second),
			wantNode:     simNode3,
			wantGPU:      gpu1,
			wantScore:    71,
			wantRejected: 11,
		},
		{
			name: "duplicate resolved, idle GPU becomes eligible",
			mutate: func(cs []Candidate) []Candidate {
				i := slices.IndexFunc(cs, func(c Candidate) bool {
					return c.Sample.NodeName == simNode4 && c.Sample.GPUUUID == simGPU2
				})
				return slices.Delete(slices.Clone(cs), i, i+1)
			},
			wantNode:     simNode4,
			wantGPU:      simGPU2,
			wantScore:    0.7*100 + 0.3*100*55/80,
			wantRejected: 8,
		},
		{
			name:         "stricter temperature limit rescales headroom",
			request:      func(r *Request) { r.MaxTemperatureCelsius = 50 },
			wantNode:     simNode3,
			wantGPU:      gpu1,
			wantScore:    0.7*80 + 0.3*20,
			wantRejected: 14, // node-1 GPU-0/1, node-2 GPU-0, node-4 GPU-1 now overheat
		},
		{
			name:         "relaxed utilization limit admits the busy GPU",
			request:      func(r *Request) { r.MaxUtilizationPercent = 100 },
			wantNode:     simNode3,
			wantGPU:      gpu1,
			wantScore:    71,
			wantRejected: 9,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			candidates := simCandidates(simulatedCluster())
			if tt.mutate != nil {
				candidates = tt.mutate(candidates)
			}
			req := validRequest()
			if tt.request != nil {
				tt.request(&req)
			}
			now := cmp.Or(tt.evaluateAt, testNow)

			got, err := NewEngine().Select(candidates, req, now)
			if err != nil {
				t.Fatalf("Select() unexpected error: %v", err)
			}
			if !got.Found || got.SelectedNode != tt.wantNode || got.SelectedGPU != tt.wantGPU ||
				math.Abs(got.Score-tt.wantScore) > 1e-9 {
				t.Fatalf("Select() = {Found:%v %s/%s Score:%v}, want %s/%s with %v",
					got.Found, got.SelectedNode, got.SelectedGPU, got.Score,
					tt.wantNode, tt.wantGPU, tt.wantScore)
			}
			if len(got.Rejections) != tt.wantRejected {
				t.Fatalf("len(Rejections) = %d, want %d: %+v",
					len(got.Rejections), tt.wantRejected, got.Rejections)
			}
			checkDecisionInvariants(t, candidates, req, now, got)
		})
	}
}

func TestSimulationNoEligibleGPU(t *testing.T) {
	cluster := simCandidates(simulatedCluster())

	tests := []struct {
		name       string
		candidates []Candidate
		evaluateAt time.Time
		wantReason string // expected for every GPU with a valid identity and no duplicate; "" skips
	}{
		{
			name:       "empty cluster",
			candidates: nil,
		},
		{
			name: "heat wave",
			candidates: mapCandidates(cluster, func(c Candidate) Candidate {
				if c.Sample.TemperatureCelsius == nil {
					return c
				}
				r := *c.Sample.TemperatureCelsius
				r.Value += 50
				return withTemp(c, &r)
			}),
		},
		{
			name: "telemetry outage",
			candidates: mapCandidates(cluster, func(c Candidate) Candidate {
				return withUtil(withTemp(c, nil), nil)
			}),
		},
		{
			name:       "Prometheus lagging ten minutes",
			candidates: cluster,
			evaluateAt: testNow.Add(10 * time.Minute),
		},
		{
			name: "only GPU is duplicated",
			candidates: []Candidate{
				simCandidate(simNode1, simGPU0, 40, 10, 0),
				simCandidate(simNode1, simGPU0, 40, 10, 0),
			},
			wantReason: ReasonDuplicateGPU,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			now := cmp.Or(tt.evaluateAt, testNow)
			got, err := NewEngine().Select(tt.candidates, validRequest(), now)
			if err != nil {
				t.Fatalf("Select() unexpected error: %v", err)
			}
			if got.Found || got.SelectedNode != "" || got.SelectedGPU != "" || got.Score != 0 {
				t.Fatalf("Select() = %+v, want Found=false with zero selection", got)
			}
			if len(got.Rejections) != len(tt.candidates) {
				t.Fatalf("len(Rejections) = %d, want every candidate (%d) rejected",
					len(got.Rejections), len(tt.candidates))
			}
			if tt.wantReason != "" {
				for _, r := range got.Rejections {
					if r.Reason != tt.wantReason {
						t.Fatalf("rejection %+v, want reason %s", r, tt.wantReason)
					}
				}
			}
			checkDecisionInvariants(t, tt.candidates, validRequest(), now, got)
		})
	}
}

func TestSimulationInputOrderIndependent(t *testing.T) {
	cluster := simCandidates(simulatedCluster())
	clusters := []struct {
		name       string
		candidates []Candidate
	}{
		{"baseline", cluster},
		{"winner overheats", replaceGPU(cluster,
			simCandidate(simNode3, gpu1, 85, 20, 10*time.Second))},
		{"exact three-way tie", append(slices.Clone(cluster),
			simCandidate("gpu-node-0", simGPU3, 40, 20, 0))},
	}

	rng := rand.New(rand.NewPCG(2026, 1119))
	engine := NewEngine()

	for _, tc := range clusters {
		t.Run(tc.name, func(t *testing.T) {
			candidates := tc.candidates
			want, err := engine.Select(candidates, validRequest(), testNow)
			if err != nil {
				t.Fatalf("Select() unexpected error: %v", err)
			}

			orders := make([][]Candidate, 0, 501)
			orders = append(orders, reversed(candidates))
			for range 500 {
				perm := slices.Clone(candidates)
				rng.Shuffle(len(perm), func(i, j int) { perm[i], perm[j] = perm[j], perm[i] })
				orders = append(orders, perm)
			}

			for i, input := range orders {
				before := slices.Clone(input)
				got, err := engine.Select(input, validRequest(), testNow)
				if err != nil {
					t.Fatalf("order %d: Select() unexpected error: %v", i, err)
				}
				if !slices.Equal(input, before) {
					t.Fatalf("order %d: Select() modified its input slice", i)
				}
				if got.Found != want.Found ||
					got.SelectedNode != want.SelectedNode ||
					got.SelectedGPU != want.SelectedGPU ||
					got.Score != want.Score ||
					!slices.Equal(got.Rejections, want.Rejections) {
					t.Fatalf("order %d: Select() = %+v, want %+v", i, got, want)
				}
			}
		})
	}

	// The three-way tie is decided by NodeName: gpu-node-0 sorts first.
	got, _ := engine.Select(clusters[2].candidates, validRequest(), testNow)
	if got.SelectedNode != "gpu-node-0" || got.SelectedGPU != simGPU3 {
		t.Fatalf("three-way tie picked %s/%s, want gpu-node-0/GPU-3", got.SelectedNode, got.SelectedGPU)
	}
}

func TestSimulationDuplicateGPUIdentity(t *testing.T) {
	best := simCandidate("gpu-node-9", simGPU0, 20, 0, 0) // new identity; would score 92.5

	tests := []struct {
		name       string
		extra      []Candidate
		wantNode   string
		wantGPU    string
		wantDupes  int
		wantReason map[string]string // node/uuid -> reason for extra GPUs
	}{
		{
			name:      "identical duplicate of the best GPU",
			extra:     []Candidate{best, best},
			wantNode:  simNode3,
			wantGPU:   gpu1,
			wantDupes: 2,
		},
		{
			name: "conflicting duplicate: fresh and stale snapshots",
			extra: []Candidate{
				best,
				simCandidate("gpu-node-9", simGPU0, 20, 0, time.Hour),
			},
			wantNode:  simNode3,
			wantGPU:   gpu1,
			wantDupes: 2,
		},
		{
			name: "conflicting duplicate: idle and overloaded snapshots",
			extra: []Candidate{
				best,
				simCandidate("gpu-node-9", simGPU0, 20, 100, 0),
			},
			wantNode:  simNode3,
			wantGPU:   gpu1,
			wantDupes: 2,
		},
		{
			name:      "triplicate",
			extra:     []Candidate{best, best, best},
			wantNode:  simNode3,
			wantGPU:   gpu1,
			wantDupes: 3,
		},
		{
			name: "same UUID on different nodes is two GPUs",
			extra: []Candidate{
				simCandidate("gpu-node-5", "GPU-X", 20, 0, 0),
				simCandidate("gpu-node-6", "GPU-X", 20, 0, 0),
			},
			wantNode:  "gpu-node-5",
			wantGPU:   "GPU-X",
			wantDupes: 0,
		},
		{
			name: "repeated empty identity is invalid, not duplicate",
			extra: []Candidate{
				simCandidate("", "", 20, 0, 0),
				simCandidate("", "", 20, 0, 0),
			},
			wantNode:  simNode3,
			wantGPU:   gpu1,
			wantDupes: 0,
			wantReason: map[string]string{
				"/": ReasonInvalidIdentity,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Drop the fixture's own duplicate pair so wantDupes counts
			// only what the case adds.
			candidates := slices.DeleteFunc(simCandidates(simulatedCluster()), func(c Candidate) bool {
				return c.Sample.NodeName == simNode4 && c.Sample.GPUUUID == simGPU2
			})
			candidates = append(candidates, tt.extra...)

			got, err := NewEngine().Select(candidates, validRequest(), testNow)
			if err != nil {
				t.Fatalf("Select() unexpected error: %v", err)
			}
			if !got.Found || got.SelectedNode != tt.wantNode || got.SelectedGPU != tt.wantGPU {
				t.Fatalf("Select() picked %s/%s (Found=%v), want %s/%s",
					got.SelectedNode, got.SelectedGPU, got.Found, tt.wantNode, tt.wantGPU)
			}

			dupes := 0
			for _, r := range got.Rejections {
				if r.Reason == ReasonDuplicateGPU {
					dupes++
				}
				if want, ok := tt.wantReason[r.NodeName+"/"+r.GPUUUID]; ok && r.Reason != want {
					t.Fatalf("rejection %+v, want reason %s", r, want)
				}
			}
			if dupes != tt.wantDupes {
				t.Fatalf("DuplicateGPU rejections = %d, want %d: %+v", dupes, tt.wantDupes, got.Rejections)
			}
			checkDecisionInvariants(t, candidates, validRequest(), testNow, got)
		})
	}
}

// TestSimulationAgainstReferenceModel generates random clusters whose
// expected outcome is known by construction, and compares Select
// with an independent, brute-force reference selection.
func TestSimulationAgainstReferenceModel(t *testing.T) {
	rng := rand.New(rand.NewPCG(2026, 1119))
	engine := NewEngine()

	for i := range 500 {
		req := Request{
			MaxTemperatureCelsius: []float64{60, 80, 90}[rng.IntN(3)],
			MaxUtilizationPercent: []float64{50, 90}[rng.IntN(2)],
			MaxTelemetryAge:       []time.Duration{30 * time.Second, 2 * time.Minute}[rng.IntN(2)],
		}
		gpus := randomCluster(rng, req)
		candidates := simCandidates(gpus)

		got, err := engine.Select(candidates, req, testNow)
		if err != nil {
			t.Fatalf("cluster %d: Select() unexpected error: %v", i, err)
		}

		want := referenceSelect(gpus, req)
		if got.Found != want.Found ||
			got.SelectedNode != want.SelectedNode ||
			got.SelectedGPU != want.SelectedGPU ||
			got.Score != want.Score ||
			!slices.Equal(got.Rejections, want.Rejections) {
			t.Fatalf("cluster %d (%d GPUs, %+v):\ngot  %+v\nwant %+v",
				i, len(candidates), req, got, want)
		}
		checkDecisionInvariants(t, candidates, req, testNow, got)
	}
}

// referenceSelect derives the expected Decision from the generator's
// ground truth without calling Filter or Select.
func referenceSelect(gpus []simGPU, req Request) Decision {
	var want Decision
	for _, g := range gpus {
		s := g.candidate.Sample
		if g.wantReason != "" {
			want.Rejections = append(want.Rejections, Rejection{
				NodeName: s.NodeName, GPUUUID: s.GPUUUID, Reason: g.wantReason,
			})
			continue
		}

		score := Score(s.TemperatureCelsius.Value, s.UtilizationPercent.Value, req.MaxTemperatureCelsius)
		better := !want.Found ||
			score > want.Score ||
			score == want.Score && (s.NodeName < want.SelectedNode ||
				s.NodeName == want.SelectedNode && s.GPUUUID < want.SelectedGPU)
		if better {
			want.Found = true
			want.SelectedNode = s.NodeName
			want.SelectedGPU = s.GPUUUID
			want.Score = score
		}
	}
	slices.SortFunc(want.Rejections, compareRejections)
	return want
}

type simKind int

const (
	simHealthy simKind = iota
	simHot
	simOverloaded
	simMissing
	simStale
	simFuture
	simInvalid
	simNoIdentity
	simKindCount
)

// randomCluster builds 1–8 nodes with 1–8 GPUs each. Values sit on a
// coarse grid so that exact score ties are common. About one GPU in
// ten is reported twice with an independent snapshot.
func randomCluster(rng *rand.Rand, req Request) []simGPU {
	var gpus []simGPU
	for n := range 1 + rng.IntN(8) {
		node := fmt.Sprintf("sim-node-%02d", n)
		for g := range 1 + rng.IntN(8) {
			gpus = append(gpus, randomGPU(rng, node, fmt.Sprintf("GPU-%d", g), req))
		}
	}

	for _, g := range slices.Clone(gpus) {
		s := g.candidate.Sample
		if g.wantReason != ReasonInvalidIdentity && rng.IntN(10) == 0 {
			gpus = append(gpus, randomGPU(rng, s.NodeName, s.GPUUUID, req))
		}
	}

	type key struct{ node, gpu string }
	counts := map[key]int{}
	for _, g := range gpus {
		counts[key{g.candidate.Sample.NodeName, g.candidate.Sample.GPUUUID}]++
	}
	for i, g := range gpus {
		s := g.candidate.Sample
		if g.wantReason != ReasonInvalidIdentity && counts[key{s.NodeName, s.GPUUUID}] > 1 {
			gpus[i].wantReason = ReasonDuplicateGPU
		}
	}

	return gpus
}

// randomGPU returns a GPU in a randomly chosen state and the reason
// Filter must report for it ("" when eligible).
func randomGPU(rng *rand.Rand, node, uuid string, req Request) simGPU {
	maxAgeSeconds := int(req.MaxTelemetryAge / time.Second)

	temp := float64(5 * rng.IntN(int(req.MaxTemperatureCelsius)/5+1))
	util := float64(10 * rng.IntN(int(req.MaxUtilizationPercent)/10+1))
	age := time.Duration(rng.IntN(maxAgeSeconds+1)) * time.Second

	kind := simKind(rng.IntN(int(simKindCount)))
	switch kind {
	case simHealthy:
		return simGPU{candidate: simCandidate(node, uuid, temp, util, age)}

	case simHot:
		temp = req.MaxTemperatureCelsius + float64(1+rng.IntN(30))
		util = float64(10 * rng.IntN(11))
		return simGPU{simCandidate(node, uuid, temp, util, age), ReasonTemperatureExceeded, 0}

	case simOverloaded:
		util = req.MaxUtilizationPercent + float64(1+rng.IntN(int(100-req.MaxUtilizationPercent)))
		return simGPU{simCandidate(node, uuid, temp, util, age), ReasonUtilizationExceeded, 0}

	case simMissing:
		c := simCandidate(node, uuid, temp, util, age)
		if rng.IntN(2) == 0 {
			c = withTemp(c, nil)
		} else {
			c = withUtil(c, nil)
		}
		return simGPU{c, ReasonMissingTelemetry, 0}

	case simStale:
		age = req.MaxTelemetryAge + time.Duration(1+rng.IntN(600))*time.Second
		return simGPU{simCandidate(node, uuid, temp, util, age), ReasonStaleTelemetry, 0}

	case simFuture:
		age = -time.Duration(1+rng.IntN(60)) * time.Second
		return simGPU{simCandidate(node, uuid, temp, util, age), ReasonFutureTelemetry, 0}

	case simInvalid:
		c := simCandidate(node, uuid, temp, util, age)
		switch rng.IntN(5) {
		case 0:
			c = withTemp(c, &telemetry.Reading{Value: math.NaN(), ObservedAt: testNow})
		case 1:
			c = withUtil(c, &telemetry.Reading{Value: math.Inf(1), ObservedAt: testNow})
		case 2:
			c = withUtil(c, &telemetry.Reading{Value: 150, ObservedAt: testNow})
		case 3:
			c = withTemp(c, &telemetry.Reading{Value: -300, ObservedAt: testNow})
		default:
			c = withTemp(c, &telemetry.Reading{Value: temp}) // zero ObservedAt
		}
		return simGPU{c, ReasonInvalidTelemetry, 0}

	default: // simNoIdentity
		return simGPU{simCandidate(node, "", temp, util, age), ReasonInvalidIdentity, 0}
	}
}

// checkDecisionInvariants verifies properties that must hold for any
// Decision, independent of the specific scenario:
//
//   - Rejections equal Filter's rejections, and every candidate is
//     either eligible or rejected exactly once.
//   - Found is true exactly when at least one candidate is eligible.
//   - Found=false keeps the selection fields at their zero values.
//   - The selected GPU is eligible, never rejected, its Score matches
//     scoreCandidate, and no eligible GPU beats it on score or on the
//     NodeName/GPUUUID tie-break.
func checkDecisionInvariants(
	t *testing.T,
	candidates []Candidate,
	req Request,
	now time.Time,
	d Decision,
) {
	t.Helper()

	filtered, err := Filter(candidates, req, now)
	if err != nil {
		t.Fatalf("Filter() unexpected error: %v", err)
	}
	if !slices.Equal(d.Rejections, filtered.Rejections) {
		t.Fatalf("Decision.Rejections differ from Filter rejections")
	}
	if len(filtered.Eligible)+len(filtered.Rejections) != len(candidates) {
		t.Fatalf("eligible %d + rejected %d != candidates %d",
			len(filtered.Eligible), len(filtered.Rejections), len(candidates))
	}
	if d.Found != (len(filtered.Eligible) > 0) {
		t.Fatalf("Found = %v with %d eligible candidates", d.Found, len(filtered.Eligible))
	}

	if !d.Found {
		if d.SelectedNode != "" || d.SelectedGPU != "" || d.Score != 0 {
			t.Fatalf("Found=false but selection is %+v", d)
		}
		return
	}

	selected := false
	for _, c := range filtered.Eligible {
		s := c.Sample
		score := scoreCandidate(c, req)
		if s.NodeName == d.SelectedNode && s.GPUUUID == d.SelectedGPU {
			selected = true
			if score != d.Score {
				t.Fatalf("Decision.Score = %v, selected GPU scores %v", d.Score, score)
			}
			continue
		}
		if score > d.Score ||
			score == d.Score && compareCandidates(c, Candidate{Sample: telemetry.Sample{
				NodeName: d.SelectedNode, GPUUUID: d.SelectedGPU,
			}}) < 0 {
			t.Fatalf("%s/%s (score %v) should have beaten %s/%s (score %v)",
				s.NodeName, s.GPUUUID, score, d.SelectedNode, d.SelectedGPU, d.Score)
		}
	}
	if !selected {
		t.Fatalf("selected %s/%s is not an eligible candidate", d.SelectedNode, d.SelectedGPU)
	}
	for _, r := range d.Rejections {
		if r.NodeName == d.SelectedNode && r.GPUUUID == d.SelectedGPU {
			t.Fatalf("selected %s/%s is also rejected: %+v", d.SelectedNode, d.SelectedGPU, r)
		}
	}
}

// replaceGPU returns a copy of cs where entries with the same identity
// as c are replaced by c.
func replaceGPU(cs []Candidate, c Candidate) []Candidate {
	return mapCandidates(cs, func(old Candidate) Candidate {
		if old.Sample.NodeName == c.Sample.NodeName && old.Sample.GPUUUID == c.Sample.GPUUUID {
			return c
		}
		return old
	})
}

func mapCandidates(cs []Candidate, fn func(Candidate) Candidate) []Candidate {
	out := make([]Candidate, 0, len(cs))
	for _, c := range cs {
		out = append(out, fn(c))
	}
	return out
}
