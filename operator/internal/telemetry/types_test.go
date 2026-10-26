package telemetry

import (
	"math"
	"testing"
	"time"
)

func TestSampleValidate(t *testing.T) {
	now := time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name    string
		sample  Sample
		wantErr bool
	}{
		{
			name: "valid GPU telemetry",
			sample: Sample{
				NodeName:           "gpu-node-1",
				GPUUUID:            "GPU-test-001",
				TemperatureCelsius: &Reading{Value: 68, ObservedAt: now},
				UtilizationPercent: &Reading{Value: 72, ObservedAt: now},
				MemoryUsedMiB:      &Reading{Value: 4096, ObservedAt: now},
			},
		},
		{
			name: "missing metrics are allowed",
			sample: Sample{
				NodeName: "gpu-node-1",
				GPUUUID:  "GPU-test-001",
			},
		},
		{
			name: "zero utilization is valid",
			sample: Sample{
				NodeName:           "gpu-node-1",
				GPUUUID:            "GPU-test-001",
				UtilizationPercent: &Reading{Value: 0, ObservedAt: now},
			},
		},
		{
			name: "utilization above 100 is invalid",
			sample: Sample{
				NodeName:           "gpu-node-1",
				GPUUUID:            "GPU-test-001",
				UtilizationPercent: &Reading{Value: 101, ObservedAt: now},
			},
			wantErr: true,
		},
		{
			name: "NaN is invalid",
			sample: Sample{
				NodeName:           "gpu-node-1",
				GPUUUID:            "GPU-test-001",
				TemperatureCelsius: &Reading{Value: math.NaN(), ObservedAt: now},
			},
			wantErr: true,
		},
		{
			name: "missing timestamp is invalid",
			sample: Sample{
				NodeName:      "gpu-node-1",
				GPUUUID:       "GPU-test-001",
				MemoryUsedMiB: &Reading{Value: 2048},
			},
			wantErr: true,
		},
		{
			name: "missing GPU UUID",
			sample: Sample{
				NodeName: "gpu-node-1",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.sample.Validate()
			if (err != nil) != tt.wantErr {
				t.Fatalf("Validate() error = %v, wantErr = %v", err, tt.wantErr)
			}
		})
	}
}
