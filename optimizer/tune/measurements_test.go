package tune

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/symm/internal/testconfig"
	preasoning "github.com/theapemachine/symm/market/perspectives/reasoning"
	ptypes "github.com/theapemachine/symm/market/perspectives/types"
	"github.com/theapemachine/symm/optimizer/io"
	"github.com/theapemachine/symm/optimizer/types"
)

func TestLoadMeasurements(t *testing.T) {
	convey.Convey("Given a measurement JSONL file", t, func() {
		path := filepath.Join(t.TempDir(), "capture.jsonl")
		raw := []byte(
			`{"Symbol":"BTC/EUR","Source":1,"Category":"laminar","SNR":2,"Last":100}` + "\n" +
				`{"Symbol":"BTC/EUR","Source":12,"Category":"aggressive_drive","SNR":3,"Last":101}` + "\n",
		)

		convey.So(os.WriteFile(path, raw, 0o644), convey.ShouldBeNil)

		rows, skipped, err := io.LoadMeasurements(path)

		convey.Convey("It should decode each measurement row", func() {
			convey.So(err, convey.ShouldBeNil)
			convey.So(skipped, convey.ShouldEqual, 0)
			convey.So(len(rows), convey.ShouldEqual, 2)
			convey.So(rows[0].Category, convey.ShouldEqual, ptypes.CategoryLaminar)
			convey.So(rows[1].Source, convey.ShouldEqual, ptypes.SourceCVD)
		})
	})

	convey.Convey("Given a tape with a truncated tail line", t, func() {
		path := filepath.Join(t.TempDir(), "capture.jsonl")
		raw := []byte(
			`{"Symbol":"BTC/EUR","Source":1,"Category":"laminar","SNR":2,"Last":100}` + "\n" +
				`{"a` + "\n",
		)

		convey.So(os.WriteFile(path, raw, 0o644), convey.ShouldBeNil)

		rows, skipped, err := io.LoadMeasurements(path)

		convey.Convey("It should load valid rows and skip the tail fragment", func() {
			convey.So(err, convey.ShouldBeNil)
			convey.So(skipped, convey.ShouldEqual, 1)
			convey.So(len(rows), convey.ShouldEqual, 1)
		})
	})
}

// profitableRows is a tape of repeated rallies, each opened by an ignition signal,
// so the optimizer can discover an enter-on-signal / ride-the-trail strategy.
func profitableRows() []ptypes.Measurement {
	return profitableRowsFor("BTC/EUR", 3)
}

func profitableRowsFor(symbol string, legs int) []ptypes.Measurement {
	base := time.Unix(1_700_000_000, 0)
	step := time.Second
	rows := make([]ptypes.Measurement, 0, 16)

	start := 100.0
	at := base

	for range legs {
		rows = append(rows,
			ptypes.Measurement{Symbol: symbol, Category: ptypes.CategoryVerticalIgnition, SNR: 1.5, Last: start, At: at},
			ptypes.Measurement{Symbol: symbol, Last: start * 1.05, At: at.Add(step)},
			ptypes.Measurement{Symbol: symbol, Last: start * 1.10, At: at.Add(2 * step)},
			ptypes.Measurement{Symbol: symbol, Last: start * 1.07, At: at.Add(3 * step)},
		)
		start *= 1.07
		at = at.Add(5 * step)
	}

	return rows
}

func flatRows(symbols ...string) []ptypes.Measurement {
	base := time.Unix(1_700_000_000, 0)
	rows := make([]ptypes.Measurement, 0, len(symbols))

	for symbolIndex, symbol := range symbols {
		rows = append(rows, ptypes.Measurement{
			Symbol: symbol,
			Last:   100 + float64(symbolIndex),
			At:     base.Add(time.Duration(symbolIndex) * time.Second),
		})
	}

	return rows
}

func loadTuneTestConfig(t *testing.T) {
	t.Helper()

	viper.Reset()
	testconfig.Load(t)
	viper.Set("trading.replay.execution_stress_enabled", false)
}

func TestTuneMeasurements(t *testing.T) {
	convey.Convey("Given a profitable measurement tape", t, func() {
		loadTuneTestConfig(t)
		defer viper.Reset()

		outputPath := filepath.Join(t.TempDir(), "perspectives.yaml")
		bestCount := 0
		candidateCount := 0

		summary, err := TuneMeasurements(
			context.Background(),
			profitableRows(),
			types.TuneOptions{
				OutputPath: outputPath,
				Workers:    2,
				BeamWidth:  6,
				MaxRounds:  6,
				OnBest:     func(types.BestTree) { bestCount++ },
				OnCandidate: func(types.CandidateScore) {
					candidateCount++
				},
			},
		)

		convey.Convey("It searches, reports, and writes a re-parseable Thought playbook", func() {
			convey.So(err, convey.ShouldBeNil)
			convey.So(summary.Evaluated, convey.ShouldBeGreaterThan, 0)
			convey.So(summary.BestReturn, convey.ShouldBeGreaterThan, 0)
			convey.So(summary.Trades, convey.ShouldBeGreaterThan, 0)
			convey.So(summary.Strategies, convey.ShouldBeGreaterThan, 0)
			convey.So(bestCount, convey.ShouldBeGreaterThan, 0)
			convey.So(candidateCount, convey.ShouldBeGreaterThan, 0)

			raw, readErr := os.ReadFile(outputPath)
			convey.So(readErr, convey.ShouldBeNil)
			convey.So(string(raw), convey.ShouldContainSubstring, "when:")

			// The written playbook is a Thought forest the live story can load.
			thoughts, parseErr := preasoning.ParseThoughts(raw)
			convey.So(parseErr, convey.ShouldBeNil)
			convey.So(len(thoughts), convey.ShouldBeGreaterThan, 0)
		})
	})
}

func TestTuneMeasurementsFiltersFundableRows(t *testing.T) {
	convey.Convey("Given a capture with EUR and non-EUR symbols", t, func() {
		loadTuneTestConfig(t)
		defer viper.Reset()

		fundable := profitableRowsFor("BTC/EUR", 3)
		unfundable := profitableRowsFor("ETH/BTC", 3)
		rows := append(append([]ptypes.Measurement{}, fundable...), unfundable...)
		outputPath := filepath.Join(t.TempDir(), "perspectives.yaml")

		summary, err := TuneMeasurements(
			context.Background(),
			rows,
			types.TuneOptions{
				OutputPath: outputPath,
				Workers:    2,
				BeamWidth:  6,
				MaxRounds:  6,
			},
		)

		convey.Convey("It should search only fundable quote-currency rows", func() {
			convey.So(err, convey.ShouldBeNil)
			convey.So(summary.MeasurementCount, convey.ShouldEqual, len(rows))
			convey.So(summary.FundableMeasurements, convey.ShouldEqual, len(fundable))
			convey.So(summary.MinRoundTrips, convey.ShouldEqual, 1)
			convey.So(summary.Trades, convey.ShouldBeGreaterThan, 0)

			_, readErr := os.ReadFile(outputPath)
			convey.So(readErr, convey.ShouldBeNil)
		})
	})
}

func TestTuneMeasurementsMaxMeasurements(t *testing.T) {
	convey.Convey("Given a capture longer than the requested tune limit", t, func() {
		loadTuneTestConfig(t)
		defer viper.Reset()

		first := profitableRowsFor("BTC/EUR", 2)
		rows := append(append([]ptypes.Measurement{}, first...), profitableRowsFor("ETH/EUR", 2)...)
		outputPath := filepath.Join(t.TempDir(), "perspectives.yaml")

		summary, err := TuneMeasurements(
			context.Background(),
			rows,
			types.TuneOptions{
				OutputPath:      outputPath,
				MaxMeasurements: len(first),
				Workers:         2,
				BeamWidth:       6,
				MaxRounds:       6,
			},
		)

		convey.Convey("It should search only the capped prefix", func() {
			convey.So(err, convey.ShouldBeNil)
			convey.So(summary.MeasurementCount, convey.ShouldEqual, len(first))
			convey.So(summary.FundableMeasurements, convey.ShouldEqual, len(first))
			convey.So(summary.Trades, convey.ShouldBeGreaterThan, 0)

			_, readErr := os.ReadFile(outputPath)
			convey.So(readErr, convey.ShouldBeNil)
		})
	})

	convey.Convey("Given a negative tune limit", t, func() {
		summary, err := TuneMeasurements(
			context.Background(),
			profitableRows(),
			types.TuneOptions{MaxMeasurements: -1},
		)

		convey.Convey("It should return an error", func() {
			convey.So(err, convey.ShouldNotBeNil)
			convey.So(summary.MeasurementCount, convey.ShouldEqual, 0)
		})
	})
}

func TestTuneMeasurementsWritesDiscountedSparseCandidate(t *testing.T) {
	convey.Convey("Given a sparse winner on a larger EUR universe", t, func() {
		loadTuneTestConfig(t)
		defer viper.Reset()

		rows := profitableRowsFor("BTC/EUR", 2)
		rows = append(rows, flatRows("ETH/EUR", "SOL/EUR")...)
		outputPath := filepath.Join(t.TempDir(), "perspectives.yaml")

		summary, err := TuneMeasurements(
			context.Background(),
			rows,
			types.TuneOptions{
				OutputPath: outputPath,
				Workers:    2,
				BeamWidth:  6,
				MaxRounds:  6,
			},
		)

		convey.Convey("It should write when the discounted sparse score remains positive", func() {
			convey.So(err, convey.ShouldBeNil)
			convey.So(summary.MinRoundTrips, convey.ShouldEqual, 3)
			convey.So(summary.Trades, convey.ShouldBeLessThan, summary.MinRoundTrips)
			convey.So(summary.BestScore, convey.ShouldBeGreaterThan, 0)

			raw, readErr := os.ReadFile(outputPath)
			convey.So(readErr, convey.ShouldBeNil)
			convey.So(string(raw), convey.ShouldContainSubstring, "when:")
		})
	})
}

func BenchmarkFundableRows(b *testing.B) {
	rows := append(profitableRowsFor("BTC/EUR", 2), profitableRowsFor("ETH/BTC", 2)...)

	for b.Loop() {
		_ = fundableRows(rows, "EUR")
	}
}

func BenchmarkTuneMeasurements(b *testing.B) {
	viper.Reset()
	testconfig.MustLoad()
	viper.Set("trading.replay.execution_stress_enabled", false)

	rows := profitableRows()

	for b.Loop() {
		_, err := TuneMeasurements(
			context.Background(),
			rows,
			types.TuneOptions{
				BeamWidth: 4,
				MaxRounds: 2,
			},
		)

		if err != nil {
			b.Fatal(err)
		}
	}
}
