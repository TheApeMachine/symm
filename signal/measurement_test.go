package signal

import (
	"container/ring"
	"errors"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	krakenmarket "github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/logic"
)

func TestUniformConfidence(t *testing.T) {
	Convey("Given four causal categories", t, func() {
		So(UniformConfidence(4), ShouldEqual, 0.25)
	})

	Convey("Given three toxicity categories", t, func() {
		So(UniformConfidence(3), ShouldAlmostEqual, 1.0/3.0, 0.0001)
	})
}

func TestBestEffortFromBookTouch(t *testing.T) {
	Convey("Given one book update in the ring", t, func() {
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

		measurement, err := BestEffort(
			logic.SourceDepthFlow,
			"BTC/USD",
			logic.CategoryDenseNeutrality,
			4,
			measurements,
			at,
		)

		Convey("It should publish a uniform best-effort measurement", func() {
			So(err, ShouldBeNil)
			So(measurement.Publishable(), ShouldBeTrue)
			So(measurement.Confidence, ShouldEqual, 0.25)
			So(measurement.Surprise, ShouldEqual, 0.25)
			So(measurement.Source, ShouldEqual, logic.SourceDepthFlow)
			So(measurement.Category, ShouldEqual, logic.CategoryDenseNeutrality)
		})
	})
}

func TestBestEffortFromTickerTouch(t *testing.T) {
	Convey("Given a book-triggered ticker in the ring", t, func() {
		measurements := ring.New(4)
		at := time.Unix(100, 0)
		ticker := &krakenmarket.TickerUpdate{
			Symbol:    "BTC/EUR",
			Bid:       49990,
			Ask:       50010,
			AskQty:    1,
			BidQty:    1,
			Timestamp: at.Add(-time.Second),
		}

		measurements.Value = ticker
		measurements = measurements.Next()

		measurement, err := BestEffort(
			logic.SourceLiquidity,
			"BTC/EUR",
			logic.CategoryMedianDepth,
			3,
			measurements,
			at,
		)

		Convey("It should publish a uniform best-effort measurement", func() {
			So(err, ShouldBeNil)
			So(measurement.Publishable(), ShouldBeTrue)
			So(measurement.Confidence, ShouldAlmostEqual, 1.0/3.0, 0.0001)
		})
	})
}

func TestBestEffortFromTradeTouch(t *testing.T) {
	Convey("Given one trade in the ring", t, func() {
		measurements := ring.New(4)
		at := time.Unix(100, 0)
		trade := &krakenmarket.TradeUpdate{
			Symbol:    "BTC/USD",
			Price:     50000,
			Qty:       0.01,
			Timestamp: at.Add(-time.Second),
		}

		measurements.Value = trade
		measurements = measurements.Next()

		measurement, err := BestEffort(
			logic.SourceHawkes,
			"BTC/USD",
			logic.CategoryOrganic,
			4,
			measurements,
			at,
		)

		Convey("It should publish a uniform best-effort measurement", func() {
			So(err, ShouldBeNil)
			So(measurement.Publishable(), ShouldBeTrue)
			So(measurement.Price, ShouldEqual, 50000)
		})
	})
}

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

		measurement, finishErr := FinishMeasure(
			logic.SourceHawkes,
			"BTC/USD",
			logic.CategoryOrganic,
			4,
			nil,
			at,
			candidate,
			nil,
		)

		Convey("It should return the candidate unchanged", func() {
			So(finishErr, ShouldBeNil)
			So(measurement, ShouldResemble, candidate)
		})
	})
}

func TestFinishMeasureFallsBackToBestEffort(t *testing.T) {
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

		measurement, finishErr := FinishMeasure(
			logic.SourceToxicity,
			"ETH/USD",
			logic.CategoryHardSupport,
			3,
			measurements,
			at,
			logic.Measurement{},
			nil,
		)

		Convey("It should publish a uniform best-effort measurement", func() {
			So(finishErr, ShouldBeNil)
			So(measurement.Publishable(), ShouldBeTrue)
			So(measurement.Source, ShouldEqual, logic.SourceToxicity)
			So(measurement.Confidence, ShouldAlmostEqual, 1.0/3.0, 0.0001)
			So(measurement.BestEffort, ShouldBeTrue)
			So(measurement.GapReason, ShouldNotBeEmpty)
		})
	})
}

func TestFinishMeasureReturnsMeasureError(t *testing.T) {
	Convey("Given a candidate error", t, func() {
		at := time.Unix(100, 0)

		_, finishErr := FinishMeasure(
			logic.SourceHawkes,
			"BTC/USD",
			logic.CategoryOrganic,
			4,
			nil,
			at,
			logic.Measurement{},
			errors.New("signal: measure failed"),
		)

		Convey("It should propagate the error", func() {
			So(finishErr, ShouldNotBeNil)
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
		_, _ = BestEffort(
			logic.SourceDepthFlow,
			"BTC/USD",
			logic.CategoryDenseNeutrality,
			4,
			measurements,
			at,
		)
	}
}
