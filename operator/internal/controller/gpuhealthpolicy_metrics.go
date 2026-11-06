package controller

import (
	"github.com/prometheus/client_golang/prometheus"
)

// PolicyMetrics exposes the latest observed GPUHealthPolicy state.
//
// Label cardinality is bounded by the number of GPUHealthPolicy
// objects: only namespace and policy name are used.
type PolicyMetrics struct {
	healthy            *prometheus.GaugeVec
	telemetryAvailable *prometheus.GaugeVec
	matchedNodes       *prometheus.GaugeVec
}

// NewPolicyMetrics registers GPUHealthPolicy collectors.
// Pass controller-runtime's metrics.Registry in production,
// and an isolated registry in unit tests.
func NewPolicyMetrics(reg prometheus.Registerer) *PolicyMetrics {
	labels := []string{"namespace", "policy"}

	m := &PolicyMetrics{
		healthy: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "hymio_gpu_policy_healthy",
				Help: "GPU health: 1 healthy, 0 unhealthy, -1 unknown.",
			},
			labels,
		),
		telemetryAvailable: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "hymio_gpu_policy_telemetry_available",
				Help: "Telemetry availability: 1 available, 0 unavailable, -1 unknown.",
			},
			labels,
		),
		matchedNodes: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "hymio_gpu_policy_matched_nodes",
				Help: "Number of Kubernetes nodes matching the GPU health policy.",
			},
			labels,
		),
	}

	reg.MustRegister(
		m.healthy,
		m.telemetryAvailable,
		m.matchedNodes,
	)

	return m
}

// Set records a successfully observed policy status.
func (m *PolicyMetrics) Set(
	namespace, policy string,
	healthy, available float64,
	nodes int32,
) {
	if m == nil {
		return
	}

	m.healthy.WithLabelValues(namespace, policy).Set(healthy)
	m.telemetryAvailable.WithLabelValues(namespace, policy).Set(available)
	m.matchedNodes.WithLabelValues(namespace, policy).Set(float64(nodes))
}

// Delete removes all metric series belonging to a deleted policy.
func (m *PolicyMetrics) Delete(namespace, policy string) {
	if m == nil {
		return
	}

	m.healthy.DeleteLabelValues(namespace, policy)
	m.telemetryAvailable.DeleteLabelValues(namespace, policy)
	m.matchedNodes.DeleteLabelValues(namespace, policy)
}
