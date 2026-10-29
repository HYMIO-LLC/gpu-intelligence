package telemetry

import (
	"fmt"
	"time"
)

type HealthState string

const (
	HealthHealthy   HealthState = "Healthy"
	HealthUnhealthy HealthState = "Unhealthy"
	HealthUnknown   HealthState = "Unknown"
)

const DefaultMaxAge = 2 * time.Minute

type Evaluation struct {
	Health             HealthState
	TelemetryAvailable bool
	TotalGPUs          int
	ValidGPUs          int
	OverheatedGPUs     int
	Reason             string
	Message            string
}

// EvaluateTemperatures evaluates GPU temperature samples.
// A fresh temperature above maxTemperatureCelsius is unhealthy.
// Missing or stale readings are never treated as healthy.
func EvaluateTemperatures(
	samples []Sample,
	maxTemperatureCelsius float64,
	now time.Time,
	maxAge time.Duration,
) Evaluation {
	result := Evaluation{
		Health:    HealthUnknown,
		TotalGPUs: len(samples),
	}

	if maxAge <= 0 {
		result.Reason = "InvalidConfiguration"
		result.Message = "Telemetry freshness window must be positive"
		return result
	}

	if len(samples) == 0 {
		result.Reason = "NoTelemetry"
		result.Message = "No GPU temperature samples available"
		return result
	}

	for _, sample := range samples {
		reading := sample.TemperatureCelsius

		if reading == nil {
			continue
		}

		if err := sample.Validate(); err != nil {
			continue
		}

		age := now.Sub(reading.ObservedAt)

		// Reject future timestamps and stale readings.
		if age < 0 || age > maxAge {
			continue
		}

		result.ValidGPUs++

		if reading.Value > maxTemperatureCelsius {
			result.OverheatedGPUs++
		}
	}

	result.TelemetryAvailable =
		result.ValidGPUs == result.TotalGPUs

	if result.OverheatedGPUs > 0 {
		result.Health = HealthUnhealthy
		result.Reason = "TemperatureExceeded"
		result.Message = fmt.Sprintf(
			"%d GPU(s) exceed temperature threshold; %d/%d have valid readings",
			result.OverheatedGPUs,
			result.ValidGPUs,
			result.TotalGPUs,
		)
		return result
	}

	if !result.TelemetryAvailable {
		result.Health = HealthUnknown
		result.Reason = "IncompleteTelemetry"
		result.Message = fmt.Sprintf(
			"Only %d/%d GPU(s) have fresh valid temperature readings",
			result.ValidGPUs,
			result.TotalGPUs,
		)
		return result
	}

	result.Health = HealthHealthy
	result.Reason = "TemperatureWithinLimit"
	result.Message = fmt.Sprintf(
		"All %d GPU(s) are within the temperature threshold",
		result.ValidGPUs,
	)
	return result
}
