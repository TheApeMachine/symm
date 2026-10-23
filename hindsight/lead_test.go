package hindsight_test

import (
	"testing"

	"github.com/theapemachine/symm/hindsight"
	"github.com/theapemachine/symm/system"
)

func TestApplyExecutionLeadOnFields(t *testing.T) {
	system.Cfg = system.NewConfig()
	system.Cfg.Detector = &system.Detector{
		MinPrecursorTicks:     10,
		ExecutionLeadFraction: 0.1,
	}

	fields := map[string]float64{
		"a_idx": 100, "a_tick": 1000,
		"b_idx": 110, "b_tick": 1010,
		"c_idx": 130, "c_tick": 1030,
		"d_idx": 140, "d_tick": 1040,
		"b_price": 1.1, "c_price": 1.2,
	}

	hindsight.ApplyExecutionLeadFields(fields)

	if fields["b_tick"] != 1009 || fields["c_tick"] != 1029 {
		t.Fatalf("shifted ticks = B %v C %v, want 1009 / 1029", fields["b_tick"], fields["c_tick"])
	}

	if fields["entry_tick"] != fields["b_tick"] || fields["exit_tick"] != fields["c_tick"] {
		t.Fatal("entry/exit ticks must match shifted B and C")
	}
}
