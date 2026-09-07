package core_test

import (
	"github.com/theapemachine/symm/nomagique/core"
	"testing"
)

func TestRecordField(t *testing.T) {
	fields := core.To[map[string]core.Primitive](core.Record(map[string]any{"zero": 0.0, "epoch": uint64(9)}))
	if value, err := core.Field[float64](fields, "zero"); err != nil || value != 0 {
		t.Fatalf("observed zero: %g, %v", value, err)
	}
	if _, err := core.Field[float64](fields, "absent"); err == nil {
		t.Fatal("missing field must fail")
	}
	if _, err := core.Field[float64](fields, "epoch"); err == nil {
		t.Fatal("wrong type must fail")
	}
}
