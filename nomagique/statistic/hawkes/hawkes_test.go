package hawkes_test

import (
	"testing"
	"unsafe"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
	nmhawkes "github.com/theapemachine/symm/nomagique/statistic/hawkes"
)

func TestHawkes(t *testing.T) {
	Convey("Given a Hawkes Primitive", t, func() {
		op := nmhawkes.NewHawkes()
		So(op, ShouldNotBeNil)

		Convey("It processes arrivals and yields 62 metrics", func() {
			originSec := 1000.0

			for step := range 15 {
				atSec := originSec + float64(step)*0.1
				mark := 1.0

				if step%2 == 1 {
					mark = -1.0
				}

				raws := []float64{mark, atSec}
				results := make([]float64, 0, 62)

				for ptr := range op.Next(data.NewValue(
					unsafe.Pointer(&raws[0]),
					unsafe.Pointer(&raws[1]),
				).Next(nil)) {
					results = append(results, *(*float64)(ptr))
				}

				So(op.Error(), ShouldBeNil)
				So(len(results), ShouldEqual, 62)
				So(results[0], ShouldEqual, float64(step+1))

				fromSec := results[61]
				So(fromSec, ShouldEqual, originSec)

				countBuy := results[1]
				countSell := results[2]
				totalCount := results[0]
				So(countBuy+countSell, ShouldEqual, totalCount)

				fracBuy := results[3]
				fracSell := results[4]
				So(fracBuy, ShouldAlmostEqual, countBuy/totalCount, 1e-9)
				So(fracSell, ShouldAlmostEqual, countSell/totalCount, 1e-9)
				So(fracBuy+fracSell, ShouldAlmostEqual, 1.0, 1e-9)

				span := atSec - originSec

				if span > 0 {
					So(results[5], ShouldAlmostEqual, countBuy/span, 1e-4)
					So(results[6], ShouldAlmostEqual, countSell/span, 1e-4)
					So(results[7], ShouldAlmostEqual, totalCount/span, 1e-4)
				}
			}
		})

		Convey("It rejects regressing event timestamps", func() {
			opFresh := nmhawkes.NewHawkes()
			mark := 1.0
			at1 := 100.0
			at2 := 90.0

			for range opFresh.Next(data.NewValue(
				unsafe.Pointer(&mark),
				unsafe.Pointer(&at1),
			).Next(nil)) {
			}
			So(opFresh.Error(), ShouldBeNil)

			for range opFresh.Next(data.NewValue(
				unsafe.Pointer(&mark),
				unsafe.Pointer(&at2),
			).Next(nil)) {
			}
			So(opFresh.Error(), ShouldNotBeNil)
		})

		Convey("It rejects invalid excitation marks when 3 operands arrive", func() {
			opFresh := nmhawkes.NewHawkes()
			buy := 0.5
			sell := 0.5
			atSec := 100.0

			for range opFresh.Next(data.NewValue(
				unsafe.Pointer(&buy),
				unsafe.Pointer(&sell),
				unsafe.Pointer(&atSec),
			).Next(nil)) {
			}
			So(opFresh.Error(), ShouldNotBeNil)
		})
	})

	Convey("Given a Fit Primitive", t, func() {
		fit := nmhawkes.NewFit()
		So(fit, ShouldNotBeNil)

		Convey("It fits continuous-time events and yields 20 metrics", func() {
			events := make([][2]float64, 0, 20)

			for eventIndex := range 20 {
				side := 0.0

				if eventIndex%2 == 1 {
					side = 1.0
				}

				events = append(events, [2]float64{float64(eventIndex) * 0.1, side})
			}

			span := 2.0
			origin := 0.0

			fitIn := data.NewValue(
				unsafe.Pointer(&events),
				unsafe.Pointer(&span),
				unsafe.Pointer(&origin),
			)

			fitResults := make([]float64, 0, 20)

			for ptr := range fit.Next(fitIn.Next(nil)) {
				fitResults = append(fitResults, *(*float64)(ptr))
			}

			So(fit.Error(), ShouldBeNil)
			So(len(fitResults), ShouldEqual, 20)
			So(fitResults[0], ShouldBeGreaterThan, 0)
			So(fitResults[1], ShouldBeGreaterThan, 0)
			So(fitResults[2], ShouldBeGreaterThan, 0)
		})
	})

	Convey("Given a Parameters Primitive", t, func() {
		params := nmhawkes.NewParameters()
		So(params, ShouldNotBeNil)

		Convey("It computes spectral radius and branching properties", func() {
			var fitOutputs [20]float64
			fitOutputs[0] = 0.5
			fitOutputs[1] = 0.5
			fitOutputs[2] = 2.0
			fitOutputs[3] = 0.4
			fitOutputs[4] = 0.2
			fitOutputs[5] = 0.2
			fitOutputs[6] = 0.4

			inValues := make([]unsafe.Pointer, 20)

			for valIndex := range fitOutputs {
				inValues[valIndex] = unsafe.Pointer(&fitOutputs[valIndex])
			}

			paramResults := make([]float64, 0, 29)

			for ptr := range params.Next(data.NewValue(inValues...).Next(nil)) {
				paramResults = append(paramResults, *(*float64)(ptr))
			}

			So(params.Error(), ShouldBeNil)
			So(len(paramResults), ShouldEqual, 29)
			So(paramResults[18], ShouldBeGreaterThan, 0)
			So(paramResults[18], ShouldBeLessThan, 1)
		})
	})
}
