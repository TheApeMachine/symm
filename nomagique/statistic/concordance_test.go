package statistic

import (
	"errors"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/tests"
)

func TestConcordanceNext(t *testing.T) {
	Convey("Direct and inverse movement have equal affinity without a fixed window", t, func() {
		direct, inverse, changing, silent, scaled := NewConcordance(), NewConcordance(), NewConcordance(), NewConcordance(), NewConcordance()
		var directIn, inverseIn, changingIn, silentIn, scaledIn [][3]float64

		for index := 0; index < 100; index++ {
			movement := float64(index%7 - 3)
			directIn = append(directIn, [3]float64{movement, movement, 1})
			inverseIn = append(inverseIn, [3]float64{movement, -movement, 1})
			scaledIn = append(scaledIn, [3]float64{movement, movement, 1000})
			silentIn = append(silentIn, [3]float64{0, movement, 1})
			sign := 1.0

			if index%2 == 0 {
				sign = -1
			}

			changingIn = append(changingIn, [3]float64{movement, sign * movement, 1})
		}

		directOut := tests.CollectSeq[[3]float64](direct.Next(tests.SliceToSeq(directIn)))
		inverseOut := tests.CollectSeq[[3]float64](inverse.Next(tests.SliceToSeq(inverseIn)))
		scaledOut := tests.CollectSeq[[3]float64](scaled.Next(tests.SliceToSeq(scaledIn)))
		silentOut := tests.CollectSeq[[3]float64](silent.Next(tests.SliceToSeq(silentIn)))
		changingOut := tests.CollectSeq[[3]float64](changing.Next(tests.SliceToSeq(changingIn)))

		directLast := directOut[len(directOut)-1]
		inverseLast := inverseOut[len(inverseOut)-1]

		So(directLast[0], ShouldEqual, inverseLast[0])
		So(directLast[0], ShouldEqual, scaledOut[len(scaledOut)-1][0])
		So(inverseLast[1], ShouldEqual, -1)
		So(changingOut[len(changingOut)-1][0], ShouldBeLessThan, directLast[0])
		So(silentOut[len(silentOut)-1][0], ShouldEqual, 0)
		So(tests.CollectSeq[[3]float64](inverse.Next(tests.SliceToSeq([][3]float64{{0, 1, 1}})))[0][0], ShouldBeLessThan, 0)
		So(tests.CollectSeq[[3]float64](direct.Next(tests.SliceToSeq([][3]float64{{1, -1, 1}})))[0][0], ShouldBeLessThan, 0)
	})

	Convey("A negative clock increment records ErrDomain", t, func() {
		op := NewConcordance()
		out := tests.CollectSeq[[3]float64](op.Next(tests.SliceToSeq([][3]float64{{1, 1, -1}})))

		So(len(out), ShouldEqual, 0)
		So(errors.Is(op.Error(), core.ErrDomain), ShouldBeTrue)
	})
}

func BenchmarkConcordanceNext(b *testing.B) {
	op := NewConcordance()
	var pair [3]float64
	b.ReportAllocs()

	for index := 0; b.Loop(); index++ {
		pair = [3]float64{float64(index % 5), -float64(index % 5), 1}

		for range op.Next(tests.SliceToSeq([][3]float64{pair})) {
		}
	}
}
