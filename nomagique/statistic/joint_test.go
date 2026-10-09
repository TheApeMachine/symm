package statistic

import (
	"errors"
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/tests"
)

func TestJointNext(t *testing.T) {
	Convey("Given a Joint primitive over two channels", t, func() {
		op := NewJoint(2)

		Convey("tracks per-channel residuals and the average energy", func() {
			vectors := [][]float64{
				{math.Log(10), math.Log(100)},
				{math.Log(20), math.Log(100)},
				{math.Log(40), math.Log(100)},
			}
			var readings []JointReading

			for _, vector := range vectors {
				for _, reading := range tests.CollectSeq[JointReading](op.Next(tests.SliceToSeq([]JointInput{{Values: vector}}))) {
					reading.Channels = append([]CausalResidualResult(nil), reading.Channels...)
					reading.Energies = append([]float64(nil), reading.Energies...)
					readings = append(readings, reading)
				}
			}

			So(op.Error(), ShouldBeNil)
			So(len(readings), ShouldEqual, 3)
			So(len(readings[0].Channels), ShouldEqual, 2)

			first := readings[0]
			So(first.SNRDefined, ShouldBeFalse)
			So(first.Channels[0].Count, ShouldEqual, 1)
			So(first.Channels[0].HasPrior, ShouldBeFalse)

			last := readings[2]
			channel := last.Channels[0]
			So(channel.Count, ShouldEqual, 3)
			So(channel.HasPrior, ShouldBeTrue)
			So(channel.Baseline, ShouldAlmostEqual, math.Sqrt(200), 1e-9)
			So(channel.PriorVariance, ShouldAlmostEqual, math.Pow(math.Log(2), 2)/2, 1e-12)

			// The flat channel has no prior dispersion: no energy, so the SNR
			// averages the moving channel's energy alone.
			So(len(last.Energies), ShouldEqual, 1)
			So(last.Energies[0], ShouldAlmostEqual, channel.ZScore*channel.ZScore, 1e-12)
			So(last.SNRDefined, ShouldBeTrue)
			So(last.SNR, ShouldAlmostEqual, last.Energies[0], 1e-12)
		})

		Convey("a mismatched dimension records ErrShape", func() {
			fresh := NewJoint(2)
			out := tests.CollectSeq[JointReading](fresh.Next(tests.SliceToSeq([]JointInput{{Values: []float64{1}}})))

			So(len(out), ShouldEqual, 0)
			So(errors.Is(fresh.Error(), core.ErrShape), ShouldBeTrue)
		})
	})
}
