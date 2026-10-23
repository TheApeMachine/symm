package hindsight

import (
	"testing"

	"github.com/theapemachine/symm/system"
)

func TestAnnotationVectorShiftsBAndC(t *testing.T) {
	system.Cfg = system.NewConfig()
	system.Cfg.Detector = &system.Detector{
		MinPrecursorTicks:     10,
		ExecutionLeadFraction: 0.5,
	}

	trades := []annotationTrade{
		{tick: 100, price: 1.0, ask: 1.01},
		{tick: 108, price: 1.04, ask: 1.05},
		{tick: 110, price: 1.05, ask: 1.06},
		{tick: 127, price: 1.08, bid: 1.07},
		{tick: 128, price: 1.09, bid: 1.08},
		{tick: 130, price: 1.10, bid: 1.09},
		{tick: 140, price: 1.08, bid: 1.07},
	}

	vec, err := annotationVector("up", [4]uint64{100, 110, 130, 140}, trades)
	if err != nil {
		t.Fatal(err)
	}

	if vec[3] != 108 {
		t.Fatalf("stored b_tick = %v, want 108 from the earlier print", vec[3])
	}

	if vec[5] != 128 {
		t.Fatalf("stored c_tick = %v, want 128 from the earlier print", vec[5])
	}

	if vec[15] != vec[3] || vec[17] != vec[5] {
		t.Fatal("entry/exit ticks must match shifted B and C")
	}
}
