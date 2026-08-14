package resonance

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/nomagique/learning"
)

func TestResolve(t *testing.T) {
	Convey("Given a pending longer-horizon call and a later mark", t, func() {
		history := &sampleHistory{
			issued: map[int64]issuedTask{
				4: {features: []float64{0.2}, prediction: []float64{0.3}},
			},
			pending: map[int64][]issuedHorizon{
				2: {{
					horizon: 2, forecast: 0.3, mark: 100,
					issueTick: 4, train: false,
				}},
			},
			ledger:   newHorizonLedger(),
			sequence: 2,
		}

		Convey("It should score the realized move without inventing a train step", func() {
			So(history.resolve(nil, 101), ShouldBeNil)
			So(history.ledger.resolved, ShouldEqual, 1)
			So(history.lastResolution, ShouldNotBeNil)
			So(history.lastResolution.horizon, ShouldEqual, 2)
			_, retained := history.issued[4]
			So(retained, ShouldBeTrue)
			_, pending := history.pending[2]
			So(pending, ShouldBeFalse)
		})
	})
}

func TestIssue(t *testing.T) {
	Convey("Given a ready one-step lean", t, func() {
		history := &sampleHistory{
			issued:   map[int64]issuedTask{},
			pending:  map[int64][]issuedHorizon{},
			sequence: 3,
		}
		forecast := []learning.RLSOutput{{
			Value: 0.2, Scale: 0.05, DegreesOfFreedom: 4, Ready: true,
		}, {
			Value: 0.1, Scale: 0.05, DegreesOfFreedom: 4, Ready: true,
		}}

		Convey("It should stand the same call behind every live probe horizon", func() {
			err := history.issue(&learning.ResonanceManifold{}, 9, 110, 2, forecast)
			So(err, ShouldBeNil)
			So(history.issued[9].prediction, ShouldResemble, []float64{0.2})
			So(history.pending[4], ShouldHaveLength, 1)
			So(history.pending[4][0].train, ShouldBeTrue)
			So(history.pending[5], ShouldHaveLength, 1)
			So(history.pending[5][0].horizon, ShouldEqual, 2)
			So(history.pending[5][0].forecast, ShouldEqual, 0.2)
			So(history.pending[5][0].train, ShouldBeFalse)
		})
	})
}

func TestPruneTicks(t *testing.T) {
	Convey("Given an older tick whose issued prior is gone", t, func() {
		history := &sampleHistory{
			issued: map[int64]issuedTask{},
			marks:  map[int64]float64{1: 100, 2: 101},
			ticks:  []int64{1, 2},
		}

		Convey("It should drop the spent tick and keep the live one", func() {
			history.pruneTicks()
			So(history.ticks, ShouldResemble, []int64{2})
			_, retained := history.marks[1]
			So(retained, ShouldBeFalse)
		})
	})
}
