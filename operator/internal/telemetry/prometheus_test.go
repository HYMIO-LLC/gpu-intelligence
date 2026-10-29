package telemetry

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPrometheusClientFetchNode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			metric := r.URL.Query().Get("query")

			values := map[string]string{
				"DCGM_FI_DEV_GPU_TEMP": "68",
				"DCGM_FI_DEV_GPU_UTIL": "72",
				"DCGM_FI_DEV_FB_USED":  "4096",
			}

			value, ok := values[metric]
			if !ok {
				http.Error(w, "unknown metric", http.StatusBadRequest)
				return
			}

			w.Header().Set("Content-Type", "application/json")

			fmt.Fprintf(w, `{
                "status": "success",
                "data": {
                    "resultType": "vector",
                    "result": [{
                        "metric": {
                            "node": "gpu-node-1",
                            "UUID": "GPU-test-001"
                        },
                        "value": [1793159780.125, "%s"]
                    }]
                }
            }`, value)
		},
	))
	defer server.Close()

	client := NewPrometheusClient(server.URL)

	samples, err := client.FetchNode(context.Background(), "gpu-node-1")
	if err != nil {
		t.Fatal(err)
	}

	if len(samples) != 1 {
		t.Fatalf("got %d samples, want 1", len(samples))
	}

	s := samples[0]

	if s.GPUUUID != "GPU-test-001" ||
		s.TemperatureCelsius == nil ||
		s.UtilizationPercent == nil ||
		s.MemoryUsedMiB == nil {
		t.Fatalf("unexpected sample: %+v", s)
	}

	if s.TemperatureCelsius.Value != 68 ||
		s.UtilizationPercent.Value != 72 ||
		s.MemoryUsedMiB.Value != 4096 {
		t.Fatalf("incorrect metric values: %+v", s)
	}

	if s.TemperatureCelsius.ObservedAt.IsZero() {
		t.Fatal("missing observation timestamp")
	}
}

func TestPrometheusClientHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "unavailable", http.StatusServiceUnavailable)
		},
	))
	defer server.Close()

	client := NewPrometheusClient(server.URL)

	_, err := client.FetchNode(context.Background(), "gpu-node-1")

	if err == nil {
		t.Fatal("expected HTTP error")
	}
}

func TestPrometheusClientEmptyVector(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, `{
                "status":"success",
                "data":{"resultType":"vector","result":[]}
            }`)
		},
	))
	defer server.Close()

	client := NewPrometheusClient(server.URL)

	samples, err := client.FetchNode(context.Background(), "gpu-node-1")
	if err != nil {
		t.Fatal(err)
	}

	if len(samples) != 0 {
		t.Fatalf("expected no GPU samples, got %d", len(samples))
	}
}
