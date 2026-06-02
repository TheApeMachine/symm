package perspectives

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/numeric/adaptive"
)

func TestNewCategories(t *testing.T) {
	Convey("Given a three-band liquidity-style classifier", t, func() {
		categories, err := NewCategories(
			[]float64{25, 75},
			[]CategoryType{
				CategoryExtremeScarcity,
				CategoryMedianDepth,
				CategoryRobustLiquidity,
			},
		)

		So(err, ShouldBeNil)

		Convey("It should classify by observation band, not highest score", func() {
			low, err := categories.Classify(10)
			So(err, ShouldBeNil)
			So(low, ShouldEqual, CategoryExtremeScarcity)

			mid, err := categories.Classify(50)
			So(err, ShouldBeNil)
			So(mid, ShouldEqual, CategoryMedianDepth)

			high, err := categories.Classify(90)
			So(err, ShouldBeNil)
			So(high, ShouldEqual, CategoryRobustLiquidity)
		})

		Convey("It should read high clarity deep inside a band", func() {
			So(categories.Clarity(10), ShouldBeGreaterThan, 0.5)
			So(categories.Clarity(25), ShouldBeLessThan, 0.1)
		})
	})

	Convey("Given mismatched bounds and categories", t, func() {
		_, err := NewCategories(
			[]float64{25},
			[]CategoryType{CategoryExtremeScarcity},
		)

		So(err, ShouldNotBeNil)
	})
}

func TestScoreCategorySNR(t *testing.T) {
	Convey("Given a warmed SNR floor", t, func() {
		floor := adaptive.NewSNRField()
		symbol := "BTC/EUR"

		for index := range 15 {
			value := 0.55

			if index%2 == 1 {
				value = 0.45
			}

			_, err := ScoreCategorySNR(floor, symbol, value)
			So(err, ShouldBeNil)
		}

		Convey("It should spike when standout jumps, not when clarity stays flat", func() {
			fromStandout, err := ScoreCategorySNR(floor, symbol, 0.9)
			So(err, ShouldBeNil)

			fromClarity, err := ScoreCategorySNR(floor, symbol, 0.05)
			So(err, ShouldBeNil)

			So(fromStandout, ShouldBeGreaterThan, fromClarity)
		})

		Convey("It should error on non-unit standout", func() {
			_, err := ScoreCategorySNR(floor, symbol, 420_976_732_492.9974)
			So(err, ShouldNotBeNil)
		})

		Convey("It should error on nil floor", func() {
			_, err := ScoreCategorySNR(nil, symbol, 0.5)
			So(err, ShouldNotBeNil)
		})
	})
}

func TestCategoryObserve(t *testing.T) {
	Convey("Given a tracked category", t, func() {
		category := NewCategory(CategoryLaminar)

		Convey("It should hold when the type is unchanged", func() {
			first, err := category.Observe(CategoryLaminar, 0.8, 0.5)
			So(err, ShouldBeNil)
			So(first, ShouldEqual, 0.8)

			second, err := category.Observe(CategoryLaminar, 0.8, 0.5)
			So(err, ShouldBeNil)
			So(second, ShouldEqual, 0.8)
		})

		Convey("It should charge standout when the type shifts", func() {
			category := NewCategory(CategoryLaminar)

			_, err := category.Observe(CategoryLaminar, 0, 0)
			So(err, ShouldBeNil)

			after, err := category.Observe(CategoryTurbulent, 0.6, 0.9)
			So(err, ShouldBeNil)
			So(category.Type, ShouldEqual, CategoryTurbulent)
			So(after, ShouldEqual, 0.6)
			So(category.Confidence.Value(), ShouldEqual, 0.9)
		})
	})
}

func BenchmarkCategoriesClassify(b *testing.B) {
	categories, err := NewCategories(
		[]float64{25, 75},
		[]CategoryType{CategoryExtremeScarcity, CategoryMedianDepth, CategoryRobustLiquidity},
	)

	if err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()

	for b.Loop() {
		if _, err := categories.Classify(50); err != nil {
			b.Fatal(err)
		}
	}
}
