package algo_test

import (
	"testing"
	"unsafe"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/algo"
	"github.com/theapemachine/symm/nomagique/transport"
)

func TestSquareRootRLS_NoiseScale(t *testing.T) {
	Convey("RLS noise scale matches discounted objective", t, func() {
		node := algo.NewSquareRootRLS(1.0)
		query := algo.Query{
			Design:   []float64{1.0},
			Target:   1.0,
			Observed: true,
			Lambda:   0.5,
		}

		var reading algo.Reading

		for out := range node.Next(transport.NewOne(unsafe.Pointer(&query)).Next(nil)) {
			reading = *(*algo.Reading)(out)
		}

		So(node.Error(), ShouldBeNil)
		So(reading.Beta[0], ShouldAlmostEqual, 2.0/3.0, 1e-9)
		// J(2/3)/2 = (0.5*(2/3)^2 + (1-2/3)^2)/2 = (2/9 + 1/9)/2 = (1/3)/2 = 1/6
		So(reading.NoiseScale, ShouldAlmostEqual, 1.0/6.0, 1e-9)

		Convey("Lambda=1 preserves the standard undiscounted increment", func() {
			node1 := algo.NewSquareRootRLS(1.0)
			query1 := algo.Query{
				Design:   []float64{1.0},
				Target:   1.0,
				Observed: true,
				Lambda:   1.0,
			}

			var reading1 algo.Reading

			for out := range node1.Next(transport.NewOne(unsafe.Pointer(&query1)).Next(nil)) {
				reading1 = *(*algo.Reading)(out)
			}

			So(node1.Error(), ShouldBeNil)
			So(reading1.Beta[0], ShouldAlmostEqual, 0.5, 1e-9)
			// J(0.5)/2 = (0.5^2 + (1-0.5)^2)/2 = (0.25 + 0.25)/2 = 0.25
			So(reading1.NoiseScale, ShouldAlmostEqual, 0.25, 1e-9)
		})
	})
}

func TestSquareRootRLS_PredictiveVariance(t *testing.T) {
	Convey("Student-t predictive distribution validity and finite variance", t, func() {
		node := algo.NewSquareRootRLS(1.0)
		targets := []float64{1.0, 2.0, 0.0, 1.0}

		// Step 1: Target 1.0
		q1 := algo.Query{Design: []float64{1.0}, Target: targets[0], Observed: true, Lambda: 1.0}
		for out := range node.Next(transport.NewOne(unsafe.Pointer(&q1)).Next(nil)) {
			_ = *(*algo.Reading)(out)
		}

		// Query after 1 observation: nu = 1 <= 2, finite variance undefined
		check1 := algo.Query{Design: []float64{1.0}, Lambda: 1.0}
		var r1 algo.Reading
		for out := range node.Next(transport.NewOne(unsafe.Pointer(&check1)).Next(nil)) {
			r1 = *(*algo.Reading)(out)
		}
		So(r1.Ready, ShouldBeTrue)
		So(r1.DegreesOfFreedom, ShouldAlmostEqual, 1.0, 1e-9)
		So(r1.PredictiveVarianceDefined, ShouldBeFalse)

		// Step 2: Target 2.0
		q2 := algo.Query{Design: []float64{1.0}, Target: targets[1], Observed: true, Lambda: 1.0}
		for out := range node.Next(transport.NewOne(unsafe.Pointer(&q2)).Next(nil)) {
			_ = *(*algo.Reading)(out)
		}

		// Query after 2 observations: nu = 2 <= 2, finite variance undefined
		check2 := algo.Query{Design: []float64{1.0}, Lambda: 1.0}
		var r2 algo.Reading
		for out := range node.Next(transport.NewOne(unsafe.Pointer(&check2)).Next(nil)) {
			r2 = *(*algo.Reading)(out)
		}
		So(r2.Ready, ShouldBeTrue)
		So(r2.DegreesOfFreedom, ShouldAlmostEqual, 2.0, 1e-9)
		So(r2.PredictiveVarianceDefined, ShouldBeFalse)

		// Step 3: Target 0.0
		q3 := algo.Query{Design: []float64{1.0}, Target: targets[2], Observed: true, Lambda: 1.0}
		for out := range node.Next(transport.NewOne(unsafe.Pointer(&q3)).Next(nil)) {
			_ = *(*algo.Reading)(out)
		}

		// Query after 3 observations: nu = 3 > 2, finite variance defined!
		check3 := algo.Query{Design: []float64{1.0}, Lambda: 1.0}
		var r3 algo.Reading
		for out := range node.Next(transport.NewOne(unsafe.Pointer(&check3)).Next(nil)) {
			r3 = *(*algo.Reading)(out)
		}
		So(r3.Ready, ShouldBeTrue)
		So(r3.DegreesOfFreedom, ShouldAlmostEqual, 3.0, 1e-9)
		So(r3.PredictiveVarianceDefined, ShouldBeTrue)

		// Step 4: Target 1.0
		q4 := algo.Query{Design: []float64{1.0}, Target: targets[3], Observed: true, Lambda: 1.0}
		for out := range node.Next(transport.NewOne(unsafe.Pointer(&q4)).Next(nil)) {
			_ = *(*algo.Reading)(out)
		}

		// Query after 4 observations: nu = 4 > 2
		check4 := algo.Query{Design: []float64{1.0}, Lambda: 1.0}
		var r4 algo.Reading
		for out := range node.Next(transport.NewOne(unsafe.Pointer(&check4)).Next(nil)) {
			r4 = *(*algo.Reading)(out)
		}
		So(r4.Ready, ShouldBeTrue)
		So(r4.DegreesOfFreedom, ShouldAlmostEqual, 4.0, 1e-9)
		So(r4.PredictiveVarianceDefined, ShouldBeTrue)
		So(r4.Scale*r4.Scale, ShouldAlmostEqual, 0.84, 1e-2)
		So(r4.PredictiveVariance, ShouldAlmostEqual, 1.68, 1e-2)
		So(r4.PredictiveVariance, ShouldAlmostEqual, (r4.Scale*r4.Scale)*4.0/(4.0-2.0), 1e-9)
	})
}
