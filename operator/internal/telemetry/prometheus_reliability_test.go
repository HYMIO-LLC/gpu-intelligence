package telemetry

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestPrometheusClientMultipleGPUs(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			values := map[string][]string{
				"DCGM_FI_DEV_GPU_TEMP": {"68", "75"},
				"DCGM_FI_DEV_GPU_UTIL": {"72", "90"},
				"DCGM_FI_DEV_FB_USED":  {"4096", "8192"},
			}

			metric := r.URL.Query().Get("query")
			result := []any{}

			for i, uuid := range []string{"GPU-002", "GPU-001"} {
				result = append(result, map[string]any{
					"metric": map[string]string{
						"node": "gpu-node-1",
						"UUID": uuid,
					},
					"value": []any{1793159780.125, values[metric][i]},
				})
			}

			// Another Node should not appear in FetchNode results.
			result = append(result, map[string]any{
				"metric": map[string]string{
					"node": "gpu-node-2",
					"UUID": "GPU-OTHER",
				},
				"value": []any{1793159780.125, "50"},
			})

			_ = json.NewEncoder(w).Encode(map[string]any{
				"status": "success",
				"data": map[string]any{
					"resultType": "vector",
					"result":     result,
				},
			})
		},
	))
	defer server.Close()

	client := NewPrometheusClient(server.URL)
	samples, err := client.FetchNode(
		context.Background(), "gpu-node-1",
	)
	if err != nil {
		t.Fatal(err)
	}

	if len(samples) != 2 {
		t.Fatalf("expected 2 GPUs, got %d", len(samples))
	}

	// Results must be ordered deterministically by GPU UUID.
	if samples[0].GPUUUID != "GPU-001" ||
		samples[1].GPUUUID != "GPU-002" {
		t.Fatalf("unexpected GPU ordering: %+v", samples)
	}

	if samples[0].TemperatureCelsius.Value != 75 ||
		samples[1].TemperatureCelsius.Value != 68 {
		t.Fatal("incorrect GPU metric mapping")
	}
}

func TestPrometheusClientMissingMetric(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			metric := r.URL.Query().Get("query")
			result := []any{}

			if metric == "DCGM_FI_DEV_GPU_TEMP" {
				result = append(result, map[string]any{
					"metric": map[string]string{
						"node": "gpu-node-1",
						"UUID": "GPU-001",
					},
					"value": []any{1793159780.125, "68"},
				})
			}

			_ = json.NewEncoder(w).Encode(map[string]any{
				"status": "success",
				"data": map[string]any{
					"resultType": "vector",
					"result":     result,
				},
			})
		},
	))
	defer server.Close()

	samples, err := NewPrometheusClient(server.URL).
		FetchNode(context.Background(), "gpu-node-1")
	if err != nil {
		t.Fatal(err)
	}

	if len(samples) != 1 ||
		samples[0].TemperatureCelsius == nil {
		t.Fatal("expected temperature reading")
	}

	if samples[0].UtilizationPercent != nil ||
		samples[0].MemoryUsedMiB != nil {
		t.Fatal("missing metrics must remain nil")
	}
}

func TestPrometheusClientMalformedJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("{invalid-json"))
		},
	))
	defer server.Close()

	_, err := NewPrometheusClient(server.URL).
		FetchNode(context.Background(), "gpu-node-1")

	if err == nil {
		t.Fatal("expected JSON parsing error")
	}
}

func TestPrometheusClientInvalidValue(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status": "success",
				"data": map[string]any{
					"resultType": "vector",
					"result": []any{
						map[string]any{
							"metric": map[string]string{
								"node": "gpu-node-1",
								"UUID": "GPU-001",
							},
							"value": []any{1793159780.125, "NaN"},
						},
					},
				},
			})
		},
	))
	defer server.Close()

	_, err := NewPrometheusClient(server.URL).
		FetchNode(context.Background(), "gpu-node-1")

	if err == nil {
		t.Fatal("expected invalid metric value error")
	}
}

func TestPrometheusClientCancelledContext(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			<-r.Context().Done()
		},
	))
	defer server.Close()

	ctx, cancel := context.WithTimeout(
		context.Background(), 50*time.Millisecond,
	)
	defer cancel()

	_, err := NewPrometheusClient(server.URL).
		FetchNode(ctx, "gpu-node-1")

	if err == nil {
		t.Fatal("expected context timeout error")
	}
}
