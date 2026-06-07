package broker

import (
	"fmt"
	"math"
	"os"
	"time"

	"github.com/theapemachine/errnie"
)

/*
EffectiveNetworkLatency reads runs/network_latency.json and returns the p95 sample.
*/
func EffectiveNetworkLatency() time.Duration {
	latencyFile, err := os.Open("runs/network_latency.json")

	if err != nil {
		errnie.Error(err)
		return 0
	}
	defer latencyFile.Close()

	values := make([]int64, 0, 64)

	for {
		var value int64
		_, scanErr := fmt.Fscanf(latencyFile, "%d\n", &value)

		if scanErr != nil {
			break
		}

		if value > 0 {
			values = append(values, value)
		}
	}

	return time.Duration(percentileInt64(values, 0.95))
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
