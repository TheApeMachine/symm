package audit

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/data"
)

func TestDynamicsStage(t *testing.T) {
	Convey("Given a DynamicsStage", t, func() {
		stage := NewDynamicsStage(context.Background())

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

		Convey("Two sources lighting in turn give a structured sequence that passes", func() {
			values := make([]*data.Value, 0, 60)

			for tick := uint64(1); tick <= 60; tick++ {
				values = append(values, signalJoin(tick, "BTC/USD", []fixtureSignal{
					{"cvd", "signed_net_fraction", oscillating(tick, false)},
					{"liquidity", "relative_spread", oscillating(tick, true)},
				}))
			}

			violations, err := feed(values...)
			So(err, ShouldBeNil)
			So(violations, ShouldBeEmpty)

			stats := stage.Data().(map[string]any)
			So(stats["distinct_tokens"], ShouldEqual, 2)
			So(stats["entropy"], ShouldEqual, 0.0)
			So(stats["null_transition_entropy"].(float64), ShouldBeGreaterThan, 0.0)
			So(stats["null_p_value"].(float64), ShouldBeLessThan, 0.05)
		})

		Convey("One source lighting alone fails absorbing collapse", func() {
			values := make([]*data.Value, 0, 60)

			for tick := uint64(1); tick <= 60; tick++ {
				values = append(values, regimeJoin(tick, true))
			}

			violations, err := feed(values...)
			So(err, ShouldBeNil)
			So(hasViolation(violations, "dynamics.absorbing_token_collapse"), ShouldBeTrue)
		})

		Convey("Too few tokens fail as insufficient evidence", func() {
			violations, err := feed(regimeJoin(1, true), regimeJoin(2, true), regimeJoin(3, true))
			So(err, ShouldBeNil)
			So(hasViolation(violations, "dynamics.insufficient_evidence"), ShouldBeTrue)
		})

		Convey("A frame carries the touch midpoint only when the tick recorded one", func() {
			signals := func(tick uint64) []fixtureSignal {
				return []fixtureSignal{{"cvd", "signed_net_fraction", oscillating(tick, false)}}
			}

			_, err := feed(
				signalJoin(1, "BTC/USD", signals(1), 50000, 50010),
				signalJoin(2, "BTC/USD", signals(2), 50000, 50010),
				signalJoin(3, "BTC/USD", signals(3), 50000, 50010),
				signalJoin(4, "BTC/USD", signals(4)),
			)
			So(err, ShouldBeNil)

			frames := stage.Data().(map[string]any)["frames"].([]*FrameData)
			So(len(frames), ShouldEqual, 2)
			So(frames[0].Tick, ShouldEqual, 3)
			So(*frames[0].Price, ShouldEqual, 50005.0)
			So(len(frames[0].Brightness), ShouldEqual, 15)
			So(frames[1].Price, ShouldBeNil)
		})

		Convey("A nil measurement is an error", func() {
			_, err := feed(nil)
			So(err, ShouldNotBeNil)
		})
	})
}
