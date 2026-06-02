package optimizer

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken/trading"
	"github.com/theapemachine/symm/market/perspectives"
)

func TestLoadMeasurements(t *testing.T) {
	convey.Convey("Given a measurement JSONL file", t, func() {
		path := filepath.Join(t.TempDir(), "measurements.jsonl")
		raw := []byte(
			`{"Symbol":"BTC/EUR","Source":1,"Category":"laminar","SNR":2,"Last":100}` + "\n" +
				`{"Symbol":"BTC/EUR","Source":12,"Category":"aggressive_drive","SNR":3,"Last":101}` + "\n",
		)

		convey.So(os.WriteFile(path, raw, 0o644), convey.ShouldBeNil)

		rows, skipped, err := LoadMeasurements(path)

		convey.Convey("It should decode each measurement row", func() {
			convey.So(err, convey.ShouldBeNil)
			convey.So(skipped, convey.ShouldEqual, 0)
			convey.So(len(rows), convey.ShouldEqual, 2)
			convey.So(rows[0].Category, convey.ShouldEqual, perspectives.CategoryLaminar)
			convey.So(rows[1].Source, convey.ShouldEqual, perspectives.SourceCVD)
		})
	})

	convey.Convey("Given a tape with a truncated tail line", t, func() {
		path := filepath.Join(t.TempDir(), "measurements.jsonl")
		raw := []byte(
			`{"Symbol":"BTC/EUR","Source":1,"Category":"laminar","SNR":2,"Last":100}` + "\n" +
				`{"a` + "\n",
		)

		convey.So(os.WriteFile(path, raw, 0o644), convey.ShouldBeNil)

		rows, skipped, err := LoadMeasurements(path)

		convey.Convey("It should load valid rows and skip the tail fragment", func() {
			convey.So(err, convey.ShouldBeNil)
			convey.So(skipped, convey.ShouldEqual, 1)
			convey.So(len(rows), convey.ShouldEqual, 1)
		})
	})
}

func TestWriteBranches(t *testing.T) {
	convey.Convey("Given an optimized branch list", t, func() {
		path := filepath.Join(t.TempDir(), "perspectives.yaml")
		branches := perspectives.BranchList{{
			Category:    perspectives.CategoryLaminar,
			Observation: perspectives.ObservationNotHolding,
			Metric:      "unrealized_return",
			Regime:      perspectives.RegimeBullish,
			Condition:   perspectives.ConditionIsGreaterThanOrEqual,
			Unit:        perspectives.UnitPercentage,
			Value:       1.5,
			ValueSet:    true,
			Action: perspectives.Action{
				Type:     perspectives.ActionLimit,
				Side:     trading.Buy,
				Symbol:   "BTC/EUR",
				Price:    100,
				Quantity: 0.1,
			},
		}}

		err := WriteBranches(path, branches)
		raw, readErr := os.ReadFile(path)

		convey.Convey("It should write a tree document", func() {
			convey.So(err, convey.ShouldBeNil)
			convey.So(readErr, convey.ShouldBeNil)
			convey.So(string(raw), convey.ShouldContainSubstring, "branches:")
			convey.So(string(raw), convey.ShouldContainSubstring, "laminar")
			convey.So(string(raw), convey.ShouldContainSubstring, "observation: not_holding")
			convey.So(string(raw), convey.ShouldContainSubstring, "metric: unrealized_return")
			convey.So(string(raw), convey.ShouldContainSubstring, "regime: bullish")
			convey.So(string(raw), convey.ShouldContainSubstring, "condition: '>='")
			convey.So(string(raw), convey.ShouldContainSubstring, "unit: percentage")
			convey.So(string(raw), convey.ShouldContainSubstring, "value: 1.5")
			convey.So(string(raw), convey.ShouldContainSubstring, "side: buy")
			convey.So(string(raw), convey.ShouldContainSubstring, "symbol: BTC/EUR")
		})
	})
}

func TestTuneMeasurements(t *testing.T) {
	convey.Convey("Given a candidate report path", t, func() {
		path := filepath.Join(t.TempDir(), "candidates.jsonl")
		candidates := make([]CandidateScore, 0)
		rows := []perspectives.Measurement{
			{
				Symbol: "BTC/EUR", Source: perspectives.SourceFluid,
				Category: perspectives.CategoryLaminar,
				SNR:      2,
				Last:     100,
			},
			{
				Symbol: "BTC/EUR", Source: perspectives.SourceExhaustion,
				Category: perspectives.CategoryExhaustion,
				SNR:      2,
				Last:     110,
			},
		}

		summary, err := TuneMeasurements(
			context.Background(),
			rows,
			TuneOptions{
				CandidateReportPath: path,
				Workers:             2,
				MaxThresholds:       2,
				BeamWidth:           4,
				CandidateLimit:      8,
				OnCandidate: func(candidate CandidateScore) {
					candidates = append(candidates, candidate)
				},
			},
		)
		raw, readErr := os.ReadFile(path)
		lines := strings.Split(strings.TrimSpace(string(raw)), "\n")

		convey.Convey("It should write one JSON row per scored candidate", func() {
			convey.So(err, convey.ShouldBeNil)
			convey.So(readErr, convey.ShouldBeNil)
			convey.So(summary.Candidates, convey.ShouldEqual, 8)
			convey.So(len(candidates), convey.ShouldEqual, summary.Candidates)
			convey.So(candidates[0].ProfitLoss(), convey.ShouldEqual, candidates[0].Score)
			convey.So(candidates[0].ReturnPct(), convey.ShouldEqual, candidates[0].Score*100)
			convey.So(len(lines), convey.ShouldEqual, summary.Candidates)
			convey.So(lines[0], convey.ShouldContainSubstring, `"profit_loss"`)
			convey.So(lines[0], convey.ShouldContainSubstring, `"return_pct"`)
			convey.So(lines[0], convey.ShouldContainSubstring, `"observation":"not_holding"`)
			convey.So(lines[0], convey.ShouldContainSubstring, `"unit":"snr"`)
		})
	})
}
