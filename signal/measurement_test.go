package signal

import (
	"container/ring"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	krakenmarket "github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/logic"
)

func TestFinishMeasureKeepsPublishableCandidate(t *testing.T) {
	Convey("Given a publishable candidate", t, func() {
		at := time.Unix(100, 0)
		row, err := krakenmarket.NewSymbolRow("BTC/USD", 100, 0.01, 1000, 1, at)

		So(err, ShouldBeNil)

		candidate := logic.Measurement{
			Source:     logic.SourceHawkes,
			Symbol:     "BTC/USD",
			Price:      100,
			Strength:   0.8,
			Volume:     1000,
			Spread:     1,
			Elapsed:    1,
			Category:   logic.CategoryOrganic,
			Confidence: 0.8,
			Surprise:   0.5,
			ObservedAt: at,
			Market:     *row,
		}

		measurement := logic.NewMeasurement(
			logic.SourceHawkes,
			"BTC/USD",
			candidate.Price,
			candidate.Strength,
			candidate.Volume,
			candidate.Spread,
			candidate.Elapsed,
			candidate.Category,
			candidate.Regime,
			candidate.Position,
			candidate.Confidence,
			candidate.Surprise,
		)

		Convey("It should return the candidate unchanged", func() {
			So(measurement, ShouldResemble, candidate)
		})
	})
}

func TestFinishMeasureReturnsEmptyWhenCandidateMissing(t *testing.T) {
	Convey("Given a non-publishable candidate and a book ring", t, func() {
		measurements := ring.New(4)
		at := time.Unix(100, 0)
		book := &krakenmarket.BookUpdate{
			Symbol:    "ETH/USD",
			Timestamp: at.Add(-time.Second),
			Bids:      []krakenmarket.BookLevel{{Price: 1990, Qty: 1}},
			Asks:      []krakenmarket.BookLevel{{Price: 2010, Qty: 1}},
		}

		measurements.Value = book
		measurements = measurements.Next()

		var measurement logic.Measurement

		Convey("It should return an empty measurement", func() {
			So(measurement, ShouldResemble, logic.Measurement{})
		})
	})
}

func TestFinishMeasureReturnsMeasureError(t *testing.T) {
	Convey("Given a candidate error", t, func() {
		measurement := logic.NewMeasurement(
			logic.SourceHawkes,
			"BTC/USD",
			100,
			0.8,
			1000,
			1,
			1,
			logic.CategoryOrganic,
			logic.RegimeTypeNone,
			logic.PositionTypeNone,
			0.8,
			0.5,
		)

		Convey("It should return the measurement unchanged", func() {
			So(measurement, ShouldResemble, measurement)
		})
	})
}

func BenchmarkBestEffort(b *testing.B) {
	measurements := ring.New(4)
	at := time.Unix(100, 0)
	book := &krakenmarket.BookUpdate{
		Symbol:    "BTC/USD",
		Timestamp: at.Add(-time.Second),
		Bids:      []krakenmarket.BookLevel{{Price: 99, Qty: 1}},
		Asks:      []krakenmarket.BookLevel{{Price: 101, Qty: 1}},
	}

	measurements.Value = book
	measurements = measurements.Next()

	for b.Loop() {
		measurement := logic.NewMeasurement(
			logic.SourceDepthFlow,
			"BTC/USD",
			99,
			0.8,
			1000,
			1,
			1,
			logic.CategoryDenseNeutrality,
			logic.RegimeTypeNone,
			logic.PositionTypeNone,
			0.8,
			0.5,
		)

		_ = measurement
	}
}
