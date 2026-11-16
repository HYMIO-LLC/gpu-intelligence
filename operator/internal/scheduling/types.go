package scheduling

import (
	"time"

	"github.com/HYMIO-LLC/gpu-intelligence/operator/internal/telemetry"
)

// Request describes the constraints for a single-GPU workload.
type Request struct {
	MaxTemperatureCelsius float64
	MaxUtilizationPercent float64
	MaxTelemetryAge       time.Duration
}

// Candidate represents one GPU being considered for placement.
type Candidate struct {
	Sample telemetry.Sample
}

// Rejection describes why a candidate cannot be selected.
type Rejection struct {
	NodeName string
	GPUUUID  string
	Reason   string
}

// Decision describes the result of evaluating GPU candidates.
// No Kubernetes resources are modified.
type Decision struct {
	// Found indicates whether an eligible GPU was selected.
	Found bool

	// Selection fields are meaningful only when Found is true.
	SelectedNode string
	SelectedGPU  string
	Score        float64

	// Rejections records why other candidates were excluded.
	Rejections []Rejection
}

// Engine evaluates GPUs and returns a scheduling recommendation.
type Engine interface {
	Select(
		candidates []Candidate,
		request Request,
		now time.Time,
	) (Decision, error)
}
