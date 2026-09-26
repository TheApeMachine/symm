package store

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestManifoldMarketProject(t *testing.T) {
	Convey("Order projection preserves identity and measures forcing from the opposing aggressor", t, func() {
		market := &manifoldMarket{identities: map[string]int64{}}
		input := manifoldInput{symbol: "BTC/USD", sequence: 4, orders: []manifoldOrder{
			{"bid-first", true, 99, 1, 0}, {"bid-second", true, 98, 3, 1},
			{"ask-first", false, 100, 2, 0}, {"ask-second", false, 101, 4, 1},
		}}
		var identity int64
		first, departed := market.project(input, [3]uint32{8, 8, 8}, &identity)
		So(departed, ShouldBeEmpty)
		So(first.N, ShouldEqual, 4)
		So(first.ContentIDs, ShouldResemble, []int64{1, 2, 3, 4})
		So(first.Pos[2], ShouldEqual, 0.25)
		So(first.Pos[5], ShouldEqual, 0.75)
		So(first.Pos[0], ShouldBeGreaterThan, 0)
		So(first.Pos[9], ShouldBeLessThan, 1)
		input.excitation[0] = 1
		forced, departed := market.project(input, [3]uint32{8, 8, 8}, &identity)
		So(departed, ShouldBeEmpty)
		So(forced.ContentIDs, ShouldResemble, first.ContentIDs)
		So(forced.Energy[0], ShouldEqual, first.Energy[0])
		So(forced.Energy[2], ShouldAlmostEqual, 2*first.Energy[2])
		input.orders = input.orders[1:]
		input.orders[0].rank = 0
		reduced, departed := market.project(input, [3]uint32{8, 8, 8}, &identity)
		So(reduced.N, ShouldEqual, 3)
		So(departed, ShouldResemble, []int64{1})
		So(reduced.ContentIDs, ShouldResemble, []int64{2, 3, 4})
	})
}
