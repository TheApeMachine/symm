package audit

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/data"
)

func TestStabilityStage(t *testing.T) {
	Convey("Given a StabilityStage", t, func() {
		stage := NewStabilityStage(context.Background())

		feed := func(values ...*data.Value) ([]*Violation, error) {
			go func() {
				for _, value := range values {
					stage.Channel() <- value
				}

				close(stage.Channel())
			}()

			err := stage.Run()
			return stage.Violations(), err
		}

		regime := func(ticks uint64, cvdLit func(tick uint64) bool) []*data.Value {
			values := make([]*data.Value, 0, ticks)

			for tick := uint64(1); tick <= ticks; tick++ {
				values = append(values, regimeJoin(tick, cvdLit(tick)))
			}

			return values
		}

		Convey("One stationary regime passes", func() {
			violations, err := feed(regime(60, func(uint64) bool { return true })...)
			So(err, ShouldBeNil)
			So(violations, ShouldBeEmpty)

			stats := stage.Data().(map[string]any)
			So(stats["warming_frames"], ShouldEqual, int64(2))
		})

		Convey("A regime shift between the halves fails non-stationary", func() {
			violations, err := feed(regime(60, func(tick uint64) bool { return tick <= 30 })...)
			So(err, ShouldBeNil)
			So(hasViolation(violations, "grid.non_stationary"), ShouldBeTrue)
		})

		Convey("Frames that mostly light nothing fail incomplete", func() {
			values := make([]*data.Value, 0, 100)

			for tick := uint64(1); tick <= 100; tick++ {
				cvd := constantRaw

				if tick <= 25 {
					cvd = oscillating(tick, false)
				}

				values = append(values, signalJoin(tick, "BTC/USD", []fixtureSignal{{"cvd", "signed_net_fraction", cvd}}))
			}

			violations, err := feed(values...)
			So(err, ShouldBeNil)
			So(hasViolation(violations, "grid.incomplete"), ShouldBeTrue)
		})

		Convey("Too few frames fail as insufficient evidence", func() {
			violations, err := feed(regime(5, func(uint64) bool { return true })...)
			So(err, ShouldBeNil)
			So(hasViolation(violations, "grid.insufficient_evidence"), ShouldBeTrue)
		})

		Convey("An unpinned signal label is an error", func() {
			_, err := feed(signalJoin(1, "BTC/USD", []fixtureSignal{{"cvd", "not_a_pinned_label", 1}}))
			So(err, ShouldNotBeNil)
		})

		Convey("A nil measurement is an error", func() {
			_, err := feed(nil)
			So(err, ShouldNotBeNil)
		})
	})
}
