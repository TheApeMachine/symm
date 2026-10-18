package strategy

import (
	"encoding/binary"
	"math"
	"testing"

	"github.com/theapemachine/symm/data"
)

func TestDetectFieldsReadsPackedVector(t *testing.T) {
	keys := []string{
		"a_idx", "a_tick", "b_idx", "b_tick", "c_idx", "c_tick", "d_idx", "d_tick",
		"b_price", "c_price",
	}
	values := []float64{0, 28034, 7, 34704, 106, 68218, 130, 87257, 0.0276, 0.02924}
	packed := make([]byte, len(values)*8)

	for i, value := range values {
		binary.LittleEndian.PutUint64(packed[i*8:], math.Float64bits(value))
	}

	stored := data.New()
	stored.Prefix = ""
	stored.Layers = [][]byte{packed}
	stored.Index = []string{
		"detections/up/CPOOL/USD/1792189318815706000/0000000000000068218.bin/" + joinKeys(keys),
	}

	fields, err := detectFields(stored)

	if err != nil {
		t.Fatal(err)
	}

	if fields["a_tick"] != 28034 || fields["c_tick"] != 68218 || fields["b_price"] != 0.0276 {
		t.Fatalf("fields = %#v", fields)
	}
}

func TestDetectFieldsReadsGobVector(t *testing.T) {
	value := data.NewValue("detections", "up", "CPOOL/USD")
	value.Append([]float64{0, 28034, 7, 34704, 106, 68218, 130, 87257, 0.0276, 0.02924},
		"a_idx", "a_tick", "b_idx", "b_tick", "c_idx", "c_tick", "d_idx", "d_tick", "b_price", "c_price",
	)

	fields, err := detectFields(value)

	if err != nil {
		t.Fatal(err)
	}

	if fields["b_tick"] != 34704 || fields["c_price"] != 0.02924 {
		t.Fatalf("fields = %#v", fields)
	}
}

func joinKeys(keys []string) string {
	out := keys[0]

	for _, key := range keys[1:] {
		out += "/" + key
	}

	return out
}
