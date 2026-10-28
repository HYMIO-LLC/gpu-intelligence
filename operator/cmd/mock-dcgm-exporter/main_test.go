package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMetricsEndpoint(t *testing.T) {
	handler := metricsHandler(gpuMetrics{
		Node: "gpu-node-1", UUID: "GPU-test-001",
		Temperature: 68, Utilization: 72, MemoryMiB: 4096,
	})

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	body := rec.Body.String()
	required := []string{
		`DCGM_FI_DEV_GPU_TEMP{node="gpu-node-1",UUID="GPU-test-001"} 68`,
		`DCGM_FI_DEV_GPU_UTIL{node="gpu-node-1",UUID="GPU-test-001"} 72`,
		`DCGM_FI_DEV_FB_USED{node="gpu-node-1",UUID="GPU-test-001"} 4096`,
	}
	for _, expected := range required {
		if !strings.Contains(body, expected) {
			t.Errorf("missing metric: %s", expected)
		}
	}
}

func TestHealthEndpoint(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	metricsHandler(gpuMetrics{}).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("health status = %d", rec.Code)
	}
}

func TestMetricLabelEscaping(t *testing.T) {
	actual := escapeLabel("gpu\"node\\test\n")
	expected := `gpu\"node\\test\n`
	if actual != expected {
		t.Fatalf("escaped label = %q, want %q", actual, expected)
	}
}
