package hindsight

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/hindsight/tables"
	"github.com/theapemachine/symm/hindsight/tables/tablestest"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/tests/market"
)

func TestTapeStep(t *testing.T) {
	Convey("Durable directional legs require a subsequent reversal", t, func() {
		var tape Tape
		var completed []Leg
		fixture := market.NewOpportunityTape("BTC/USD", time.Unix(100, 0), 4)

		for index, step := range fixture.Steps {
			payload, err := json.Marshal(kraken.Trade{Data: []kraken.TradeData{{Symbol: fixture.Symbol, Price: *decimal.NewFromFloat64(step.ExecutableBid)}}})
			So(err, ShouldBeNil)
			So(tape.Step(RawFrame{Kind: "trade", ReceivedAt: step.EventTime, Payload: payload}, func(leg Leg) error { completed = append(completed, leg); return nil }), ShouldBeNil)

			if index == 0 {
				So(completed, ShouldBeEmpty)
			}
		}
		So(len(completed), ShouldBeGreaterThan, 1)

		for _, leg := range completed {
			So(leg.ConfirmedAt.After(leg.Through), ShouldBeTrue)
		}
		So(tape.Step(RawFrame{Kind: "trade", Payload: []byte("broken")}, func(Leg) error { return nil }), ShouldNotBeNil)
	})
}

func TestTapeRead(t *testing.T) {
	Convey("Given durable captures", t, func() {
		catalog := tablestest.New(t)
		writer := tables.NewWriter(catalog)

		var tape Tape

		var run RunID = "test"

		writer.AddCapture(tables.CaptureRow{
			Run: string(run), Sequence: 1, Stream: "spot", StreamEpoch: 1, StreamSequence: 1,
			ReceivedAt: time.Unix(100, 0), Kind: "trade", PayloadHash: "hash",
			Payload: []byte(`{"data":[]}`),
		})

		So(writer.Commit(t.Context()), ShouldBeNil)
		So(tape.Read(t.Context(), catalog, run, func(Leg) error { return nil }), ShouldBeNil)
		So(tape.LastSequence, ShouldEqual, 1)

		Convey("A second read consumes nothing already seen", func() {
			So(tape.Read(t.Context(), catalog, run, func(Leg) error { return nil }), ShouldBeNil)
			So(tape.LastSequence, ShouldEqual, 1)
		})

		Convey("An undecodable capture stops advancement", func() {
			writer.AddCapture(tables.CaptureRow{
				Run: string(run), Sequence: 2, Stream: "spot", StreamEpoch: 1, StreamSequence: 2,
				ReceivedAt: time.Unix(101, 0), Kind: "trade", PayloadHash: "hash",
				Payload: []byte("invalid capture"),
			})

			So(writer.Commit(t.Context()), ShouldBeNil)
			So(tape.Read(t.Context(), catalog, run, func(Leg) error { return nil }), ShouldNotBeNil)
			So(tape.LastSequence, ShouldEqual, 1)
		})
	})
}

func TestTapeMacroLeg(t *testing.T) {
	Convey("A leg survives pullbacks and reports its whole excursion", t, func() {
		var tape Tape
		var completed []Leg
		at := time.Unix(100, 0)
		emit := func(index int, price int64) {
			payload, err := json.Marshal(kraken.Trade{Data: []kraken.TradeData{{
				Symbol: "BTC/USD", Price: *decimal.NewFromInt64(price),
			}}})
			So(err, ShouldBeNil)
			So(tape.Step(
				RawFrame{Kind: "trade", ReceivedAt: at.Add(time.Duration(index) * time.Second), Payload: payload},
				func(leg Leg) error { completed = append(completed, leg); return nil },
			), ShouldBeNil)
		}

		// A breakout to 200 interrupted twice. Neither dip gives back half the
		// excursion accumulated at that point, so neither ends the leg.
		for index, price := range []int64{100, 130, 120, 170, 155, 200} {
			emit(index, price)
		}
		So(completed, ShouldBeEmpty)

		Convey("A retracement of half the excursion confirms it", func() {
			emit(6, 145)
			So(completed, ShouldHaveLength, 1)
			leg := completed[0]
			So(leg.Start.Cmp(decimal.NewFromInt64(100)), ShouldEqual, 0)
			So(leg.End.Cmp(decimal.NewFromInt64(200)), ShouldEqual, 0)
			So(leg.Through, ShouldEqual, at.Add(5*time.Second))
			So(leg.ConfirmedAt, ShouldEqual, at.Add(6*time.Second))

			Convey("The confirmed extremum becomes the next leg's origin", func() {
				emit(7, 100)
				So(completed, ShouldHaveLength, 1) // The reversal is still developing.
				emit(8, 155)
				So(completed, ShouldHaveLength, 2)
				So(completed[1].Start.Cmp(decimal.NewFromInt64(200)), ShouldEqual, 0)
				So(completed[1].End.Cmp(decimal.NewFromInt64(100)), ShouldEqual, 0)
			})
		})
	})
}
