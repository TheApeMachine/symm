package broker

import (
	"math/big"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestDepthLadderRecord(t *testing.T) {
	Convey("Given retained decision depth", t, func() {
		ladder := &DepthLadder{}
		price, quantity := big.NewRat(101, 1), big.NewRat(3, 1)
		ladder.Record(price, quantity)
		quantity.SetInt64(1)
		So(ladder.Surviving(price, big.NewRat(5, 1)).RatString(), ShouldEqual, "3")

		Convey("The retained level budget cannot be overwritten by deeper prices", func() {
			for index := 1; index <= ladderLevels; index++ {
				ladder.Record(big.NewRat(int64(101+index), 1), quantity)
			}
			So(ladder.Count, ShouldEqual, ladderLevels)
			So(ladder.Surviving(big.NewRat(int64(101+ladderLevels), 1), quantity).Sign(), ShouldEqual, 0)
		})
	})
}

func TestDepthLadderSurviving(t *testing.T) {
	Convey("Given a level observed before execution", t, func() {
		ladder := &DepthLadder{}
		price := big.NewRat(101, 1)
		ladder.Record(price, big.NewRat(3, 1))
		So(ladder.Surviving(price, big.NewRat(2, 1)).RatString(), ShouldEqual, "2")
		So(ladder.Surviving(price, big.NewRat(5, 1)).RatString(), ShouldEqual, "3")
		So(ladder.Surviving(big.NewRat(102, 1), big.NewRat(5, 1)).Sign(), ShouldEqual, 0)

		Convey("A subsequent decision starts a fresh retained ladder", func() {
			ladder.Count = 0
			ladder.Record(big.NewRat(102, 1), big.NewRat(1, 1))
			So(ladder.Surviving(price, big.NewRat(5, 1)).Sign(), ShouldEqual, 0)
		})
	})
}

func BenchmarkDepthLadderSurviving(b *testing.B) {
	ladder := &DepthLadder{}
	quantity := big.NewRat(3, 1)

	for index := range ladderLevels {
		ladder.Record(big.NewRat(int64(101+index), 1), quantity)
	}
	price := big.NewRat(int64(100+ladderLevels), 1)
	b.ReportAllocs()

	for b.Loop() {
		ladder.Surviving(price, quantity)
	}
}
