package resonance

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/learning"
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
			So(history.lastResolution.error, ShouldEqual, 0)
			_, retained := history.issued[4]
			So(retained, ShouldBeTrue)
			_, pending := history.pending[2]
			So(pending, ShouldBeFalse)
		})
	})

	Convey("Given a train step whose realized move is flat", t, func() {
		history := &sampleHistory{
			issued: map[int64]issuedTask{
				4: {features: []float64{0.2}, prediction: []float64{0.3}},
			},
			pending: map[int64][]issuedHorizon{
				2: {{
					horizon: 1, forecast: 0.3, mark: 100,
					issueTick: 4, train: true,
				}},
			},
			ledger:   newHorizonLedger(),
			sequence: 2,
		}

		Convey("It should drop the prior without teaching a zero-direction target", func() {
			So(history.resolve(nil, 100), ShouldBeNil)
			So(history.ledger.resolved, ShouldEqual, 0)
			_, retained := history.issued[4]
			So(retained, ShouldBeFalse)
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

		Convey("It should stand the same signed call behind every live probe horizon", func() {
			err := history.issue(&learning.ResonanceManifold{}, 9, 110, 2, forecast)
			So(err, ShouldBeNil)
			So(history.issued[9].prediction, ShouldResemble, []float64{1})
			So(history.pending[4], ShouldHaveLength, 1)
			So(history.pending[4][0].train, ShouldBeTrue)
			So(history.pending[5], ShouldHaveLength, 1)
			So(history.pending[5][0].horizon, ShouldEqual, 2)
			So(history.pending[5][0].forecast, ShouldEqual, 1)
			So(history.pending[5][0].train, ShouldBeTrue)
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

func TestObserveTickMove(t *testing.T) {
	Convey("Given successive marks on one book", t, func() {
		history := &sampleHistory{}

		Convey("It should ignore the first mark and measure later log moves", func() {
			So(history.observeTickMove(100), ShouldBeNil)
			So(history.moveScale(), ShouldEqual, 0)
			So(history.observeTickMove(101), ShouldBeNil)
			So(history.moveScale(), ShouldBeGreaterThan, 0)
			So(history.observeTickMove(101), ShouldBeNil)
			So(history.moveStat.Count, ShouldEqual, 1)
		})
	})
}

func TestDistinguishable(t *testing.T) {
	Convey("Given a book whose typical tick move is already known", t, func() {
		history := &sampleHistory{}
		So(history.observeTickMove(100), ShouldBeNil)
		So(history.observeTickMove(101), ShouldBeNil)
		scale := history.moveScale()

		Convey("It should reject a move inside that scale and accept one that matches it", func() {
			So(history.distinguishable(scale/2), ShouldBeFalse)
			So(history.distinguishable(scale), ShouldBeTrue)
		})
	})
}

func TestInFlight(t *testing.T) {
	Convey("Given an empty history", t, func() {
		history := &sampleHistory{}

		Convey("It should report idle until a train prior is outstanding", func() {
			So(history.inFlight(), ShouldBeFalse)
			history.pending = map[int64][]issuedHorizon{2: {{horizon: 2}}}
			So(history.inFlight(), ShouldBeFalse)
			history.issued = map[int64]issuedTask{1: {}}
			So(history.inFlight(), ShouldBeTrue)
		})
	})
}
