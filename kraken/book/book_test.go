package book

import (
	"testing"
	"time"

	"github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken/market"
)

var sampleBookSnapshot = []byte(`{
  "channel":"book",
  "type":"snapshot",
  "data":[
    {
      "symbol":"BTC/EUR",
      "bids":[{"price":100.0,"qty":2.0}],
      "asks":[{"price":101.0,"qty":1.0}]
    }
  ]
}`)

var sampleBookBidDelta = []byte(`{
  "channel":"book",
  "type":"update",
  "data":[
    {
      "symbol":"BTC/EUR",
      "bids":[{"price":100.5,"qty":3.0}]
    }
  ]
}`)

var sampleBookAskDelta = []byte(`{
  "channel":"book",
  "type":"update",
  "data":[
    {
      "symbol":"BTC/EUR",
      "asks":[{"price":101.5,"qty":1.5}]
    }
  ]
}`)

func TestBookMergesBidOnlyDeltaAfterSnapshot(t *testing.T) {
	convey.Convey("Given a book observer with a merged snapshot top", t, func() {
		book := &Book{
			bySymbol:  make(map[string]float64),
			spreadBPS: make(map[string]float64),
			density:   make(map[string]float64),
			ready:     make(map[string]bool),
			updatedAt: make(map[string]time.Time),
			tops:      make(map[string]topState),
		}

		snapshot, err := market.ParseBookTopDelta(sampleBookSnapshot)
		convey.So(err, convey.ShouldBeNil)
		book.applyTopDelta(snapshot)

		bidDelta, err := market.ParseBookTopDelta(sampleBookBidDelta)
		convey.So(err, convey.ShouldBeNil)
		book.applyTopDelta(bidDelta)

		spread, ok := book.SpreadBPS("BTC/EUR")

		convey.Convey("It should keep the prior ask and update spread from the merged top", func() {
			convey.So(ok, convey.ShouldBeTrue)
			convey.So(spread, convey.ShouldAlmostEqual, 49.63, 0.01)
		})
	})
}

func TestBookMergesAskOnlyDeltaAfterSnapshot(t *testing.T) {
	convey.Convey("Given a book observer with a merged snapshot top", t, func() {
		book := &Book{
			bySymbol:  make(map[string]float64),
			spreadBPS: make(map[string]float64),
			density:   make(map[string]float64),
			ready:     make(map[string]bool),
			updatedAt: make(map[string]time.Time),
			tops:      make(map[string]topState),
		}

		snapshot, err := market.ParseBookTopDelta(sampleBookSnapshot)
		convey.So(err, convey.ShouldBeNil)
		book.applyTopDelta(snapshot)

		askDelta, err := market.ParseBookTopDelta(sampleBookAskDelta)
		convey.So(err, convey.ShouldBeNil)
		book.applyTopDelta(askDelta)

		spread, ok := book.SpreadBPS("BTC/EUR")

		convey.Convey("It should keep the prior bid and update spread from the merged top", func() {
			convey.So(ok, convey.ShouldBeTrue)
			convey.So(spread, convey.ShouldAlmostEqual, 148.88, 0.01)
		})
	})
}

func TestBookDoesNotMarkReadyFromSingleSideDelta(t *testing.T) {
	convey.Convey("Given a book observer with no prior top", t, func() {
		book := &Book{
			bySymbol:  make(map[string]float64),
			spreadBPS: make(map[string]float64),
			density:   make(map[string]float64),
			ready:     make(map[string]bool),
			updatedAt: make(map[string]time.Time),
			tops:      make(map[string]topState),
		}

		bidDelta, err := market.ParseBookTopDelta(sampleBookBidDelta)
		convey.So(err, convey.ShouldBeNil)
		book.applyTopDelta(bidDelta)

		_, ok := book.SpreadBPS("BTC/EUR")

		convey.Convey("It should not publish spread until both sides exist", func() {
			convey.So(ok, convey.ShouldBeFalse)
		})
	})
}
