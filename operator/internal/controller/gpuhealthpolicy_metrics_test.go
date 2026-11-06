package controller

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestPolicyMetricsValues(t *testing.T) {
	cases := []struct {
		name      string
		healthy   float64
		available float64
	}{
		{"healthy", 1, 1},
		{"overheated", 0, 1},
		{"telemetry unavailable", -1, 0},
		{"unknown", -1, -1},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			registry := prometheus.NewRegistry()
			m := NewPolicyMetrics(registry)

			m.Set("default", "gpu-test", tc.healthy, tc.available, 2)

			if got := testutil.ToFloat64(
				m.healthy.WithLabelValues("default", "gpu-test"),
			); got != tc.healthy {
				t.Fatalf("healthy: got %v, want %v", got, tc.healthy)
			}

			if got := testutil.ToFloat64(
				m.telemetryAvailable.WithLabelValues("default", "gpu-test"),
			); got != tc.available {
				t.Fatalf("available: got %v, want %v", got, tc.available)
			}

			if got := testutil.ToFloat64(
				m.matchedNodes.WithLabelValues("default", "gpu-test"),
			); got != 2 {
				t.Fatalf("matched nodes: got %v, want 2", got)
			}
		})
	}
}

func TestPolicyMetricsDelete(t *testing.T) {
	registry := prometheus.NewRegistry()
	m := NewPolicyMetrics(registry)

	m.Set("default", "deleted-policy", 1, 1, 1)
	m.Set("other", "retained-policy", 0, 1, 2)

	m.Delete("default", "deleted-policy")

	families, err := registry.Gather()
	if err != nil {
		t.Fatal(err)
	}

	for _, family := range families {
		if len(family.GetMetric()) != 1 {
			t.Fatalf(
				"%s: expected 1 remaining series, got %d",
				family.GetName(), len(family.GetMetric()),
			)
		}

		for _, metric := range family.GetMetric() {
			for _, label := range metric.GetLabel() {
				if label.GetName() == "policy" &&
					label.GetValue() == "deleted-policy" {
					t.Fatal("deleted policy metric remains")
				}
			}
		}
	}
}

func TestPolicyMetricsTransition(t *testing.T) {
	registry := prometheus.NewRegistry()
	m := NewPolicyMetrics(registry)

	m.Set("default", "gpu-test", 1, 1, 1)
	m.Set("default", "gpu-test", 0, 1, 1)
	m.Set("default", "gpu-test", -1, 0, 1)
	m.Set("default", "gpu-test", 1, 1, 1)

	if got := testutil.ToFloat64(
		m.healthy.WithLabelValues("default", "gpu-test"),
	); got != 1 {
		t.Fatalf("expected recovered healthy value, got %v", got)
	}
}
