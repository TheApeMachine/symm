package audit

import (
	"strings"

	"github.com/theapemachine/symm/data"
)

func floatMetrics(value *data.Value) map[string]float64 {
	out := map[string]float64{}

	if value == nil {
		return out
	}

	for index, values := range data.Layer[[]float64](value) {
		rest := index

		if value.Prefix != "" && strings.HasPrefix(index, value.Prefix+"/") {
			rest = strings.TrimPrefix(index, value.Prefix+"/")
		}

		labels := strings.Split(rest, "/")

		for i, reading := range values {
			if i < len(labels) {
				out[labels[i]] = reading
			}
		}
	}

	return out
}
