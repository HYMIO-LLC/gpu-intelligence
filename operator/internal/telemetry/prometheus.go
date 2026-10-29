package telemetry

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type PrometheusClient struct {
	BaseURL string
	HTTP    *http.Client
}

var _ Client = (*PrometheusClient)(nil)

func NewPrometheusClient(baseURL string) *PrometheusClient {
	return &PrometheusClient{
		BaseURL: strings.TrimRight(baseURL, "/"),
		HTTP: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
}

type promResponse struct {
	Status    string `json:"status"`
	Error     string `json:"error"`
	ErrorType string `json:"errorType"`
	Data      struct {
		ResultType string `json:"resultType"`
		Result     []struct {
			Metric map[string]string `json:"metric"`
			Value  []json.RawMessage `json:"value"`
		} `json:"result"`
	} `json:"data"`
}

func (c *PrometheusClient) query(
	ctx context.Context,
	metricName string,
	nodeName string,
) (map[string]Reading, error) {

	if c.BaseURL == "" {
		return nil, fmt.Errorf("Prometheus URL is required")
	}

	endpoint, err := url.Parse(c.BaseURL + "/api/v1/query")
	if err != nil {
		return nil, fmt.Errorf("invalid Prometheus URL: %w", err)
	}

	// Query a specific Node without interpolating raw input into PromQL.
	// Select by node labels after receiving the vector.
	params := endpoint.Query()
	params.Set("query", metricName)
	endpoint.RawQuery = params.Encode()

	req, err := http.NewRequestWithContext(
		ctx, http.MethodGet, endpoint.String(), nil,
	)
	if err != nil {
		return nil, err
	}

	client := c.HTTP
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Second}
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Prometheus request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf(
			"Prometheus returned HTTP %d", resp.StatusCode,
		)
	}

	var result promResponse
	decoder := json.NewDecoder(io.LimitReader(resp.Body, 4<<20))
	if err := decoder.Decode(&result); err != nil {
		return nil, fmt.Errorf("invalid Prometheus JSON: %w", err)
	}

	if result.Status != "success" {
		return nil, fmt.Errorf(
			"Prometheus query error (%s): %s",
			result.ErrorType, result.Error,
		)
	}

	if result.Data.ResultType != "vector" {
		return nil, fmt.Errorf(
			"unexpected result type: %s", result.Data.ResultType,
		)
	}

	readings := make(map[string]Reading)

	for _, item := range result.Data.Result {
		if item.Metric["node"] != nodeName {
			continue
		}

		uuid := item.Metric["UUID"]
		if uuid == "" {
			return nil, fmt.Errorf("missing GPU UUID")
		}

		if _, exists := readings[uuid]; exists {
			return nil, fmt.Errorf("duplicate GPU UUID: %s", uuid)
		}

		if len(item.Value) != 2 {
			return nil, fmt.Errorf("invalid sample for GPU %s", uuid)
		}

		var timestamp float64
		var valueString string

		if err := json.Unmarshal(item.Value[0], &timestamp); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(item.Value[1], &valueString); err != nil {
			return nil, err
		}

		value, err := strconv.ParseFloat(valueString, 64)
		if err != nil || math.IsNaN(value) || math.IsInf(value, 0) {
			return nil, fmt.Errorf(
				"invalid metric value for GPU %s: %s",
				uuid, valueString,
			)
		}

		if math.IsNaN(timestamp) || math.IsInf(timestamp, 0) ||
			timestamp <= 0 || timestamp > float64(math.MaxInt64)/1e9 {
			return nil, fmt.Errorf("invalid timestamp for GPU %s", uuid)
		}

		seconds, fraction := math.Modf(timestamp)

		observedAt := time.Unix(
			int64(seconds),
			int64(math.Round(fraction*1e9)),
		).UTC()

		readings[uuid] = Reading{
			Value:      value,
			ObservedAt: observedAt,
		}
	}

	return readings, nil
}

func (c *PrometheusClient) FetchNode(
	ctx context.Context,
	nodeName string,
) ([]Sample, error) {

	if nodeName == "" {
		return nil, fmt.Errorf("node name is required")
	}

	metrics := []struct {
		name string
		set  func(*Sample, *Reading)
	}{
		{
			"DCGM_FI_DEV_GPU_TEMP",
			func(s *Sample, r *Reading) { s.TemperatureCelsius = r },
		},
		{
			"DCGM_FI_DEV_GPU_UTIL",
			func(s *Sample, r *Reading) { s.UtilizationPercent = r },
		},
		{
			"DCGM_FI_DEV_FB_USED",
			func(s *Sample, r *Reading) { s.MemoryUsedMiB = r },
		},
	}

	samples := make(map[string]*Sample)

	for _, metric := range metrics {
		readings, err := c.query(ctx, metric.name, nodeName)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", metric.name, err)
		}

		for uuid, reading := range readings {
			sample := samples[uuid]
			if sample == nil {
				sample = &Sample{
					NodeName: nodeName,
					GPUUUID:  uuid,
				}
				samples[uuid] = sample
			}

			r := reading
			metric.set(sample, &r)
		}
	}

	result := make([]Sample, 0, len(samples))

	for _, sample := range samples {
		if err := sample.Validate(); err != nil {
			return nil, err
		}
		result = append(result, *sample)
	}

	// Deterministic ordering simplifies testing and downstream consumers.
	for i := 0; i < len(result); i++ {
		for j := i + 1; j < len(result); j++ {
			if result[j].GPUUUID < result[i].GPUUUID {
				result[i], result[j] = result[j], result[i]
			}
		}
	}

	return result, nil
}
