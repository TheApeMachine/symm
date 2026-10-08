package statistic

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/transport"
)

func TestNewPriorMoments(t *testing.T) {
	Convey("Given a stream of reliability-weighted observations", t, func() {
		observations := []PriorObservation{
			{Value: 2, Authority: 1, Memory: 1, Epoch: 1, HasEpoch: true},
			{Value: 4, Authority: 1, Memory: 1, Epoch: 2, HasEpoch: true},
			{Value: 100, Authority: 0, Memory: 1},
			{Value: 6, Authority: 1, Memory: 1, Epoch: 3, HasEpoch: true},
		}

		summaries := collectReadings[PriorObservation, PriorSummary](t, NewPriorMoments(), observations)
		So(len(summaries), ShouldEqual, 4)

		Convey("zero-authority completions count as samples without evidence", func() {
			So(summaries[2].Samples, ShouldEqual, 3)
			So(summaries[2].Defined, ShouldBeTrue)
		})

		Convey("the weighted mean tracks the observed values", func() {
			So(summaries[3].Mean, ShouldAlmostEqual, 4, 1e-9)
		})

		Convey("two distinct equal-weight observations give their sample variance", func() {
			So(summaries[1].VarianceDefined, ShouldBeTrue)
			So(summaries[1].Variance, ShouldAlmostEqual, 2)
			So(summaries[1].Maturity, ShouldAlmostEqual, 0.5)
		})
	})
}

func TestNewPriorMomentsDomain(t *testing.T) {
	Convey("Given an authority outside [0, 1]", t, func() {
		operation := NewPriorMoments()
		yielded := 0

		for range operation.Next(transport.NewValues(PriorObservation{Value: 1, Authority: 1.5, Memory: 1}).Next(nil)) {
			yielded++
		}

		Convey("the run stops and records the domain violation", func() {
			So(yielded, ShouldEqual, 0)
			So(operation.Error(), ShouldNotBeNil)
		})
	})
}

func TestNewPriorMomentsAgeOnly(t *testing.T) {
	Convey("Given age-only queries against an empty prior", t, func() {
		queries := []PriorObservation{
			{Memory: 1, Epoch: 3, HasEpoch: true, AgeOnly: true},
			{Memory: 1, Epoch: 3, HasEpoch: true, AgeOnly: true},
		}

		aged := collectReadings[PriorObservation, PriorSummary](t, NewPriorMoments(), queries)

		Convey("no sample is counted and no evidence is invented", func() {
			So(aged[0].Samples, ShouldEqual, 0)
			So(aged[1].Samples, ShouldEqual, 0)
			So(aged[1].Defined, ShouldBeFalse)
		})
	})
}

func TestNewPriorMomentsAging(t *testing.T) {
	Convey("Given a memory of 4 and a 4-epoch gap", t, func() {
		operation := NewPriorMoments()
		steps := []PriorObservation{
			{Value: 2, Authority: 1, Memory: 4, Epoch: 10, HasEpoch: true},
			{Value: 2, Authority: 1, Memory: 4, Epoch: 14, HasEpoch: true},
		}

		summaries := collectReadings[PriorObservation, PriorSummary](t, operation, steps)
		weight := summaries[1].EvidenceAuthority * summaries[1].Support

		Convey("total weight is the decayed prior plus the new authority", func() {
			decayed := math.Exp(4 * math.Log(1-1.0/4))
			So(weight, ShouldAlmostEqual, decayed+1, 1e-9)
		})
	})
}
