package reasoning

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/internal/testconfig"
)

func TestForestDedup(t *testing.T) {
	Convey("Given a reasoning forest", t, func() {
		forest := Seeds(DeriveVocabulary(ignitionRows()))[0]
		cache := newForestDedup()

		Convey("It should treat the first insert as fresh", func() {
			So(cache.insert(forest), ShouldBeFalse)
		})

		Convey("It should treat an identical forest as duplicate", func() {
			So(cache.insert(forest), ShouldBeFalse)
			So(cache.insert(forest), ShouldBeTrue)
		})

		Convey("It should distinguish threshold changes", func() {
			changed := cloneForest(forest)
			changed[0].When.All[1].Value = 2

			So(cache.insert(forest), ShouldBeFalse)
			So(cache.insert(changed), ShouldBeFalse)
		})
	})
}

func TestSearchParallelMatchesSequential(t *testing.T) {
	Convey("Given a profitable tape", t, func() {
		testconfig.Load(t)

		rows := rallyTape()
		costs := frictionlessCosts()

		sequential, err := Search(context.Background(), rows, costs, SearchConfig{
			BeamWidth: 4,
			MaxRounds: 3,
			Patience:  2,
			Workers:   1,
		})
		So(err, ShouldBeNil)

		parallel, err := Search(context.Background(), rows, costs, SearchConfig{
			BeamWidth: 4,
			MaxRounds: 3,
			Patience:  2,
			Workers:   4,
		})
		So(err, ShouldBeNil)

		Convey("It should evaluate the same number of candidates", func() {
			So(parallel.Evaluated, ShouldEqual, sequential.Evaluated)
		})

		Convey("It should find the same best return", func() {
			So(parallel.Best.Return, ShouldEqual, sequential.Best.Return)
			So(parallel.Best.Trades, ShouldEqual, sequential.Best.Trades)
		})
	})
}
