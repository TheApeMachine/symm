package causal

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestOpportunityRunway(t *testing.T) {
	Convey("Given excess velocity versus history", t, func() {
		samples := ladderTrainingSamples(minCausalHistory + 4)
		samples[len(samples)-1] = newCausalSample(0.2, 80, 2, 0.05)

		runway := opportunityRunway(samples, 2*time.Second)

		Convey("It should compress runway when speed exceeds typical", func() {
			So(runway, ShouldBeLessThan, 2*time.Second)
			So(runway, ShouldBeGreaterThan, 0)
		})
	})

	Convey("Given zero elapsed time", t, func() {
		Convey("It should return zero runway", func() {
			So(opportunityRunway(ladderTrainingSamples(4), 0), ShouldEqual, 0)
		})
	})
}

func BenchmarkOpportunityRunway(b *testing.B) {
	samples := ladderTrainingSamples(minCausalHistory + 8)

	b.ReportAllocs()

	for b.Loop() {
		_ = opportunityRunway(samples, time.Second)
	}
}
