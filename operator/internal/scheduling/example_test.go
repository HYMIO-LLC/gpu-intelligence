package scheduling_test

import (
	"fmt"
	"time"

	"github.com/HYMIO-LLC/gpu-intelligence/operator/internal/scheduling"
	"github.com/HYMIO-LLC/gpu-intelligence/operator/internal/telemetry"
)

// Reproducible demo scenarios for the Phase 1 engine.
// Run: go test ./internal/scheduling -run Example -v
func ExampleDefaultEngine_Select() {
	now := time.Date(2026, 11, 18, 12, 0, 0, 0, time.UTC)

	gpu := func(node, uuid string, temp, util float64, age time.Duration) scheduling.Candidate {
		at := now.Add(-age)
		return scheduling.Candidate{Sample: telemetry.Sample{
			NodeName:           node,
			GPUUUID:            uuid,
			TemperatureCelsius: &telemetry.Reading{Value: temp, ObservedAt: at},
			UtilizationPercent: &telemetry.Reading{Value: util, ObservedAt: at},
		}}
	}

	request := scheduling.Request{
		MaxTemperatureCelsius: 80,
		MaxUtilizationPercent: 90,
		MaxTelemetryAge:       2 * time.Minute,
	}

	scenarios := []struct {
		name       string
		candidates []scheduling.Candidate
	}{
		{"hot spot and cold spot", []scheduling.Candidate{
			gpu("gpu-node-1", "GPU-A", 78, 85, 10*time.Second),
			gpu("gpu-node-2", "GPU-B", 45, 30, 10*time.Second),
			gpu("gpu-node-3", "GPU-C", 92, 5, 10*time.Second),
		}},
		{"stale idle GPU is not trusted", []scheduling.Candidate{
			gpu("gpu-node-1", "GPU-A", 30, 0, 10*time.Minute),
			gpu("gpu-node-2", "GPU-B", 65, 60, 30*time.Second),
		}},
		{"tie broken by node name", []scheduling.Candidate{
			gpu("gpu-node-2", "GPU-B", 60, 50, 0),
			gpu("gpu-node-1", "GPU-A", 60, 50, 0),
		}},
		{"nothing eligible", []scheduling.Candidate{
			gpu("gpu-node-1", "GPU-A", 85, 20, 0),
			gpu("gpu-node-2", "GPU-B", 60, 95, 0),
		}},
	}

	engine := scheduling.NewEngine()
	for _, s := range scenarios {
		d, err := engine.Select(s.candidates, request, now)
		if err != nil {
			fmt.Println(s.name, "error:", err)
			continue
		}

		fmt.Printf("%s:\n", s.name)
		if d.Found {
			fmt.Printf("  selected %s/%s score=%.2f\n", d.SelectedNode, d.SelectedGPU, d.Score)
		} else {
			fmt.Println("  no eligible GPU")
		}
		for _, r := range d.Rejections {
			fmt.Printf("  rejected %s/%s: %s\n", r.NodeName, r.GPUUUID, r.Reason)
		}
	}

	// Output:
	// hot spot and cold spot:
	//   selected gpu-node-2/GPU-B score=62.12
	//   rejected gpu-node-3/GPU-C: TemperatureExceeded
	// stale idle GPU is not trusted:
	//   selected gpu-node-2/GPU-B score=33.62
	//   rejected gpu-node-1/GPU-A: StaleTelemetry
	// tie broken by node name:
	//   selected gpu-node-1/GPU-A score=42.50
	// nothing eligible:
	//   no eligible GPU
	//   rejected gpu-node-1/GPU-A: TemperatureExceeded
	//   rejected gpu-node-2/GPU-B: UtilizationExceeded
}
