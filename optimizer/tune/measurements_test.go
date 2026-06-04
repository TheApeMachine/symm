package tune

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/smartystreets/goconvey/convey"
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
	base := time.Unix(1_700_000_000, 0)
	step := time.Second
	rows := make([]perspectives.Measurement, 0, 16)

	start := 100.0
	at := base

	for leg := 0; leg < 3; leg++ {
		rows = append(rows,
			perspectives.Measurement{Symbol: "BTC/EUR", Category: perspectives.CategoryVerticalIgnition, SNR: 1.5, Last: start, At: at},
			perspectives.Measurement{Symbol: "BTC/EUR", Last: start * 1.05, At: at.Add(step)},
			perspectives.Measurement{Symbol: "BTC/EUR", Last: start * 1.10, At: at.Add(2 * step)},
			perspectives.Measurement{Symbol: "BTC/EUR", Last: start * 1.07, At: at.Add(3 * step)},
		)
		start *= 1.07
		at = at.Add(5 * step)
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

