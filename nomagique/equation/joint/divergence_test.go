package joint_test

import (
	"errors"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/algo"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/equation/joint"
	"github.com/theapemachine/symm/nomagique/equation/linear"
	"github.com/theapemachine/symm/nomagique/tests"
	"github.com/theapemachine/symm/nomagique/transport"
)

func TestNewDivergence(t *testing.T) {
	Convey("Given two independently configured log-space channels", t, func() {
		left := algo.NewWelford()
		divergence := joint.NewDivergence(
			transport.NewIO(left, algo.NewWelford()),
			transport.NewIO(linear.NewLocalRegression(), linear.NewLocalRegression()),
		)

		Convey("opposing trends retain independent residuals and velocity units", func() {
			var fields map[string]core.Primitive

			for index := range 4 {
				output := tests.Drain(t, divergence, transport.NewIO(tests.Record(map[string]any{
					"values": []float64{float64(index), -2 * float64(index)},
					"at":     int64(index) * 1e9,
				})))
				So(divergence.Error(), ShouldBeNil)
				So(output, ShouldHaveLength, 1)
				fields = tests.Fields(t, output[0])
			}

			channels := core.To[[]core.Primitive](fields["channels"])
			velocities := core.To[[]core.Primitive](fields["velocities"])
			So(channels, ShouldHaveLength, 2)
			So(velocities, ShouldHaveLength, 2)

			for index, scale := range []float64{1, -2} {
				channel := core.To[map[string]core.Primitive](channels[index])
				velocity := core.To[map[string]core.Primitive](velocities[index])
				So(tests.Number(t, channel, "residual"), ShouldAlmostEqual, 2*scale)
				// OLS on residuals [0, 1, 1.5, 2] at seconds [0, 1, 2, 3]
				// has centered cross-product 3.25 and time energy 5.
				So(tests.Number(t, velocity, "slope"), ShouldAlmostEqual, (3.25/5)*scale)
				So(core.To[bool](velocity["slope_defined"]), ShouldBeTrue)
			}
		})

		Convey("regressing event time fails explicitly", func() {
			for index, timestamp := range []int64{2e9, 1e9} {
				output := tests.Drain(t, divergence, transport.NewIO(tests.Record(map[string]any{
					"values": []float64{1, -2}, "at": timestamp,
				})))
				So(output, ShouldHaveLength, 1-index)
				So(tests.Number(t, tests.Fields(t, left.Read()), "count"), ShouldEqual, 1)
			}

			So(errors.Is(divergence.Error(), core.ErrDomain), ShouldBeTrue)
		})
	})
}

func BenchmarkNewDivergence(b *testing.B) {
	divergence := joint.NewDivergence(
		transport.NewIO(algo.NewWelford(), algo.NewWelford()),
		transport.NewIO(linear.NewLocalRegression(), linear.NewLocalRegression()),
	)
	var timestamp int64
	b.ReportAllocs()

	for b.Loop() {
		input := transport.NewIO(tests.Record(map[string]any{
			"values": []float64{1, -2}, "at": timestamp,
		}))

		if divergence.Next(input) == nil || divergence.Next(input) != nil {
			b.Fatal("expected one joint divergence record")
		}

		timestamp += 1e9
	}

	if err := divergence.Error(); err != nil {
		b.Fatal(err)
	}
}
