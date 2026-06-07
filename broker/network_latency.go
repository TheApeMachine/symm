package broker

import (
	"bytes"
	"encoding/json"
	"math"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/theapemachine/errnie"
)

/*
EffectiveNetworkLatency reads runs/network_latency.json and returns the p95 sample.
*/
func EffectiveNetworkLatency() time.Duration {
	return EffectiveNetworkLatencyFromFile("runs/network_latency.json")
}

/*
EffectiveNetworkLatencyFromFile reads latency samples from path and returns the p95.
Samples may be JSON (array or {"samples":[...]}) or newline-delimited integers (nanoseconds).
*/
func EffectiveNetworkLatencyFromFile(path string) time.Duration {
	data, err := os.ReadFile(path)

	if err != nil {
		errnie.Error(err)

		return 0
	}

	values := parseLatencySamples(data)

	if len(values) == 0 {
		return 0
	}

	return time.Duration(percentileInt64(values, 0.95))
}

func parseLatencySamples(data []byte) []int64 {
	trimmed := bytes.TrimSpace(data)

	if len(trimmed) == 0 {
		return nil
	}

	if trimmed[0] == '[' || trimmed[0] == '{' {
		if values := parseLatencyJSON(trimmed); len(values) > 0 {
			return values
		}
	}

	return parseLatencyLines(trimmed)
}

func parseLatencyJSON(data []byte) []int64 {
	var values []int64

	if err := json.Unmarshal(data, &values); err == nil {
		return positiveLatencySamples(values)
	}

	var wrapped struct {
		Samples []int64 `json:"samples"`
	}

	if err := json.Unmarshal(data, &wrapped); err == nil {
		return positiveLatencySamples(wrapped.Samples)
	}

	return nil
}

func parseLatencyLines(data []byte) []int64 {
	lines := strings.Split(string(data), "\n")
	values := make([]int64, 0, len(lines))

	for _, line := range lines {
		line = strings.TrimSpace(line)

		if line == "" {
			continue
		}

		value, err := strconv.ParseInt(line, 10, 64)

		if err != nil || value <= 0 {
			continue
		}

		values = append(values, value)
	}

	return values
}

func positiveLatencySamples(values []int64) []int64 {
	if len(values) == 0 {
		return nil
	}

	positive := make([]int64, 0, len(values))

	for _, value := range values {
		if value > 0 {
			positive = append(positive, value)
		}
	}

	return positive
}

func percentileInt64(values []int64, quantile float64) int64 {
	if len(values) == 0 {
		return 0
	}

	sorted := append([]int64(nil), values...)
	sortInt64(sorted)

	index := int(math.Floor(float64(len(sorted)-1) * quantile))

	if index < 0 {
		index = 0
	}

	return sorted[index]
}

func sortInt64(values []int64) {
	for left := 1; left < len(values); left++ {
		cursor := values[left]
		walk := left

		for walk > 0 && values[walk-1] > cursor {
			values[walk] = values[walk-1]
			walk--
		}

		values[walk] = cursor
	}
}
