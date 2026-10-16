package audit

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/data"
)

func TestPrecursorStage(t *testing.T) {
	Convey("Given a PrecursorStage", t, func() {
		stage := NewPrecursorStage(context.Background())

		feed := func(values ...*data.Value) ([]*Violation, error) {
			for _, value := range values {
				stage.Channel() <- value
			}

			close(stage.Channel())
			err := stage.Run()
			return stage.Violations(), err
		}

		episode := func(bAsk, cBid float64) *data.Value {
			return excursionValue("up", "BTC/USD", map[string]float64{
				"a_tick": 1, "b_tick": 20, "c_tick": 30, "d_tick": 40,
				"b_price": bAsk, "c_price": cBid, "b_ask": bAsk, "c_bid": cBid,
			})
		}

		tape := func(cvdLit func(tick uint64) bool) []*data.Value {
			values := make([]*data.Value, 0, 60)

			for tick := uint64(1); tick <= 60; tick++ {
				values = append(values, regimeJoin(tick, cvdLit(tick)))
			}

			return values
		}

		Convey("A distinct precursor and a profitable excursion pass", func() {
			violations, err := feed(append(tape(func(tick uint64) bool { return tick <= 20 }), episode(100, 108))...)
			So(err, ShouldBeNil)
			So(violations, ShouldBeEmpty)
			So(stage.Data().(map[string]any)["clearance_rate"], ShouldEqual, 1.0)
		})

		Convey("A precursor like its background fails no-divergence", func() {
			violations, err := feed(append(tape(func(uint64) bool { return true }), episode(100, 108))...)
			So(err, ShouldBeNil)
			So(hasViolation(violations, "precursor.no_divergence"), ShouldBeTrue)
		})

		Convey("An excursion eaten by spread and fees fails friction clearance", func() {
			violations, err := feed(append(tape(func(tick uint64) bool { return tick <= 20 }), episode(100.1, 99.9))...)
			So(err, ShouldBeNil)
			So(hasViolation(violations, "precursor.unprofitable_friction"), ShouldBeTrue)
		})

		Convey("No detected excursions fail as insufficient evidence", func() {
			violations, err := feed(tape(func(uint64) bool { return true })...)
			So(err, ShouldBeNil)
			So(hasViolation(violations, "excursion.insufficient_evidence"), ShouldBeTrue)
			So(hasViolation(violations, "precursor.insufficient_evidence"), ShouldBeTrue)
		})

		Convey("An excursion missing its touch is an error", func() {
			_, err := feed(excursionValue("up", "BTC/USD", map[string]float64{"b_ask": 0, "c_bid": 1}))
			So(err, ShouldNotBeNil)
		})

		Convey("Anything that is neither a join nor an excursion is an error", func() {
			_, err := feed(data.NewValue("test", "tick", "BTC/USD"))
			So(err, ShouldNotBeNil)
		})

		Convey("A nil measurement is an error", func() {
			_, err := feed(nil)
			So(err, ShouldNotBeNil)
		})
	})
}
