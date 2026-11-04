package telemetry

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestPrometheusFetchNodeOverallTimeout(t *testing.T) {
	var requests atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			requests.Add(1)

			time.Sleep(80 * time.Millisecond)

			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{
				"status":"success",
				"data":{"resultType":"vector","result":[]}
			}`)
		},
	))
	defer server.Close()

	client := NewPrometheusClient(server.URL)

	// Each individual HTTP request has a generous timeout.
	// The complete FetchNode call has a smaller shared budget.
	client.HTTP.Timeout = 2 * time.Second
	client.FetchTimeout = 130 * time.Millisecond

	start := time.Now()

	_, err := client.FetchNode(
		context.Background(),
		"gpu-node-1",
	)

	elapsed := time.Since(start)

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf(
			"expected deadline exceeded, got %v",
			err,
		)
	}

	if elapsed > 500*time.Millisecond {
		t.Fatalf(
			"FetchNode took too long: %s",
			elapsed,
		)
	}

	if requests.Load() < 2 {
		t.Fatalf(
			"expected deadline across sequential requests, got %d",
			requests.Load(),
		)
	}
}

func TestPrometheusFetchNodeWithinBudget(t *testing.T) {
	var requests atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			requests.Add(1)

			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{
				"status":"success",
				"data":{"resultType":"vector","result":[]}
			}`)
		},
	))
	defer server.Close()

	client := NewPrometheusClient(server.URL)
	client.FetchTimeout = time.Second

	samples, err := client.FetchNode(
		context.Background(),
		"gpu-node-1",
	)

	if err != nil {
		t.Fatal(err)
	}

	if len(samples) != 0 {
		t.Fatalf(
			"expected zero samples, got %d",
			len(samples),
		)
	}

	if requests.Load() != 3 {
		t.Fatalf(
			"expected 3 metric queries, got %d",
			requests.Load(),
		)
	}
}
