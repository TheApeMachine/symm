package utils

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestNormalizeRatio(t *testing.T) {
	Convey("Given finite normalization helpers", t, func() {
		Convey("When normalizing a ratio with a positive baseline", func() {
			So(NormalizeRatio(10, 5), ShouldEqual, 2)
		})

		Convey("When normalizing a ratio with a zero baseline", func() {
			So(NormalizeRatio(10, 0), ShouldEqual, 10)
		})

		Convey("When normalizing a ratio with a negative baseline", func() {
			So(NormalizeRatio(10, -5), ShouldEqual, 10)
		})
	})
}

func TestNormalizeDeviation(t *testing.T) {
	Convey("Given finite normalization helpers", t, func() {
		Convey("When normalizing a deviation with a positive baseline", func() {
			So(NormalizeDeviation(10, 5), ShouldEqual, 1)
		})

		Convey("When normalizing a deviation with a zero baseline", func() {
			So(NormalizeDeviation(10, 0), ShouldEqual, 10)
		})

		Convey("When normalizing a deviation with a negative baseline", func() {
			So(NormalizeDeviation(10, -5), ShouldEqual, 10)
		})
	})
}

func TestValidateFinite(t *testing.T) {
	Convey("Given finite validation helpers", t, func() {
		Convey("When validating a finite value", func() {
			So(ValidateFinite(10), ShouldBeTrue)
		})

		Convey("When validating a NaN value", func() {
			So(ValidateFinite(math.NaN()), ShouldBeFalse)
		})

		Convey("When validating an infinite value", func() {
			So(ValidateFinite(math.Inf(1)), ShouldBeFalse)
			So(ValidateFinite(math.Inf(-1)), ShouldBeFalse)
		})
	})
}

func TestValidatePositive(t *testing.T) {
	Convey("Given positive validation helpers", t, func() {
		Convey("When validating a positive value", func() {
			So(ValidatePositive(10), ShouldBeTrue)
		})

		Convey("When validating a zero value", func() {
			So(ValidatePositive(0), ShouldBeFalse)
		})

		Convey("When validating a negative value", func() {
			So(ValidatePositive(-10), ShouldBeFalse)
		})
	})
}

func TestValidateNonZero(t *testing.T) {
	Convey("Given non-zero validation helpers", t, func() {
		Convey("When validating a non-zero value", func() {
			So(ValidateNonZero(10), ShouldBeTrue)
		})

		Convey("When validating a zero value", func() {
			So(ValidateNonZero(0), ShouldBeFalse)
		})
	})
}

func BenchmarkNormalizeRatio(b *testing.B) {
	for b.Loop() {
		NormalizeRatio(10, 5)
	}
}

func BenchmarkNormalizeDeviation(b *testing.B) {
	for b.Loop() {
		NormalizeDeviation(10, 5)
	}
}

func BenchmarkValidateFinite(b *testing.B) {
	for b.Loop() {
		ValidateFinite(10)
	}
}

func BenchmarkValidatePositive(b *testing.B) {
	for b.Loop() {
		ValidatePositive(10)
	}
}

func BenchmarkValidateNonZero(b *testing.B) {
	for b.Loop() {
		ValidateNonZero(10)
	}
}
