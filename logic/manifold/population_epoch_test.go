package manifold

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestPopulationBeginEpoch(t *testing.T) {
	Convey("Given one persistent carrier population", t, func() {
		population := NewPopulation("BTC/USD", nil)

		Convey("When the accumulated population begins one field epoch", func() {
			first := population.BeginEpoch()
			second := population.BeginEpoch()

			Convey("Then epoch identity follows field boundaries", func() {
				So(first, ShouldEqual, uint64(1))
				So(second, ShouldEqual, uint64(2))
				So(population.Epoch(), ShouldEqual, uint64(2))
			})
		})
	})
}
