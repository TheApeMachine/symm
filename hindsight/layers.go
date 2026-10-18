package hindsight

import (
	"bytes"
	"context"
	"strings"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/data"
	"github.com/theapemachine/symm/store"
)

var excursionClasses = []string{"up", "down", "chop", "flat"}

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

type objects struct {
	values []*data.Value
}

func (collected *objects) Write(p []byte) (int, error) {
	collected.values = append(collected.values, data.From(append([]byte(nil), p...)))
	return len(p), nil
}

func stored(client *store.Client, prefix string) []*data.Value {
	if client == nil {
		return nil
	}

	collected := &objects{}
	_ = client.Stream(context.Background(), prefix, collected)

	return collected.values
}

func storedOne(client *store.Client, prefix string) *data.Value {
	values := stored(client, prefix)

	if len(values) == 0 {
		return nil
	}

	return values[0]
}

func readObject(ctx context.Context, client *store.Client, key string) (*data.Value, error) {
	if client == nil {
		return nil, errnie.Err(errnie.Validation, "[hindsight] no store", nil)
	}

	var buf bytes.Buffer

	if err := client.Stream(ctx, key, &buf); err != nil {
		return nil, err
	}

	if buf.Len() == 0 {
		return nil, errnie.Err(errnie.NotFound, "[hindsight] "+key+" has no object", nil)
	}

	return data.From(buf.Bytes()), nil
}
