package prior_test

import (
	"fmt"
	"math/rand"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/learning/associative/prior"
	"github.com/theapemachine/symm/nomagique/transport"
)

func TestPrimitiveNext(t *testing.T) {
	Convey("Updates and read-only aging match the canonical prior recurrence", t, func() {
		for _, memory := range []float64{0, 10} {
			Convey(fmt.Sprintf("memory %g", memory), func() {
				node := prior.New(memory)
				reference := equation.PriorMoments{}
				random := rand.New(rand.NewSource(83))

				for index := 0; index < 400; index++ {
					value := random.NormFloat64()
					authority := random.Float64()

					if index%11 == 0 {
						authority = 0
					}

					observation := prior.Observation{Value: value, Authority: authority, HasValue: true}

					if index < 200 {
						observation.Epoch = uint64(index + 1)
						observation.HasEpoch = true
						So(reference.Observe(value, authority, memory, observation.Epoch), ShouldBeNil)
					}

					if index >= 200 {
						So(reference.Observe(value, authority, memory), ShouldBeNil)
					}

					got, err := transport.Evaluate(node, transport.Values(observation))
					So(err, ShouldBeNil)
					want := reference.Summary(memory)
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
					reference.Age(epoch, memory)
					got, err := transport.Evaluate(node, transport.Values(prior.Observation{Epoch: epoch, HasEpoch: true}))
					So(err, ShouldBeNil)
					want := reference.Summary(memory)
					So(got.Mean, ShouldEqual, want.Mean)
					So(got.Support, ShouldEqual, want.Support)
					So(got.Samples, ShouldEqual, want.Samples)
				}

				So(reference.Observe(7, 0.5, memory, 1000001), ShouldBeNil)
				got, err := transport.Evaluate(node, transport.Values(prior.Observation{
					Value: 7, Authority: 0.5, HasValue: true, Epoch: 1000001, HasEpoch: true,
				}))
				So(err, ShouldBeNil)
				So(got.Mean, ShouldEqual, reference.Summary(memory).Mean)
			})
		}
	})
}
