package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
)

type gpuMetrics struct {
	Node        string
	UUID        string
	Temperature float64
	Utilization float64
	MemoryMiB   float64
}

func escapeLabel(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "\n", "\\n")
	return strings.ReplaceAll(s, "\"", "\\\"")
}

func metricsHandler(m gpuMetrics) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, "ok")
	})

	mux.HandleFunc("GET /metrics", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		labels := fmt.Sprintf(
			`node="%s",UUID="%s"`,
			escapeLabel(m.Node), escapeLabel(m.UUID),
		)

		fmt.Fprintln(w, "# HELP DCGM_FI_DEV_GPU_TEMP Mock GPU temperature in Celsius.")
		fmt.Fprintln(w, "# TYPE DCGM_FI_DEV_GPU_TEMP gauge")
		fmt.Fprintf(w, "DCGM_FI_DEV_GPU_TEMP{%s} %g\n", labels, m.Temperature)

		fmt.Fprintln(w, "# HELP DCGM_FI_DEV_GPU_UTIL Mock GPU utilization percentage.")
		fmt.Fprintln(w, "# TYPE DCGM_FI_DEV_GPU_UTIL gauge")
		fmt.Fprintf(w, "DCGM_FI_DEV_GPU_UTIL{%s} %g\n", labels, m.Utilization)

		fmt.Fprintln(w, "# HELP DCGM_FI_DEV_FB_USED Mock framebuffer memory used in MiB.")
		fmt.Fprintln(w, "# TYPE DCGM_FI_DEV_FB_USED gauge")
		fmt.Fprintf(w, "DCGM_FI_DEV_FB_USED{%s} %g\n", labels, m.MemoryMiB)
	})

	return mux
}

func main() {
	node := os.Getenv("MOCK_NODE_NAME")
	if node == "" {
		node = "hymio-dev-control-plane"
	}

	uuid := os.Getenv("MOCK_GPU_UUID")
	if uuid == "" {
		uuid = "GPU-MOCK-001"
	}

	addr := ":9400"
	log.Printf("Starting mock DCGM exporter on %s for node %s", addr, node)

	err := http.ListenAndServe(addr, metricsHandler(gpuMetrics{
		Node:        node,
		UUID:        uuid,
		Temperature: 68,
		Utilization: 72,
		MemoryMiB:   4096,
	}))
	log.Fatal(err)
}
