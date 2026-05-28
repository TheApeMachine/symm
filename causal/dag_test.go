package causal

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestDAGNodeTableBackdoorEffect(t *testing.T) {
	Convey("Given generic DAG rows with a confounded treatment", t, func() {
		rows := make([][]float64, 0, 32)

		for index := range 32 {
			confounder := float64(index%4) * 0.01
			treatment := confounder*50 + float64(index%5)*0.2
			target := confounder*40 + treatment*0.2
			rows = append(rows, []float64{confounder, treatment, target})
		}

		nodeTable, err := newDAGNodeTable(rows, 2, minCausalHistory)

		Convey("It should estimate intervention from node indexes", func() {
			So(err, ShouldBeNil)

			effect, effectErr := nodeTable.BackdoorEffect(1, 0)

			So(effectErr, ShouldBeNil)
			So(effect, ShouldBeGreaterThan, 0)
		})
	})
}

func TestDAGLinearModelCounterfactualUplift(t *testing.T) {
	Convey("Given generic structural rows", t, func() {
		rows := make([][]float64, 0, minCausalHistory)

		for index := range minCausalHistory {
			first := float64(index%4) * 0.005
			second := 1 + float64(index%3)*0.1
			treatment := float64(index) * 0.5
			target := first*2 + second*0.1 + treatment*0.4
			rows = append(rows, []float64{first, second, treatment, target})
		}

		nodeTable, tableErr := newDAGNodeTable(rows, 3, minCausalHistory)
		model, modelErr := nodeTable.LinearModel(0, 1, 2)
		uplift, upliftErr := model.CounterfactualUplift(rows[3], 2, rows[11][2])

		Convey("It should predict counterfactual uplift without domain field names", func() {
			So(tableErr, ShouldBeNil)
			So(modelErr, ShouldBeNil)
			So(upliftErr, ShouldBeNil)
			So(uplift, ShouldBeGreaterThan, 0)
		})
	})
}

func TestDAGNodeTableRejectsInvalidShape(t *testing.T) {
	Convey("Given ragged DAG rows", t, func() {
		_, err := newDAGNodeTable([][]float64{
			{1, 2, 3},
			{1, 2},
		}, 2, 1)

		Convey("It should return an explicit error", func() {
			So(err, ShouldNotBeNil)
		})
	})
}

func BenchmarkDAGNodeTableBackdoorEffect(b *testing.B) {
	rows := make([][]float64, 0, causalHistoryCap)

	for index := range causalHistoryCap {
		confounder := float64(index%5) * 0.01
		treatment := confounder*20 + float64(index)*0.3
		target := confounder*15 + treatment*0.2
		rows = append(rows, []float64{confounder, treatment, target})
	}

	nodeTable, err := newDAGNodeTable(rows, 2, minCausalHistory)

	if err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()

	for b.Loop() {
		if _, err := nodeTable.BackdoorEffect(1, 0); err != nil {
			b.Fatal(err)
		}
	}
}
