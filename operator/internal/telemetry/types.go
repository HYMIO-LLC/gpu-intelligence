package telemetry

import (
	"fmt"
	"math"
	"time"
)

// Reading represents a single observed GPU metric.
// A nil *Reading means that the metric is unavailable.
type Reading struct {
	Value      float64
	ObservedAt time.Time
}

// Sample contains telemetry for one GPU on one Kubernetes Node.
type Sample struct {
	NodeName           string
	GPUUUID            string
	TemperatureCelsius *Reading
	UtilizationPercent *Reading
	MemoryUsedMiB      *Reading
}

func (s Sample) Validate() error {
	if s.NodeName == "" {
		return fmt.Errorf("node name is required")
	}
	if s.GPUUUID == "" {
		return fmt.Errorf("GPU UUID is required")
	}

	metrics := []struct {
		name    string
		reading *Reading
		min     float64
		max     float64
	}{
		{"temperature", s.TemperatureCelsius, -273.15, math.Inf(1)},
		{"utilization", s.UtilizationPercent, 0, 100},
		{"memory used", s.MemoryUsedMiB, 0, math.Inf(1)},
	}

	for _, m := range metrics {
		if m.reading == nil {
			continue
		}
		if m.reading.ObservedAt.IsZero() {
			return fmt.Errorf("%s timestamp is required", m.name)
		}
		if math.IsNaN(m.reading.Value) ||
			math.IsInf(m.reading.Value, 0) ||
			m.reading.Value < m.min ||
			m.reading.Value > m.max {
			return fmt.Errorf("invalid %s value", m.name)
		}
	}
	return nil
}
