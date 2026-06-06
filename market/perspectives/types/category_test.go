package types

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
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

		Convey("It should read high clarity deep inside a band, 1/N at a boundary", func() {
			So(categories.Clarity(10), ShouldBeGreaterThan, 0.5)
			So(categories.Clarity(25), ShouldAlmostEqual, 1.0/3.0, 0.02) // boundary = uniform floor (3 categories)
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

func TestCategoryObserve(t *testing.T) {
	Convey("Given a tracked category", t, func() {
		category := NewCategory(CategoryLaminar)

		Convey("It should record the selected category", func() {
			err := category.Observe(CategoryTurbulent, 0.6)
			So(err, ShouldBeNil)
			So(category.Type, ShouldEqual, CategoryTurbulent)
		})

		Convey("It should accept honest extreme confidence without clamping", func() {
			So(category.Observe(CategoryLaminar, 0), ShouldBeNil)
			So(category.Observe(CategoryLaminar, 1), ShouldBeNil)
		})

		Convey("It should reject out-of-band confidence", func() {
			So(category.Observe(CategoryLaminar, 1.5), ShouldNotBeNil)
			So(category.Observe(CategoryLaminar, -0.1), ShouldNotBeNil)
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
