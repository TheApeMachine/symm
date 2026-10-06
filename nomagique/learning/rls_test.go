package learning_test

import (
	"fmt"
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/learning"
)

func TestRLSNext(t *testing.T) {
	for _, fixture := range [][3]float64{{1, 100, 1}, {3, 10, 0.98}, {4, 1, 0.995}} {
		dimension := int(fixture[0])

		Convey(fmt.Sprintf("RLS dimension %d predicts before it trains", dimension), t, func() {
			node := learning.NewRLS(dimension, fixture[1], fixture[2])
			features := make([]float64, dimension)

			for index := range features {
				features[index] = float64(index + 1)
			}

			first := data.Read[[][]float64](node.Next(data.NewValue(append(append([]float64(nil), features...), 1))))
			So(node.Error(), ShouldBeNil)
			So(first[0][6], ShouldEqual, 1)
			beta := append([]float64(nil), first[1]...)

			query := data.Read[[][]float64](node.Next(data.NewValue(features)))
			So(node.Error(), ShouldBeNil)
			So(query[0][6], ShouldEqual, 0)
			So(query[1], ShouldResemble, beta)
			prediction := beta[0]

			for index, feature := range features {
				prediction += beta[index+1] * feature
			}

			So(query[0][0], ShouldAlmostEqual, prediction)
		})
	}

	Convey("A row of the wrong width is a shape failure", t, func() {
		node := learning.NewRLS(2, 1, 1)

		for range node.Next(data.NewValue([]float64{1, 2, 3, 4})) {
			t.Fatal("a mis-shaped row must yield nothing")
		}

		So(node.Error(), ShouldNotBeNil)
	})
}

func BenchmarkRLSNext(b *testing.B) {
	const dimension = 154
	node := learning.NewRLS(dimension, 1, 1)
	features := make([]float64, dimension)
	labeled := make([]float64, dimension+1)
	step := 0
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		for index := range features {
			features[index] = math.Sin(float64((step + 1) * (index + 1)))
		}

		copy(labeled, features)
		labeled[dimension] = features[0] - features[1]

		for out := range node.Next(data.NewValue(labeled)) {
			_ = *(*[][]float64)(out)
		}

		for out := range node.Next(data.NewValue(features)) {
			_ = *(*[][]float64)(out)
		}

		if err := node.Error(); err != nil {
			b.Fatal(err)
		}

		step++
	}
}
