package strategy

import (
	"testing"

	"github.com/theapemachine/symm/nomagique/data"
)

func TestInspectMatchNarrowsStoredPaths(t *testing.T) {
	current := &policy{
		tokenizer: &Tokenizer{Depth: 3, Regions: 1, Threshold: 0.5},
		keys: []string{
			"R01/R02/R03/enter.json",
			"R01/R02/R04/exit.json",
			"R01/R05/R06/enter.json",
			"R07/R02/R03/exit.json",
		},
	}

	metrics := map[string]float64{}
	inspectMatch(current, []string{"R01", "R02"}, 0.7, metrics)

	want := map[string]float64{
		"match_candidates:01": 3, "match_enter:01": 2, "match_exit:01": 1,
		"match_candidates:02": 2, "match_enter:02": 1, "match_exit:02": 1,
		"path_token:00:0": 1, "path_token:01:0": 2,
		"match_key:000": 0, "match_key:001": 1,
		"token_depth": 2, "token_capacity": 3, "stored_paths": 4,
	}

	for key, value := range want {
		if metrics[key] != value {
			t.Fatalf("%s = %v, want %v", key, metrics[key], value)
		}
	}
}

/*
Every Metric the dashboard reads from the Training Measurement must have a
declared dimension, or NewMetric gets no unit and validation rejects it.
*/
func TestInspectMetricsHaveDimensions(t *testing.T) {
	current := &policy{
		tokenizer: &Tokenizer{Depth: 4, Regions: 2, Threshold: 0.5},
		keys:      []string{"R01-R02/R03-R04/R05-R06/R07-R08/enter.json"},
	}

	metrics := map[string]float64{}
	inspectMatch(current, []string{"R01-R02", "R03-R04", "R05-R06", "R07-R08"}, 0.7, metrics)

	for _, name := range []string{
		"position_open", "position_exiting", "position_qty", "position_bid_depth_qty",
		"position_held_s", "position_entry", "position_bid", "position_ask",
		"position_exit_vwap", "position_cost", "position_entry_fee", "position_value",
		"position_pnl", "position_pnl_pct", "position_exit_room",
	} {
		metrics[name] = 1
	}

	for key := range metrics {
		metric := data.NewMetric(key, metrics[key], data.Unit(""), data.Timescale(""))
		unit, timescale := data.GetDimension("training", key)

		if unit == data.UnitNone || timescale == data.TimescaleNone {
			t.Fatalf("%s has no declared dimension", key)
		}

		metric = data.NewMetric(key, metrics[key], unit, timescale)

		if metric.Unit() == data.UnitNone || metric.Timescale() == data.TimescaleNone {
			t.Fatalf("%s built without unit/timescale", key)
		}
	}
}

func TestKeysHashIsFNV1a(t *testing.T) {
	// FNV-1a 32 of "a\nb", computed independently.
	if got := keysHash([]string{"a", "b"}); got != 0x28e4c710 {
		t.Fatalf("keysHash = %#x", got)
	}
}
