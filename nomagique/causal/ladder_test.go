package causal

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestLadderMeasure(t *testing.T) {
	Convey("Given equivalent causal tables expressed on different treatment scales", t, func() {
		base := make([][]float64, 24)
		rescaled := make([][]float64, len(base))

		for index := range base {
			treatment := float64(index+1) * 0.0001
			target := 0.75*treatment + float64(index%3-1)*0.00001
			base[index] = []float64{treatment, target}
			rescaled[index] = []float64{treatment * 0.000001, target}
		}

		config := LadderConfig{Target: 1, TreatmentNormal: 0}
		baseOutput, baseErr := NewLadder(config).Measure(LadderInput{Rows: base})
		rescaledOutput, rescaledErr := NewLadder(config).Measure(LadderInput{Rows: rescaled})

		Convey("It should preserve dimensionless intervention evidence", func() {
			So(baseErr, ShouldBeNil)
			So(rescaledErr, ShouldBeNil)
			So(math.Abs(rescaledOutput.Intervention), ShouldBeGreaterThan,
				math.Abs(baseOutput.Intervention))
			So(rescaledOutput.InterventionScore, ShouldAlmostEqual,
				baseOutput.InterventionScore, 1e-12)
		})
	})

	Convey("Given a treatment axis reflected around zero", t, func() {
		rows := make([][]float64, 24)
		reflected := make([][]float64, len(rows))

		for index := range rows {
			treatment := float64(index+1) * 0.0001
			target := 0.5*treatment + float64(index%2)*0.00001
			rows[index] = []float64{treatment, target}
			reflected[index] = []float64{-treatment, target}
		}

		config := LadderConfig{Target: 1, TreatmentNormal: 0}
		output, err := NewLadder(config).Measure(LadderInput{Rows: rows})
		reflectedOutput, reflectedErr := NewLadder(config).Measure(LadderInput{Rows: reflected})

		Convey("It should reverse the effect without changing its evidence magnitude", func() {
			So(err, ShouldBeNil)
			So(reflectedErr, ShouldBeNil)
			So(reflectedOutput.Intervention, ShouldAlmostEqual, -output.Intervention, 1e-12)
			So(math.Abs(reflectedOutput.InterventionScore), ShouldAlmostEqual,
				math.Abs(output.InterventionScore), 1e-12)
		})
	})
}

func BenchmarkLadderMeasure(b *testing.B) {
	rows := make([][]float64, 24)

	for index := range rows {
		treatment := float64(index+1) * 0.0001
		rows[index] = []float64{treatment, 0.75 * treatment}
	}

	ladder := NewLadder(LadderConfig{Target: 1, TreatmentNormal: 0})
	b.ReportAllocs()

	for b.Loop() {
		if _, err := ladder.Measure(LadderInput{Rows: rows}); err != nil {
			b.Fatal(err)
		}
	}
}
