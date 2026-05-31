package trader

import (
	"testing"

	"github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/market/perspectives"
)

func TestThesisScore(t *testing.T) {
	convey.Convey("Given a thesis supported by multiple measurements", t, func() {
		measurements := []perspectives.Measurement{
			{Category: perspectives.CategoryAggressiveDrive, SNR: 2.0},
			{Category: perspectives.CategoryAggressiveDrive, SNR: 4.0},
		}

		convey.Convey("It should score root-mean-square signal energy", func() {
			score := thesisScore(measurements, []string{"drive"})

			convey.So(score, convey.ShouldBeGreaterThan, 3.0)
			convey.So(score, convey.ShouldBeLessThan, 3.2)
		})

		convey.Convey("It should rise when more relevant categories contribute SNR", func() {
			measurements = append(measurements, perspectives.Measurement{
				Category: perspectives.CategoryFrenzy,
				SNR:      5.0,
			})
			plain := thesisScore(measurements, []string{"drive"})
			confirmed := thesisScore(measurements, []string{"drive", "trend"})

			convey.So(confirmed, convey.ShouldBeGreaterThan, plain)
		})
	})
}

func TestRobustCenter(t *testing.T) {
	convey.Convey("Given a cross-section with one outlier", t, func() {
		median, spread := robustCenter([]float64{0.2, 0.2, 0.2, 3.0})

		convey.Convey("It should keep the center anchored to the current crowd", func() {
			convey.So(median, convey.ShouldEqual, 0.2)
			convey.So(spread, convey.ShouldEqual, 0)
		})
	})
}

func BenchmarkThesisScore(b *testing.B) {
	measurements := []perspectives.Measurement{
		{Category: perspectives.CategoryAggressiveDrive, SNR: 2.0},
		{Category: perspectives.CategoryAggressiveDrive, SNR: 4.0},
	}

	for b.Loop() {
		_ = thesisScore(measurements, []string{"drive", "trend"})
	}
}
