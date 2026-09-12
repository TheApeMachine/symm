package prior_test

import (
	"fmt"
	"math/rand"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/learning/associative/prior"
	"github.com/theapemachine/symm/nomagique/statistic"
	"github.com/theapemachine/symm/nomagique/transport"
)

/*
canonicalSummary drives the canonical prior Primitive with one observation and
returns the summary it yields, as the reference for the composed wrapper.
*/
func canonicalSummary(
	t *testing.T,
	node core.Primitive,
	request statistic.PriorObservation,
) statistic.PriorSummary {
	t.Helper()

	summaryEval := transport.NewEvaluate(node)
	var summary statistic.PriorSummary

	for out := range summaryEval.Next(transport.NewValues(request).Next(nil)) {
		summary = *(*statistic.PriorSummary)(out)
	}

	err := summaryEval.Error()

	if err != nil {
		t.Fatalf("canonical prior evaluation: %v", err)
	}

	return summary
}

func TestPrimitiveNext(t *testing.T) {
	Convey("Updates and read-only aging match the canonical prior Primitive", t, func() {
		for _, memory := range []float64{0, 10} {
			Convey(fmt.Sprintf("memory %g", memory), func() {
				node := prior.New(memory)
				reference := statistic.NewPriorMoments()
				random := rand.New(rand.NewSource(83))

				for index := 0; index < 400; index++ {
					value := random.NormFloat64()
					authority := random.Float64()

					if index%11 == 0 {
						authority = 0
					}

					observation := prior.Observation{Value: value, Authority: authority, HasValue: true}
					request := statistic.PriorObservation{Value: value, Authority: authority, Memory: memory}

					if index < 200 {
						observation.Epoch = uint64(index + 1)
						observation.HasEpoch = true
						request.Epoch = observation.Epoch
						request.HasEpoch = true
					}

					gotEval := transport.NewEvaluate(node)
					var got prior.Reading

					for out := range gotEval.Next(transport.NewValues(observation).Next(nil)) {
						got = *(*prior.Reading)(out)
					}

					err := gotEval.Error()
					So(err, ShouldBeNil)
					want := canonicalSummary(t, reference, request)
					So(got.Mean, ShouldEqual, want.Mean)
					So(got.Variance, ShouldEqual, want.Variance)
					So(got.Support, ShouldEqual, want.Support)
					So(got.Maturity, ShouldEqual, want.Maturity)
					So(got.EvidenceAuthority, ShouldEqual, want.EvidenceAuthority)
					So(got.Authority, ShouldEqual, want.Authority)
					So(got.Memory, ShouldEqual, want.Memory)
					So(got.Defined, ShouldEqual, want.Defined)
					So(got.VarianceDefined, ShouldEqual, want.VarianceDefined)
					So(got.Samples, ShouldEqual, want.Samples)
				}

				for _, epoch := range []uint64{200, 300, 1000000, 1000000} {
					gotEval := transport.NewEvaluate(node)
					var got prior.Reading

					for out := range gotEval.Next(transport.NewValues(prior.Observation{Epoch: epoch, HasEpoch: true}).Next(nil)) {
						got = *(*prior.Reading)(out)
					}

					err := gotEval.Error()
					So(err, ShouldBeNil)
					want := canonicalSummary(t, reference, statistic.PriorObservation{
						Memory: memory, Epoch: epoch, HasEpoch: true, AgeOnly: true,
					})
					So(got.Mean, ShouldEqual, want.Mean)
					So(got.Support, ShouldEqual, want.Support)
					So(got.Samples, ShouldEqual, want.Samples)
				}

				final := statistic.PriorObservation{
					Value: 7, Authority: 0.5, Memory: memory, Epoch: 1000001, HasEpoch: true,
				}
				gotEval := transport.NewEvaluate(node)
				var got prior.Reading

				for out := range gotEval.Next(transport.NewValues(prior.Observation{
					Value: 7, Authority: 0.5, HasValue: true, Epoch: 1000001, HasEpoch: true,
				}).Next(nil)) {
					got = *(*prior.Reading)(out)
				}

				err := gotEval.Error()
				So(err, ShouldBeNil)
				So(got.Mean, ShouldEqual, canonicalSummary(t, reference, final).Mean)
			})
		}
	})
}
