package learning

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestPriorObserve(t *testing.T) {
	Convey("Given equally reliable completed outcomes", t, func() {
		prior := &Prior{}

		for _, outcome := range []float64{2, 4, 6} {
			So(prior.Observe(outcome, 0.5), ShouldBeNil)
		}

		reading := prior.Reading()
		So(reading.Samples, ShouldEqual, 3)
		So(reading.Defined, ShouldBeTrue)
		So(reading.Mean, ShouldEqual, 4)
		So(reading.VarianceDefined, ShouldBeTrue)
		So(reading.Variance, ShouldEqual, 4)
		So(reading.Support, ShouldEqual, 3)
		So(reading.Maturity, ShouldAlmostEqual, 2.0/3)
		So(reading.Authority, ShouldAlmostEqual, 4.0/15)

		Convey("a later loss updates the existing summary", func() {
			So(prior.Observe(-4, 0.5), ShouldBeNil)
			So(prior.Reading().Samples, ShouldEqual, 4)
			So(prior.Reading().Mean, ShouldEqual, 2)
			So(prior.Reading().Variance, ShouldAlmostEqual, 56.0/3)
		})

		Convey("invalid authority leaves the entire estimate unchanged", func() {
			before := *prior

			for _, authority := range []float64{-0.1, 1.1} {
				So(prior.Observe(100, authority), ShouldNotBeNil)
				So(*prior, ShouldResemble, before)
			}
		})
	})

	Convey("Given outcomes with unequal observation authority", t, func() {
		prior, reverse := &Prior{}, &Prior{}
		// Weighted sum = 9, weight = 3/2, squared weight = 7/8.
		// Centered weighted sum of squares = 30; Bessel divisor = 11/12.
		values := [...]float64{-2, 4, 10}
		authorities := [...]float64{0.25, 0.5, 0.75}

		for index := range values {
			So(prior.Observe(values[index], authorities[index]), ShouldBeNil)
			other := len(values) - 1 - index
			So(reverse.Observe(values[other], authorities[other]), ShouldBeNil)
		}

		reading := prior.Reading()
		So(reading.Mean, ShouldAlmostEqual, 6)
		So(reading.Variance, ShouldAlmostEqual, 360.0/11)
		So(reading.Support, ShouldAlmostEqual, 18.0/7)
		So(reading.Maturity, ShouldAlmostEqual, 11.0/18)
		So(reverse.Reading().Mean, ShouldAlmostEqual, reading.Mean)
		So(reverse.Reading().Variance, ShouldAlmostEqual, reading.Variance)

		Convey("an outcome with no authority supplies no trusted mass", func() {
			So(prior.Observe(1000000, 0), ShouldBeNil)
			So(prior.Reading().Samples, ShouldEqual, 4)
			So(prior.Reading().Mean, ShouldEqual, reading.Mean)
			So(prior.Reading().Support, ShouldEqual, reading.Support)
		})
	})

	Convey("Given a large offset and small changes", t, func() {
		prior := &Prior{}

		for _, change := range []float64{2, 4, 6} {
			So(prior.Observe(1e12+change, 1), ShouldBeNil)
		}

		So(prior.Reading().Mean, ShouldEqual, 1e12+4)
		So(prior.Reading().Variance, ShouldAlmostEqual, 4)
	})
}

func TestPriorReading(t *testing.T) {
	Convey("Given missing, unsupported, or single-outcome evidence", t, func() {
		prior := &Prior{}
		So(prior.Reading(), ShouldResemble, PriorReading{})
		So(prior.Observe(5, 0), ShouldBeNil)
		So(prior.Reading().Samples, ShouldEqual, 1)
		So(prior.Reading().Defined, ShouldBeFalse)

		Convey("one fractional-weight zero is a mean without estimable variance", func() {
			So(prior.Observe(0, 0.3), ShouldBeNil)
			reading := prior.Reading()
			So(reading.Defined, ShouldBeTrue)
			So(reading.Mean, ShouldEqual, 0)
			So(reading.Support, ShouldEqual, 1)
			So(reading.VarianceDefined, ShouldBeFalse)
			So(reading.Maturity, ShouldEqual, 0)
			So(reading.Authority, ShouldEqual, 0)

			Convey("another zero establishes zero dispersion", func() {
				So(prior.Observe(0, 0.3), ShouldBeNil)
				So(prior.Reading().VarianceDefined, ShouldBeTrue)
				So(prior.Reading().Variance, ShouldEqual, 0)
				So(prior.Reading().Support, ShouldEqual, 2)
			})
		})
	})

	Convey("Given identical means with different outcome disagreement", t, func() {
		steady, dispersed, weak := &Prior{}, &Prior{}, &Prior{}

		for _, value := range []float64{-1, 1, 3} {
			So(steady.Observe(1, 1), ShouldBeNil)
			So(dispersed.Observe(value, 1), ShouldBeNil)
			So(weak.Observe(1, 0.1), ShouldBeNil)
		}

		before := *steady
		So(steady.Reading().Mean, ShouldEqual, dispersed.Reading().Mean)
		So(steady.Reading().Support, ShouldEqual, dispersed.Reading().Support)
		So(steady.Reading().Authority, ShouldBeGreaterThan, dispersed.Reading().Authority)
		So(weak.Reading().Mean, ShouldEqual, steady.Reading().Mean)
		So(weak.Reading().Support, ShouldAlmostEqual, steady.Reading().Support)
		So(weak.Reading().Authority, ShouldAlmostEqual, steady.Reading().Authority*0.1)
		So(*steady, ShouldResemble, before)
	})

	Convey("Given a prior with exponential memory", t, func() {
		prior := NewPrior(50)

		for range 100 {
			So(prior.Observe(10.0, 1.0), ShouldBeNil)
		}

		So(prior.Reading().Mean, ShouldAlmostEqual, 10.0)
		So(prior.Reading().Support, ShouldBeLessThan, 105)

		Convey("observing a new regime shifts the mean toward new outcomes", func() {
			for range 100 {
				So(prior.Observe(-10.0, 1.0), ShouldBeNil)
			}

			reading := prior.Reading()
			So(reading.Mean, ShouldBeLessThan, -5.0)
			So(reading.Support, ShouldBeLessThan, 105)
		})
	})
}

func BenchmarkPriorObserve(b *testing.B) {
	prior := &Prior{}
	values := [...]float64{-2, 4, 10, -8}
	index := 0
	b.ReportAllocs()

	for b.Loop() {
		if err := prior.Observe(values[index%len(values)], 0.75); err != nil {
			b.Fatal(err)
		}

		index++
		prior.Reading()
	}
}
