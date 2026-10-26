package telemetry

import "context"

// Client retrieves GPU telemetry without exposing the metric source.
// A Prometheus implementation will be added in Week 3 Day 3.
type Client interface {
	FetchNode(ctx context.Context, nodeName string) ([]Sample, error)
}
