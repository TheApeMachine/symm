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
		path := filepath.Join(t.TempDir(), "capture.jsonl")
		raw := []byte(
			`{"Symbol":"BTC/EUR","Source":1,"Category":"laminar","SNR":2,"Last":100}` + "\n" +
				`{"Symbol":"BTC/EUR","Source":12,"Category":"aggressive_drive","SNR":3,"Last":101}` + "\n",
		)

		convey.So(os.WriteFile(path, raw, 0o644), convey.ShouldBeNil)

		rows, skipped, err := LoadMeasurements(path, 0)

		convey.Convey("It should decode each measurement row", func() {
			convey.So(err, convey.ShouldBeNil)
			convey.So(skipped, convey.ShouldEqual, 0)
			convey.So(len(rows), convey.ShouldEqual, 2)
			convey.So(rows[0].Category, convey.ShouldEqual, perspectives.CategoryLaminar)
			convey.So(rows[1].Source, convey.ShouldEqual, perspectives.SourceCVD)
		})
	})

	convey.Convey("Given a tape with a truncated tail line", t, func() {
		path := filepath.Join(t.TempDir(), "capture.jsonl")
		raw := []byte(
			`{"Symbol":"BTC/EUR","Source":1,"Category":"laminar","SNR":2,"Last":100}` + "\n" +
				`{"a` + "\n",
		)

		convey.So(os.WriteFile(path, raw, 0o644), convey.ShouldBeNil)

		rows, skipped, err := LoadMeasurements(path, 0)

		convey.Convey("It should load valid rows and skip the tail fragment", func() {
			convey.So(err, convey.ShouldBeNil)
			convey.So(skipped, convey.ShouldEqual, 1)
			convey.So(len(rows), convey.ShouldEqual, 1)
		})
	})

	convey.Convey("Given only malformed measurement lines", t, func() {
		path := filepath.Join(t.TempDir(), "capture.jsonl")
		raw := []byte(`{bad` + "\n" + `{"incomplete":` + "\n")

		convey.So(os.WriteFile(path, raw, 0o644), convey.ShouldBeNil)

		rows, skipped, err := LoadMeasurements(path, 0)

		convey.Convey("It should return an error reporting skipped lines", func() {
			convey.So(len(rows), convey.ShouldEqual, 0)
			convey.So(skipped, convey.ShouldEqual, 2)
			convey.So(err, convey.ShouldNotBeNil)
			convey.So(err.Error(), convey.ShouldContainSubstring, "skipped 2")
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
		rows := profitableMultiSignalRows()

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
			convey.So(summary.Candidates, convey.ShouldBeGreaterThan, 0)
			convey.So(len(candidates), convey.ShouldEqual, summary.Candidates)
			convey.So(candidates[0].ProfitLoss(), convey.ShouldEqual, candidates[0].Score)
			if candidates[0].ClosedTrades > 0 {
				convey.So(
					candidates[0].ReturnPct(),
					convey.ShouldAlmostEqual,
					(candidates[0].Score/float64(candidates[0].ClosedTrades))*100,
					0.000001,
				)
			}
			convey.So(len(lines), convey.ShouldEqual, summary.Candidates)
			convey.So(lines[0], convey.ShouldContainSubstring, `"profit_loss"`)
			convey.So(lines[0], convey.ShouldContainSubstring, `"return_pct"`)
			convey.So(lines[0], convey.ShouldContainSubstring, `"observation":"not_holding"`)
			convey.So(lines[0], convey.ShouldContainSubstring, `"unit":"snr"`)
		})
	})
}

func TestTuneMeasurementsWritesBestTree(t *testing.T) {
	convey.Convey("Given an output path and trade activity on the tape", t, func() {
		outputPath := filepath.Join(t.TempDir(), "perspectives.yaml")
		writeCount := 0
		rows := profitableMultiSignalRows()

		_, err := TuneMeasurements(
			context.Background(),
			rows,
			TuneOptions{
				OutputPath:     outputPath,
				Workers:        2,
				MaxThresholds:  2,
				BeamWidth:      8,
				CandidateLimit: 512,
				OnBest: func(best BestTree) {
					writeCount++
				},
			},
		)
		raw, readErr := os.ReadFile(outputPath)

		convey.Convey("It should overwrite the YAML when a new best tree appears", func() {
			convey.So(err, convey.ShouldBeNil)
			convey.So(readErr, convey.ShouldBeNil)
			convey.So(writeCount, convey.ShouldBeGreaterThan, 0)
			convey.So(string(raw), convey.ShouldContainSubstring, "branches:")
		})
	})
}

func TestSubsampleMeasurements(t *testing.T) {
	convey.Convey("Given a long measurement tape", t, func() {
		rows := make([]perspectives.Measurement, 1000)

		for index := range rows {
			rows[index] = perspectives.Measurement{Symbol: "BTC/EUR", Last: float64(index)}
		}

		convey.Convey("It should cap the replay rows evenly", func() {
			sampled := SubsampleMeasurements(rows, 100)

			convey.So(len(sampled), convey.ShouldEqual, 100)
			convey.So(sampled[0].Last, convey.ShouldEqual, 0)
			convey.So(sampled[len(sampled)-1].Last, convey.ShouldEqual, 999)
		})

		convey.Convey("It should preserve tail coverage for non-divisor lengths", func() {
			shortRows := make([]perspectives.Measurement, 199)

			for index := range shortRows {
				shortRows[index] = perspectives.Measurement{Symbol: "BTC/EUR", Last: float64(index)}
			}

			sampled := SubsampleMeasurements(shortRows, 100)

			convey.So(len(sampled), convey.ShouldEqual, 100)
			convey.So(sampled[len(sampled)-1].Last, convey.ShouldBeGreaterThanOrEqualTo, 150)
		})
	})
}
