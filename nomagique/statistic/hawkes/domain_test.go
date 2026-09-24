package hawkes_test

import (
	"context"
	"math"
	"sort"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/statistic/hawkes"
)

/*
region is the search domain the node derives from a window.
*/
type region struct {
	lower   []float64
	upper   []float64
	seed    []float64
	defined bool
}

/*
domainOf drives the Domain node over one window.
*/
func domainOf(t *testing.T, path realisation) region {
	return domainFor(t, path, 1)
}

/*
domainFor drives the Domain node, choosing whether the region describes a
process whose components may excite each other.
*/
func domainFor(t *testing.T, path realisation, coupled float64) region {
	t.Helper()
	ctx := context.Background()
	client := hawkes.Domain_ServerToClient(hawkes.NewDomain())

	err := client.Write(ctx, func(params hawkes.Domain_write_Params) error {
		if err := writeFloats(params.NewTimes, path.times); err != nil {
			return err
		}

		if err := writeFloats(params.NewComponents, path.components); err != nil {
			return err
		}

		params.SetOrigin(path.origin)
		params.SetHorizon(path.horizon)
		params.SetDimension(int32(path.dimension))
		params.SetCoupled(coupled)
		return nil
	})

	if err != nil {
		t.Fatalf("domain write: %v", err)
	}

	if err := client.WaitStreaming(); err != nil {
		t.Fatalf("domain stream: %v", err)
	}

	future, release := client.Done(ctx, nil)
	defer release()
	results, err := future.Struct()

	if err != nil {
		t.Fatalf("domain done: %v", err)
	}

	lower, err := results.Lower()

	if err != nil {
		t.Fatalf("domain lower: %v", err)
	}

	upper, err := results.Upper()

	if err != nil {
		t.Fatalf("domain upper: %v", err)
	}

	seed, err := results.Seed()

	if err != nil {
		t.Fatalf("domain seed: %v", err)
	}

	return region{
		lower:   readList(lower.Len(), lower.At),
		upper:   readList(upper.Len(), upper.At),
		seed:    readList(seed.Len(), seed.At),
		defined: results.Defined(),
	}
}

func TestDomainServer_Write(t *testing.T) {
	Convey("Given a simulated window", t, func() {
		baseline := []float64{0.5, 0.3}
		excitation := []float64{0.6, 0.2, 0.3, 0.5}
		decay := 1.5
		path := simulate(2, baseline, excitation, decay, 600, 20260922)
		width := 2 + 4 + 1

		Convey("When the search region is derived", func() {
			derived := domainOf(t, path)

			Convey("Then it is defined and covers every parameter", func() {
				So(derived.defined, ShouldBeTrue)
				So(len(derived.lower), ShouldEqual, width)
				So(len(derived.upper), ShouldEqual, width)
				So(len(derived.seed), ShouldEqual, width)
			})

			Convey("Then every bound is ordered", func() {
				for index := 0; index < width; index++ {
					So(derived.lower[index], ShouldBeLessThan, derived.upper[index])
				}
			})

			Convey("Then the seed is stated in the coordinates a search moves in", func() {
				// The seed is not a parameter but the coordinate that maps
				// onto one. Handing a search the parameter instead would
				// start it on a different process entirely, and silently:
				// both are just numbers of the right count.
				start := mapCoordinates(t, derived.seed, derived.lower, derived.upper, 2)

				for index, value := range flatten(start, 2) {
					So(value, ShouldBeGreaterThan, math.Exp(derived.lower[index]))
					So(value, ShouldBeLessThan, math.Exp(derived.upper[index]))
				}
			})

			Convey("Then the decay range is the one the window can resolve", func() {
				// Slower than the window and the decay is never seen
				// decaying; faster than the arrivals are spaced and it is
				// not resolved by them. Both ends come from the data.
				span := path.horizon - path.origin
				So(math.Abs(math.Exp(derived.lower[width-1])-1/span), ShouldBeLessThan, 1e-12)

				gaps := 0

				for index := 1; index < len(path.times); index++ {
					if path.times[index]-path.times[index-1] > 0 {
						gaps++
					}
				}

				So(gaps, ShouldBeGreaterThan, 0)
				So(math.Exp(derived.upper[width-1]), ShouldBeGreaterThan, math.Exp(derived.lower[width-1]))
			})

			Convey("Then the truth the window was drawn from lies inside the region", func() {
				// A region that excluded the generating parameters would
				// make the fit unable to find them however well it searched.
				So(math.Log(baseline[0]), ShouldBeBetween, derived.lower[0], derived.upper[0])
				So(math.Log(baseline[1]), ShouldBeBetween, derived.lower[1], derived.upper[1])
				So(math.Log(decay), ShouldBeBetween, derived.lower[width-1], derived.upper[width-1])

				for index := range excitation {
					So(math.Log(excitation[index]), ShouldBeBetween, derived.lower[2+index], derived.upper[2+index])
				}
			})

			Convey("Then the seed decodes to the process the window itself suggests", func() {
				start := mapCoordinates(t, derived.seed, derived.lower, derived.upper, 2)
				counted := make([]float64, 2)
				gaps := make([]float64, 0, len(path.times))

				for index, component := range path.components {
					if path.times[index] > path.origin {
						counted[int(component)]++
					}

					if index > 0 && path.times[index]-path.times[index-1] > 0 {
						gaps = append(gaps, path.times[index]-path.times[index-1])
					}
				}

				span := path.horizon - path.origin
				sort.Float64s(gaps)

				Convey("with each component arriving at its own observed rate", func() {
					for component := 0; component < 2; component++ {
						So(math.Abs(start.baseline[component]-counted[component]/span), ShouldBeLessThan, 1e-6)
					}
				})

				Convey("decaying on the timescale the middle of the gaps sets", func() {
					// The timescale is the reciprocal of a gap drawn from
					// the middle of the observed ones. Bracketing it rather
					// than naming one pins the property without restating
					// which quantile convention was used to pick it.
					middle := len(gaps) / 2
					So(1/start.decay, ShouldBeBetween, gaps[middle-1], gaps[middle+1])
				})

				Convey("and excitation halfway to the point cascades stop dying out", func() {
					branching := branchingOf(t, start.excitation, start.decay, 2)
					So(math.Abs(radiusOf(t, branching, 2)-0.5), ShouldBeLessThan, 1e-6)
				})
			})
		})

		Convey("When the window holds too few arrivals to state a scale", func() {
			derived := domainOf(t, realisation{
				times:      []float64{2},
				components: []float64{0},
				dimension:  2,
				origin:     2,
				horizon:    2,
			})

			Convey("Then no region is derived rather than an invented one", func() {
				So(derived.defined, ShouldBeFalse)
			})
		})

		Convey("When every arrival is simultaneous", func() {
			derived := domainOf(t, realisation{
				times:      []float64{5, 5, 5},
				components: []float64{0, 1, 0},
				dimension:  2,
				origin:     4,
				horizon:    6,
			})

			Convey("Then there is no gap to state a decay scale in", func() {
				So(derived.defined, ShouldBeFalse)
			})
		})

		Convey("When the seed falls on the upper bounds themselves", func() {
			// One component makes every arrival, so its observed rate is
			// the highest the window allows, and evenly spaced arrivals put
			// the median gap on the quickest one, so the decay seed is the
			// fastest resolvable decay too.
			derived := domainOf(t, realisation{
				times:      []float64{1, 2, 3, 4, 5, 6, 7, 8},
				components: []float64{0, 0, 0, 0, 0, 0, 0, 0},
				dimension:  2,
				origin:     0,
				horizon:    8,
			})

			Convey("Then its coordinates are drawn just inside them, not infinite", func() {
				So(derived.defined, ShouldBeTrue)

				start := mapCoordinates(t, derived.seed, derived.lower, derived.upper, 2)
				So(start.baseline[0], ShouldBeLessThan, math.Exp(derived.upper[0]))
				So(start.baseline[0], ShouldAlmostEqual, math.Exp(derived.upper[0]), 1e-6)
				So(start.decay, ShouldAlmostEqual, math.Exp(derived.upper[width-1]), 1e-6)

				branching := branchingOf(t, start.excitation, start.decay, 2)
				So(radiusOf(t, branching, 2), ShouldAlmostEqual, 0.5, 1e-6)
			})
		})

		Convey("When the window has no extent", func() {
			derived := domainOf(t, realisation{
				times:      []float64{1, 2},
				components: []float64{0, 1},
				dimension:  2,
				origin:     3,
				horizon:    3,
			})

			Convey("Then no rate can be stated and no region is derived", func() {
				So(derived.defined, ShouldBeFalse)
			})
		})
	})
}
