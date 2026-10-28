package causal

import (
	"io"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestDeriveBandwidth(testingTB *testing.T) {
	Convey("Given tabular rows with control spread", testingTB, func() {
		rows := [][]float64{
			{1, 0, 0},
			{2, 1, 2},
			{3, 2, 4},
			{4, 3, 6},
			{5, 4, 8},
			{6, 5, 10},
			{7, 6, 12},
			{8, 7, 14},
			{9, 8, 16},
		}
		bandwidth, err := deriveBandwidth(rows, 0)

		So(err, ShouldBeNil)
		So(bandwidth, ShouldBeGreaterThan, 0)

		Convey("It should be invariant to control units after standardization", func() {
			wideRows := [][]float64{
				{1, 0, 0},
				{5, 1, 2},
				{10, 2, 4},
				{15, 3, 6},
				{20, 4, 8},
				{25, 5, 10},
				{30, 6, 12},
				{35, 7, 14},
				{40, 8, 16},
			}
			wideBandwidth, wideErr := deriveBandwidth(wideRows, 0)

			So(wideErr, ShouldBeNil)
			So(wideBandwidth, ShouldAlmostEqual, bandwidth)
		})
	})

	Convey("Given signed control spread centered near zero", testingTB, func() {
		rows := [][]float64{
			{-4, 0, -8},
			{-3, 1, -6},
			{-2, 2, -4},
			{-1, 3, -2},
			{0, 4, 0},
			{1, 5, 2},
			{2, 6, 4},
			{3, 7, 6},
			{4, 8, 8},
		}
		bandwidth, err := deriveBandwidth(rows, 0)

		Convey("It should derive a positive dimensionless bandwidth", func() {
			So(err, ShouldBeNil)
			So(bandwidth, ShouldBeGreaterThan, 0)
		})
	})

	Convey("Given insufficient rows", testingTB, func() {
		_, err := deriveBandwidth([][]float64{{1, 2}}, 0)

		Convey("It should reject bandwidth derivation", func() {
			So(err, ShouldNotBeNil)
		})
	})

	Convey("Given constant control values", testingTB, func() {
		_, err := deriveBandwidth([][]float64{
			{1, 0},
			{1, 1},
			{1, 2},
			{1, 3},
		}, 0)

		Convey("It should report no identifiable bandwidth yet", func() {
			So(err, ShouldEqual, io.EOF)
		})
	})
}

func TestNodeTable_association(testingTB *testing.T) {
	Convey("Given constant treatment values", testingTB, func() {
		table, tableErr := newNodeTable([][]float64{
			{0, 1},
			{0, 2},
			{0, 3},
			{0, 4},
		}, 1, 4)

		So(tableErr, ShouldBeNil)

		_, err := table.association(0)

		Convey("It should report no identifiable association yet", func() {
			So(err, ShouldEqual, io.EOF)
		})
	})
}

func TestLinearModel_counterfactualUplift(testingTB *testing.T) {
	Convey("Given a linear model fit on treatment and controls", testingTB, func() {
		rows := [][]float64{
			{0, 0, 0, 0},
			{1, 0, 2, 2.5},
			{0, 1, 3, 2.5},
			{1, 1, 5, 5},
			{2, 1, 4, 5},
			{1, 2, 7, 6.5},
			{2, 2, 6, 6.5},
			{3, 1, 8, 9.5},
		}
		table, tableErr := newNodeTable(rows, 3, 4)

		So(tableErr, ShouldBeNil)

		model, modelErr := table.fitLinearModel(0, 1, 2)

		So(modelErr, ShouldBeNil)

		uplift, upliftErr := model.counterfactualUplift(rows[len(rows)-1], 2, 20)

		So(upliftErr, ShouldBeNil)
		So(uplift, ShouldBeGreaterThan, 0)
	})
}

func TestNodeTable_fitLinearModel(testingTB *testing.T) {
	Convey("Given a structurally rank-deficient causal table", testingTB, func() {
		rows := [][]float64{
			{0, 0, 1},
			{1, 1, 4},
			{2, 2, 7},
			{3, 3, 10},
			{4, 4, 13},
			{5, 5, 16},
		}
		table, tableErr := newNodeTable(rows, 2)

		So(tableErr, ShouldBeNil)

		model, err := table.fitLinearModel(0, 1)

		Convey("It should fit the independent control span once", func() {
			So(err, ShouldBeNil)
			So(model.predictors, ShouldResemble, []int{0, 1})

			prediction, predictionErr := model.predict(rows[len(rows)-1], -1, 0)
			So(predictionErr, ShouldBeNil)
			So(prediction, ShouldAlmostEqual, rows[len(rows)-1][2], 1e-12)
		})
	})
}

func TestNodeTable_kernelBackdoorEffect(testingTB *testing.T) {
	Convey("Given a confounded table with a control", testingTB, func() {
		rows := [][]float64{
			{0, 0, 0},
			{1, 1, 2},
			{2, 2, 4},
			{3, 3, 6},
			{4, 4, 8},
			{5, 5, 10},
			{6, 6, 12},
			{7, 7, 14},
			{8, 8, 16},
			{9, 9, 18},
			{10, 10, 20},
			{11, 11, 22},
		}
		table, tableErr := newNodeTable(rows, 2, 12)

		So(tableErr, ShouldBeNil)

		linearEffect, linearErr := table.backdoorEffect(1, 0)
		kernelEffect, kernelErr := table.kernelBackdoorEffect(1, 0.5, 0)

		So(linearErr, ShouldBeNil)
		So(kernelErr, ShouldBeNil)

		Convey("It should align the kernel path with residualized backdoor adjustment", func() {
			So(kernelEffect, ShouldAlmostEqual, linearEffect, 1e-6)
		})
	})

	Convey("Given control rows outside the kernel support", testingTB, func() {
		rows := [][]float64{
			{0, 1, 3},
			{1e9, 2, 4},
			{2e9, 10, 20},
			{3e9, 4, 8},
			{4e9, 9, 17},
		}
		table, tableErr := newNodeTable(rows, 2, 5)

		So(tableErr, ShouldBeNil)

		effect, err := table.kernelBackdoorEffect(1, 0.5, 0)

		Convey("It should skip exact-zero weights without rejecting the table", func() {
			So(err, ShouldBeNil)
			So(effect, ShouldNotEqual, 0)
		})
	})
}
