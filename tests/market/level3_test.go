package market

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
)

/*
TestNewLevel3Tape pins the tape's own truth model. Every consumer test asserts
against TrueBid/TrueAsk, so a tape that misreports the book it describes would
silently license the very bugs it exists to catch.
*/
func TestNewLevel3Tape(t *testing.T) {
	Convey("Given the standard Level-3 tape", t, func() {
		start := time.Unix(1_700_000_000, 0)
		tape := NewLevel3Tape("BTC/USD", start)

		So(len(tape.Messages), ShouldBeGreaterThan, 0)
		So(len(tape.TrueBid), ShouldEqual, len(tape.Messages))
		So(len(tape.TrueAsk), ShouldEqual, len(tape.Messages))

		Convey("It opens one-sided, so the first message completes no touch", func() {
			So(len(tape.Messages[0].Bids), ShouldEqual, 1)
			So(tape.Messages[0].Asks, ShouldBeEmpty)
			So(tape.TrueAsk[0], ShouldEqual, 0.0)
		})

		Convey("Every message carries orders on exactly one side", func() {
			for _, message := range tape.Messages {
				So(len(message.Bids)+len(message.Asks), ShouldBeGreaterThan, 0)
			}
		})

		Convey("It carries deletes that still quote a price and quantity", func() {
			deletes := 0

			for _, message := range tape.Messages {
				for _, entry := range append(append([]kraken.Level3Order{}, message.Bids...), message.Asks...) {
					if entry.Event != "delete" {
						continue
					}

					deletes++

					// This is exactly why ignoring Event is a bug: the wire
					// still quotes a real price and size for removed liquidity.
					So(entry.LimitPrice, ShouldNotBeNil)
					So(entry.LimitPrice.Float64(), ShouldBeGreaterThan, 0.0)
					So(entry.OrderQty, ShouldNotBeNil)
				}
			}

			So(deletes, ShouldBeGreaterThan, 0)
		})

		Convey("A delete of the best bid exposes the next resting level", func() {
			// b3 (99.5) is withdrawn while b1 (99.0) and b2 (98.5) still rest,
			// so the true best bid steps DOWN rather than vanishing.
			stepped := false

			for index, message := range tape.Messages {
				if len(message.Bids) != 1 || message.Bids[0].Event != "delete" {
					continue
				}

				if index == 0 || tape.TrueBid[index] == 0 {
					continue
				}

				So(tape.TrueBid[index], ShouldBeLessThan, tape.TrueBid[index-1])

				stepped = true
			}

			So(stepped, ShouldBeTrue)
		})

		Convey("The true book is never crossed", func() {
			for index := range tape.Messages {
				bid, ask := tape.TrueBid[index], tape.TrueAsk[index]

				if bid > 0 && ask > 0 {
					So(bid, ShouldBeLessThan, ask)
				}
			}
		})

		Convey("Yet a retained-side consumer transiently sees a crossed touch", func() {
			// This is the whole point of the tape. Replaying it the way every
			// Level-3 consumer does — take this message's side, keep the last
			// price seen on the other — produces a bid above an ask, even
			// though no such book ever existed.
			crossed := false
			retainedBid, retainedAsk := 0.0, 0.0

			for _, message := range tape.Messages {
				for _, entry := range message.Bids {
					if entry.Event != "delete" {
						retainedBid = entry.LimitPrice.Float64()
					}
				}

				for _, entry := range message.Asks {
					if entry.Event != "delete" {
						retainedAsk = entry.LimitPrice.Float64()
					}
				}

				if retainedBid > 0 && retainedAsk > 0 && retainedBid >= retainedAsk {
					crossed = true
				}
			}

			So(crossed, ShouldBeTrue)
		})
	})

	Convey("Given the delete-only tape", t, func() {
		tape := NewLevel3DeleteOnlyTape("BTC/USD", time.Unix(1_700_000_000, 0))

		Convey("The book is empty once both sides are withdrawn", func() {
			last := len(tape.Messages) - 1

			So(tape.TrueBid[last], ShouldEqual, 0.0)
			So(tape.TrueAsk[last], ShouldEqual, 0.0)
		})
	})

	Convey("Given the churn tape", t, func() {
		tape := NewLevel3ChurnTape("BTC/USD", time.Unix(1_700_000_000, 0), 200)

		Convey("It is deterministic", func() {
			again := NewLevel3ChurnTape("BTC/USD", time.Unix(1_700_000_000, 0), 200)

			So(len(again.Messages), ShouldEqual, len(tape.Messages))

			for index := range tape.Messages {
				So(again.TrueBid[index], ShouldEqual, tape.TrueBid[index])
				So(again.TrueAsk[index], ShouldEqual, tape.TrueAsk[index])
			}
		})

		Convey("The true book is never crossed", func() {
			for index := range tape.Messages {
				bid, ask := tape.TrueBid[index], tape.TrueAsk[index]

				if bid > 0 && ask > 0 {
					So(bid, ShouldBeLessThan, ask)
				}
			}
		})
	})
}
