package hawkes

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestSample_MeasureArrival(t *testing.T) {
	Convey("Given distinct trades one nanosecond apart", t, func() {
		sample := NewSample()
		base := time.Date(2026, 5, 30, 12, 0, 0, 1, time.UTC)
		first, ready, err := sample.MeasureArrival(
			tradeInput("ALT/EUR", "buy", base),
		)
		So(err, ShouldBeNil)
		So(ready, ShouldBeTrue)
		So(first.Horizon, ShouldResemble, base)

		_, ready, err = sample.MeasureArrival(
			tradeInput("ALT/EUR", "sell", base.Add(time.Nanosecond)),
		)
		So(err, ShouldBeNil)
		So(ready, ShouldBeTrue)
		input, ready, err := sample.MeasureArrival(
			tradeInput("ALT/EUR", "buy", base.Add(2*time.Nanosecond)),
		)

		Convey("It should preserve both native timestamps exactly", func() {
			So(err, ShouldBeNil)
			So(ready, ShouldBeTrue)
			So(input.ObservedFrom, ShouldResemble, base)
			So(input.Stream.BuyTimes()[0].UnixNano(), ShouldEqual, base.UnixNano())
			So(input.Stream.SellTimes()[0].UnixNano(), ShouldEqual,
				base.Add(time.Nanosecond).UnixNano())
		})
	})

	Convey("Given arrivals beyond the derived observation window", t, func() {
		sample := NewSample()
		base := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)
		var input Input

		for index := range 128 {
			side := "buy"
			arrival := base

			if index%2 != 0 {
				side = "sell"
			}

			if index > 0 {
				arrival = base.Add(time.Hour + time.Duration(index)*time.Second)
			}

			var err error
			input, _, err = sample.MeasureArrival(tradeInput(
				"ALT/EUR",
				side,
				arrival,
			))
			So(err, ShouldBeNil)
		}

		Convey("It should release arrivals before Nomagique's selected horizon", func() {
			earliest, _, found := sample.window("ALT/EUR").arrivals.Stream().Bounds()

			So(found, ShouldBeTrue)
			So(earliest.Before(input.ObservedFrom), ShouldBeFalse)
			So(earliest.After(base), ShouldBeTrue)
		})
	})

	Convey("Given an invalid trade side", t, func() {
		sample := NewSample()
		_, _, err := sample.MeasureArrival(tradeInput(
			"ALT/EUR", "unknown", time.Now(),
		))

		Convey("It should return a validation error", func() {
			So(err, ShouldNotBeNil)
		})
	})
}

func TestTradeInput_ArrivalTime(t *testing.T) {
	Convey("Given an integer exchange timestamp", t, func() {
		expected := time.Date(2026, 5, 30, 12, 0, 0, 17, time.UTC)
		input := TradeInput{UnixNano: expected.UnixNano()}

		Convey("It should reconstruct the exact native time", func() {
			So(input.ArrivalTime(), ShouldResemble, expected)
		})
	})
}

func BenchmarkSample_MeasureArrival(t *testing.B) {
	sample := NewSample()
	base := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)
	iteration := 0

	t.ReportAllocs()

	for t.Loop() {
		side := "buy"

		if iteration%2 == 1 {
			side = "sell"
		}

		_, _, _ = sample.MeasureArrival(tradeInput(
			"ALT/EUR",
			side,
			base.Add(time.Duration(iteration)*time.Millisecond),
		))
		iteration++
	}
}

func tradeInput(symbol string, side string, at time.Time) TradeInput {
	return TradeInput{
		Symbol:    symbol,
		Side:      side,
		Timestamp: at,
	}
}
