package telemetry

import (
	"math"
	"testing"
	"time"
)

func TestSampleValidateEdgeCases(t *testing.T) {
	now := time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name    string
		sample  Sample
		wantErr bool
	}{
		{
			name:    "missing node name",
			sample:  Sample{GPUUUID: "GPU-test-001"},
			wantErr: true,
		},
		{
			name: "infinite temperature",
			sample: Sample{
				NodeName: "gpu-node-1",
				GPUUUID:  "GPU-test-001",
				TemperatureCelsius: &Reading{
					Value: math.Inf(1), ObservedAt: now,
				},
			},
			wantErr: true,
		},
		{
			name: "negative memory usage",
			sample: Sample{
				NodeName: "gpu-node-1",
				GPUUUID:  "GPU-test-001",
				MemoryUsedMiB: &Reading{
					Value: -1, ObservedAt: now,
				},
			},
			wantErr: true,
		},
		{
			name: "negative utilization",
			sample: Sample{
				NodeName: "gpu-node-1",
				GPUUUID:  "GPU-test-001",
				UtilizationPercent: &Reading{
					Value: -1, ObservedAt: now,
				},
			},
			wantErr: true,
		},
		{
			name: "100 percent utilization",
			sample: Sample{
				NodeName: "gpu-node-1",
				GPUUUID:  "GPU-test-001",
				UtilizationPercent: &Reading{
					Value: 100, ObservedAt: now,
				},
			},
		},
		{
			name: "absolute zero temperature",
			sample: Sample{
				NodeName: "gpu-node-1",
				GPUUUID:  "GPU-test-001",
				TemperatureCelsius: &Reading{
					Value: -273.15, ObservedAt: now,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.sample.Validate()
			if (err != nil) != tt.wantErr {
				t.Fatalf("Validate() = %v, wantErr=%v", err, tt.wantErr)
			}
		})
	}
}
