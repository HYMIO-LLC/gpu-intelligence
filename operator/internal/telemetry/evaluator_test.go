package telemetry

import (
	"testing"
	"time"
)

func TestEvaluateTemperatures(t *testing.T) {
	now := time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)

	makeSample := func(
		uuid string,
		temp float64,
		observedAt time.Time,
	) Sample {
		return Sample{
			NodeName: "gpu-node-1",
			GPUUUID:  uuid,
			TemperatureCelsius: &Reading{
				Value:      temp,
				ObservedAt: observedAt,
			},
		}
	}

	tests := []struct {
		name          string
		samples       []Sample
		wantHealth    HealthState
		wantAvailable bool
		wantOverheat  int
	}{
		{
			name: "healthy GPU",
			samples: []Sample{
				makeSample("GPU-001", 68, now),
			},
			wantHealth:    HealthHealthy,
			wantAvailable: true,
		},
		{
			name: "overheated GPU",
			samples: []Sample{
				makeSample("GPU-001", 90, now),
			},
			wantHealth:    HealthUnhealthy,
			wantAvailable: true,
			wantOverheat:  1,
		},
		{
			name: "temperature equals threshold",
			samples: []Sample{
				makeSample("GPU-001", 80, now),
			},
			wantHealth:    HealthHealthy,
			wantAvailable: true,
		},
		{
			name:       "no GPU samples",
			samples:    nil,
			wantHealth: HealthUnknown,
		},
		{
			name: "missing temperature",
			samples: []Sample{
				{
					NodeName: "gpu-node-1",
					GPUUUID:  "GPU-001",
				},
			},
			wantHealth: HealthUnknown,
		},
		{
			name: "stale telemetry",
			samples: []Sample{
				makeSample(
					"GPU-001",
					68,
					now.Add(-3*time.Minute),
				),
			},
			wantHealth: HealthUnknown,
		},
		{
			name: "future timestamp",
			samples: []Sample{
				makeSample(
					"GPU-001",
					68,
					now.Add(time.Minute),
				),
			},
			wantHealth: HealthUnknown,
		},
		{
			name: "multiple healthy GPUs",
			samples: []Sample{
				makeSample("GPU-001", 68, now),
				makeSample("GPU-002", 75, now),
			},
			wantHealth:    HealthHealthy,
			wantAvailable: true,
		},
		{
			name: "overheated plus missing",
			samples: []Sample{
				makeSample("GPU-001", 95, now),
				{
					NodeName: "gpu-node-1",
					GPUUUID:  "GPU-002",
				},
			},
			wantHealth:    HealthUnhealthy,
			wantAvailable: false,
			wantOverheat:  1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := EvaluateTemperatures(
				tt.samples,
				80,
				now,
				DefaultMaxAge,
			)

			if got.Health != tt.wantHealth {
				t.Errorf(
					"health = %s, want %s",
					got.Health,
					tt.wantHealth,
				)
			}

			if got.TelemetryAvailable != tt.wantAvailable {
				t.Errorf(
					"available = %v, want %v",
					got.TelemetryAvailable,
					tt.wantAvailable,
				)
			}

			if got.OverheatedGPUs != tt.wantOverheat {
				t.Errorf(
					"overheated = %d, want %d",
					got.OverheatedGPUs,
					tt.wantOverheat,
				)
			}
		})
	}
}

func TestEvaluateTemperaturesInvalidMaxAge(t *testing.T) {
	got := EvaluateTemperatures(
		nil, 80, time.Now(), 0,
	)

	if got.Health != HealthUnknown ||
		got.Reason != "InvalidConfiguration" {
		t.Fatalf("unexpected evaluation: %+v", got)
	}
}
