package tune

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/symm/market/perspectives"
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
func profitableRows() []perspectives.Measurement {
	return profitableRowsFor("BTC/EUR", 3)
}

func profitableRowsFor(symbol string, legs int) []perspectives.Measurement {
	base := time.Unix(1_700_000_000, 0)
	step := time.Second
	rows := make([]perspectives.Measurement, 0, 16)

	start := 100.0
	at := base

	for range legs {
		rows = append(rows,
			perspectives.Measurement{Symbol: symbol, Category: perspectives.CategoryVerticalIgnition, SNR: 1.5, Last: start, At: at},
			perspectives.Measurement{Symbol: symbol, Last: start * 1.05, At: at.Add(step)},
			perspectives.Measurement{Symbol: symbol, Last: start * 1.10, At: at.Add(2 * step)},
			perspectives.Measurement{Symbol: symbol, Last: start * 1.07, At: at.Add(3 * step)},
		)
		start *= 1.07
		at = at.Add(5 * step)
	}

	return rows
}

func flatRows(symbols ...string) []perspectives.Measurement {
	base := time.Unix(1_700_000_000, 0)
	rows := make([]perspectives.Measurement, 0, len(symbols))

	for symbolIndex, symbol := range symbols {
		rows = append(rows, perspectives.Measurement{
			Symbol: symbol,
			Last:   100 + float64(symbolIndex),
			At:     base.Add(time.Duration(symbolIndex) * time.Second),
		})
	}

	return rows
}

func TestTuneMeasurements(t *testing.T) {
	convey.Convey("Given a profitable measurement tape", t, func() {
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
			thoughts, parseErr := perspectives.ParseThoughts(raw)
			convey.So(parseErr, convey.ShouldBeNil)
			convey.So(len(thoughts), convey.ShouldBeGreaterThan, 0)
		})
	})
}

func TestTuneMeasurementsFiltersFundableRows(t *testing.T) {
	convey.Convey("Given a capture with EUR and non-EUR symbols", t, func() {
		viper.Reset()
		defer viper.Reset()

		viper.Set("market.quote_currency", "EUR")
		viper.Set("trading.paper.wallet_eur", 200.0)

		fundable := profitableRowsFor("BTC/EUR", 3)
		unfundable := profitableRowsFor("ETH/BTC", 3)
		rows := append(append([]perspectives.Measurement{}, fundable...), unfundable...)
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

func TestTuneMeasurementsDoesNotWriteSparseCandidate(t *testing.T) {
	convey.Convey("Given a sparse winner on a larger EUR universe", t, func() {
		viper.Reset()
		defer viper.Reset()

		viper.Set("market.quote_currency", "EUR")
		viper.Set("trading.paper.wallet_eur", 200.0)

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

		convey.Convey("It should preserve the existing playbook instead of writing", func() {
			convey.So(err, convey.ShouldBeNil)
			convey.So(summary.MinRoundTrips, convey.ShouldEqual, 3)
			convey.So(summary.Trades, convey.ShouldBeLessThan, summary.MinRoundTrips)

			_, statErr := os.Stat(outputPath)
			convey.So(os.IsNotExist(statErr), convey.ShouldBeTrue)
		})
	})
}
