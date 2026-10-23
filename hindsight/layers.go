package hindsight

import (
	"strings"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/symm/data"
)

var excursionClasses = []string{"up", "up_friction", "down", "chop", "flat"}

var excursionKeys = []string{
	"a_idx", "a_tick", "b_idx", "b_tick", "c_idx", "c_tick", "d_idx", "d_tick",
	"b_price", "c_price", "b_ask", "c_bid", "b_at", "c_at",
	"entry_idx", "entry_tick", "exit_idx", "exit_tick",
	"entry_ask", "exit_bid", "entry_at", "exit_at",
}

/*
metrics reads gob float layers. Each layer's address segments label its values.
*/
func metrics(value *data.Value) map[string]float64 {
	out := map[string]float64{}

	if value == nil {
		return out
	}

	for i, index := range value.Index {
		if i >= len(value.Layers) {
			continue
		}

		var values []float64
		if err := sonic.Unmarshal(value.Layers[i], &values); err != nil {
			continue
		}

		rest := index

		if value.Prefix != "" && strings.HasPrefix(index, value.Prefix+"/") {
			rest = strings.TrimPrefix(index, value.Prefix+"/")
		}

		labels := strings.Split(rest, "/")

		for j, reading := range values {
			if j < len(labels) {
				out[labels[j]] = reading
			}
		}
	}

	return out
}
