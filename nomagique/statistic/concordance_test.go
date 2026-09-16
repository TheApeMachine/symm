package statistic

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestConcordanceUpdate(t *testing.T) {
	Convey("Direct and inverse movement have equal affinity without a fixed window", t, func() {
		var direct, inverse, changing, silent, scaled Concordance

		for index := 0; index < 100; index++ {
			movement := float64(index%7 - 3)
			direct.Update(movement, movement, 1)
			inverse.Update(movement, -movement, 1)
			scaled.Update(movement, movement, 1000)
			silent.Update(0, movement, 1)
			sign := 1.0

			if index%2 == 0 {
				sign = -1
			}

			changing.Update(movement, sign*movement, 1)
		}

		So(direct.Reading().Strength, ShouldEqual, inverse.Reading().Strength)
		So(direct.Reading().Strength, ShouldEqual, scaled.Reading().Strength)
		So(inverse.Reading().Orientation, ShouldEqual, -1)
		So(changing.Reading().Strength, ShouldBeLessThan, direct.Reading().Strength)
		So(silent.Reading().Strength, ShouldEqual, 0)
		So(inverse.Update(0, 1, 1).Strength, ShouldBeLessThan, 0)
		So(direct.Update(1, -1, 1).Strength, ShouldBeLessThan, 0)
	})
}

func BenchmarkConcordanceUpdate(b *testing.B) {
	var statistic Concordance
	b.ReportAllocs()

	for index := 0; index < b.N; index++ {
		statistic.Update(float64(index%5), -float64(index%5), 1)
	}
}
