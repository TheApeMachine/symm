package market

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	krakenmarket "github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/logic"
)

func TestSymbolStateAbsorb(t *testing.T) {
	Convey("Given a symbol spectrum accumulator", t, func() {
		state := newSymbolState(32)

		Convey("It should overwrite duplicate source updates before the spectrum completes", func() {
			first := logic.Measurement{
				Source:     logic.SourceHawkes,
				Symbol:     "BTC/USD",
				Price:      1,
				Strength:   1,
				Volume:     1,
				Spread:     1,
				Elapsed:    1,
				Confidence: 1,
				Surprise:   1,
				ObservedAt: time.Now(),
			}

			complete, absorbErr := state.absorb(first)

			So(absorbErr, ShouldBeNil)
			So(complete, ShouldBeFalse)

			second := first
			second.Strength = 2

			complete, absorbErr = state.absorb(second)

			So(absorbErr, ShouldBeNil)
			So(complete, ShouldBeFalse)

			index, indexErr := logic.SourceIndex(logic.SourceHawkes)

			So(indexErr, ShouldBeNil)
			So(state.slots[index].Strength, ShouldEqual, 2)
		})

		Convey("It should complete only after all spectrum sources report", func() {
			for sourceIndex, source := range logic.SpectrumSources {
				measurement := logic.Measurement{
					Source:     source,
					Symbol:     "BTC/USD",
					Price:      1,
					Strength:   1,
					Volume:     1,
					Spread:     1,
					Elapsed:    1,
					Confidence: 1,
					Surprise:   1,
					ObservedAt: time.Now(),
				}

				complete, absorbErr := state.absorb(measurement)

				if sourceIndex < logic.SourceCount-1 {
					So(absorbErr, ShouldBeNil)
					So(complete, ShouldBeFalse)

					continue
				}

				So(absorbErr, ShouldBeNil)
				So(complete, ShouldBeTrue)
			}
		})
	})
}

func TestSymbolStateSlotMeasurements(t *testing.T) {
	Convey("Given a partial spectrum", t, func() {
		state := newSymbolState(32)
		eventAt := time.Now()

		row, rowErr := krakenmarket.NewSymbolRow(
			"BTC/USD",
			100,
			0.01,
			100,
			1,
			eventAt,
		)

		So(rowErr, ShouldBeNil)

		measurement := logic.Measurement{
			Source:     logic.SourceLeadLag,
			Symbol:     "BTC/USD",
			Price:      100,
			Strength:   1,
			Volume:     1,
			Spread:     1,
			Elapsed:    1,
			Confidence: 0.7,
			Surprise:   1.1,
			ObservedAt: eventAt,
			Market:     *row,
		}

		_, absorbErr := state.absorb(measurement)

		So(absorbErr, ShouldBeNil)

		readings := state.slotMeasurements()

		Convey("It should expose publishable slot readings for dashboard gauges", func() {
			So(len(readings), ShouldEqual, 1)
			So(readings[0].Source, ShouldEqual, logic.SourceLeadLag)
		})
	})
}

func BenchmarkSymbolStateSlotMeasurements(b *testing.B) {
	state := newSymbolState(32)
	eventAt := time.Unix(100, 0)
	row, rowErr := krakenmarket.NewSymbolRow(
		"BTC/USD",
		100,
		0.01,
		100,
		1,
		eventAt,
	)

	if rowErr != nil {
		b.Fatal(rowErr)
	}

	for _, source := range logic.SpectrumSources {
		_, absorbErr := state.absorb(logic.Measurement{
			Source:     source,
			Symbol:     "BTC/USD",
			Price:      100,
			Strength:   1,
			Volume:     1,
			Spread:     1,
			Elapsed:    1,
			Confidence: 0.7,
			Surprise:   1.1,
			ObservedAt: eventAt,
			Market:     *row,
		})

		if absorbErr != nil {
			b.Fatal(absorbErr)
		}
	}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_ = state.slotMeasurements()
	}
}
