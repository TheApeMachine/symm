package toxicity

import (
	"math"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/tests/market"
)

func toxicityOrder(price, qty float64, at time.Time) kraken.Level3Order {
	return kraken.Level3Order{
		Event:      "add",
		OrderID:    "order",
		LimitPrice: decimal.NewFromFloat64(price),
		OrderQty:   decimal.NewFromFloat64(qty),
		Timestamp:  at,
	}
}

func toxicityMessage(symbol string, at time.Time, bidPrice, bidQty, askPrice, askQty float64) kraken.Level3Data {
	return kraken.Level3Data{
		Symbol:    symbol,
		Timestamp: at,
		Bids:      []kraken.Level3Order{toxicityOrder(bidPrice, bidQty, at)},
		Asks:      []kraken.Level3Order{toxicityOrder(askPrice, askQty, at)},
	}
}

func crossedToxicityMessage(symbol string, at time.Time) kraken.Level3Data {
	return kraken.Level3Data{
		Symbol:    symbol,
		Timestamp: at,
		Bids:      []kraken.Level3Order{toxicityOrder(101, 10, at)},
		Asks:      []kraken.Level3Order{toxicityOrder(99, 12, at)},
	}
}

func TestLevel3Step(t *testing.T) {
	Convey("Given a sequence of touch observations", t, func() {
		entity := NewLevel3()

		Convey("the first observation anchors the previous touch", func() {
			measurement := entity.Step(toxicityMessage("BTC/USD", time.Unix(1_700_000_000, 0), 99, 10, 101, 12))

			So(measurement, ShouldNotBeNil)
			So(measurement.Err, ShouldBeNil)
			So(measurement.Metrics, ShouldNotBeEmpty)
			So(measurement.Metrics["best_price:bid"].Raw, ShouldEqual, 99.0)
			So(measurement.Metrics["best_price:ask"].Raw, ShouldEqual, 101.0)
			So(measurement.Metrics["touch_quantity:bid"].Raw, ShouldEqual, 10.0)
			So(measurement.Metrics["touch_quantity:ask"].Raw, ShouldEqual, 12.0)
			So(measurement.Metrics["touch_price_log_change:bid"].Raw, ShouldEqual, 0.0)
			So(measurement.Metrics["unfilled_residual_quantity:bid"].Raw, ShouldEqual, 10.0)

			// Stateless direct measurement is whole (Maturity 1).
			So(measurement.Maturity, ShouldEqual, 1.0)
			So(measurement.SNR, ShouldEqual, 0.0)
		})

		Convey("a later observation attributes a bid retreat", func() {
			// README §2.2: a retreat is a less aggressive NEW BEST price. The
			// touch at 99 must therefore be withdrawn before 98 becomes the
			// best bid — a bare add at 98 while 99 still rests says nothing
			// about the touch, because 99 is still the best displayed price.
			first := time.Unix(1_700_000_000, 0)
			second := time.Unix(1_700_000_001, 0)

			entity.Step(toxicityMessage("BTC/USD", first, 99, 10, 101, 12))

			withdrawn := toxicityOrder(99, 10, second)
			withdrawn.Event = "delete"

			entity.Step(kraken.Level3Data{
				Symbol:    "BTC/USD",
				Timestamp: second,
				Bids:      []kraken.Level3Order{withdrawn},
			})

			measurement := entity.Step(toxicityMessage("BTC/USD", second, 98, 5, 101, 12))

			So(measurement.Err, ShouldBeNil)
			So(measurement.Metrics["previous_best_price:bid"].Raw, ShouldEqual, 99.0)
			So(measurement.Metrics["best_price:bid"].Raw, ShouldEqual, 98.0)
			So(measurement.Metrics["touch_price_log_change:bid"].Raw, ShouldAlmostEqual, math.Log(98.0/99.0), 1e-12)
			So(measurement.Metrics["retreated_quantity:bid"].Raw, ShouldEqual, 10.0)
			So(measurement.Metrics["net_withdrawn_quantity:bid"].Raw, ShouldEqual, 0.0)
			So(measurement.Metrics["net_replenished_quantity:bid"].Raw, ShouldEqual, 0.0)
			So(measurement.Metrics["retreat_fraction:bid"].Raw, ShouldAlmostEqual, 1.0, 1e-12)
			So(measurement.Metrics["net_withdrawal_fraction:bid"].Raw, ShouldEqual, 0.0)
			So(measurement.Metrics["retreat_rate:bid"].Raw, ShouldAlmostEqual, 10.0, 1e-12)
		})

		Convey("a later observation attributes an unchanged-touch withdrawal", func() {
			entity.Step(toxicityMessage("BTC/USD", time.Unix(1_700_000_000, 0), 99, 10, 101, 12))

			measurement := entity.Step(toxicityMessage("BTC/USD", time.Unix(1_700_000_001, 0), 99, 4, 101, 12))

			So(measurement.Err, ShouldBeNil)
			So(measurement.Metrics["touch_price_log_change:bid"].Raw, ShouldEqual, 0.0)
			So(measurement.Metrics["net_withdrawn_quantity:bid"].Raw, ShouldEqual, 6.0)
			So(measurement.Metrics["net_replenished_quantity:bid"].Raw, ShouldEqual, 0.0)
			So(measurement.Metrics["retreated_quantity:bid"].Raw, ShouldEqual, 0.0)
			So(measurement.Metrics["net_withdrawal_fraction:bid"].Raw, ShouldAlmostEqual, 0.6, 1e-12)
			So(measurement.Metrics["net_replenishment_fraction:bid"].Raw, ShouldEqual, 0.0)
			So(measurement.Metrics["net_withdrawal_rate:bid"].Raw, ShouldAlmostEqual, 6.0, 1e-12)
			So(measurement.Metrics["withdrawal_fraction_baseline:bid"].Raw, ShouldNotEqual, 0.0)
			So(measurement.Metrics["withdrawal_fraction_zscore:bid"].Raw, ShouldNotEqual, 0.0)
		})
	})

	Convey("Given a one-sided Level-3 update", t, func() {
		entity := NewLevel3()

		entity.Step(toxicityMessage("BTC/USD", time.Unix(1_700_000_000, 0), 99, 10, 101, 12))

		Convey("the opposite-side touch is borrowed from the last retained touch", func() {
			// The message improves the bid to 99.5 and says nothing about the
			// ask, which must come from the retained touch.
			second := time.Unix(1_700_000_001, 0)

			measurement := entity.Step(kraken.Level3Data{
				Symbol:    "BTC/USD",
				Timestamp: second,
				Bids:      []kraken.Level3Order{toxicityOrder(99.5, 5, second)},
			})

			So(measurement, ShouldNotBeNil)
			So(measurement.Err, ShouldBeNil)
			So(measurement.Metrics["best_price:bid"].Raw, ShouldEqual, 99.5)
			So(measurement.Metrics["best_price:ask"].Raw, ShouldEqual, 101.0)
			So(measurement.Metrics["previous_best_price:ask"].Raw, ShouldEqual, 101.0)
		})

		Convey("an order behind the touch does not move it", func() {
			// 98 is worse than the resting 99, so the best bid is unchanged:
			// this message announces depth, it does not restate the side.
			second := time.Unix(1_700_000_001, 0)

			measurement := entity.Step(kraken.Level3Data{
				Symbol:    "BTC/USD",
				Timestamp: second,
				Bids:      []kraken.Level3Order{toxicityOrder(98, 5, second)},
			})

			So(measurement, ShouldNotBeNil)
			So(measurement.Err, ShouldBeNil)
			So(measurement.Metrics["best_price:bid"].Raw, ShouldEqual, 99.0)
		})
	})

	Convey("Given a crossed message", t, func() {
		entity := NewLevel3()
		at := time.Unix(1_700_000_000, 0)

		Convey("no attribution is published for an inverted book", func() {
			So(entity.Step(crossedToxicityMessage("BTC/USD", at)), ShouldBeNil)

			Convey("but its prices are retained, not discarded", func() {
				// The crossed message showed bid 101 / ask 99. Withdrawing the
				// crossing ask surrenders that side; the next ask names a new
				// touch above the retained bid and the book resolves.
				second := at.Add(time.Second)

				withdrawn := toxicityOrder(99, 12, second)
				withdrawn.Event = "delete"

				So(entity.Step(kraken.Level3Data{
					Symbol:    "BTC/USD",
					Timestamp: second,
					Asks:      []kraken.Level3Order{withdrawn},
				}), ShouldBeNil)

				resolved := entity.Step(kraken.Level3Data{
					Symbol:    "BTC/USD",
					Timestamp: second,
					Asks:      []kraken.Level3Order{toxicityOrder(103, 4, second)},
				})

				So(resolved, ShouldNotBeNil)
				So(resolved.Err, ShouldBeNil)
				So(resolved.Metrics["best_price:bid"].Raw, ShouldEqual, 101.0)
				So(resolved.Metrics["best_price:ask"].Raw, ShouldEqual, 103.0)
			})
		})
	})

	Convey("Given a message with no usable touch on either side", t, func() {
		entity := NewLevel3()

		Convey("Step reports nothing rather than panicking", func() {
			measurement := entity.Step(kraken.Level3Data{
				Symbol:    "BTC/USD",
				Timestamp: time.Unix(1_700_000_000, 0),
			})

			So(measurement, ShouldBeNil)
		})
	})

	Convey("Given a symbol whose feed opens one-sided", t, func() {
		entity := NewLevel3()
		first := time.Unix(1_700_000_000, 0)
		second := first.Add(time.Second)
		third := second.Add(time.Second)

		Convey("the opening side commits without reporting a half-formed touch", func() {
			measurement := entity.Step(kraken.Level3Data{
				Symbol:    "BTC/USD",
				Timestamp: first,
				Bids:      []kraken.Level3Order{toxicityOrder(99, 10, first)},
			})

			So(measurement, ShouldBeNil)

			Convey("and the next side completes the touch against the retained one", func() {
				completing := entity.Step(kraken.Level3Data{
					Symbol:    "BTC/USD",
					Timestamp: second,
					Asks:      []kraken.Level3Order{toxicityOrder(101, 12, second)},
				})

				So(completing, ShouldNotBeNil)
				So(completing.Err, ShouldBeNil)
				So(completing.Metrics["best_price:bid"].Raw, ShouldEqual, 99.0)
				So(completing.Metrics["best_price:ask"].Raw, ShouldEqual, 101.0)

				Convey("so later one-sided updates keep measuring", func() {
					later := entity.Step(kraken.Level3Data{
						Symbol:    "BTC/USD",
						Timestamp: third,
						Bids:      []kraken.Level3Order{toxicityOrder(99, 4, third)},
					})

					So(later, ShouldNotBeNil)
					So(later.Err, ShouldBeNil)
					So(later.Metrics["best_price:ask"].Raw, ShouldEqual, 101.0)
					So(later.Metrics["net_withdrawn_quantity:bid"].Raw, ShouldEqual, 6.0)
				})
			})
		})
	})

	Convey("Given a delete of the displayed best bid", t, func() {
		entity := NewLevel3()
		first := time.Unix(1_700_000_000, 0)
		second := first.Add(time.Second)

		entity.Step(toxicityMessage("BTC/USD", first, 99, 10, 101, 12))

		Convey("the removed order is not counted as resting liquidity", func() {
			deleted := toxicityOrder(99, 10, second)
			deleted.Event = "delete"

			measurement := entity.Step(kraken.Level3Data{
				Symbol:    "BTC/USD",
				Timestamp: second,
				Bids:      []kraken.Level3Order{deleted},
			})

			So(measurement, ShouldBeNil)
		})
	})

	Convey("Given the standard Level-3 tape", t, func() {
		tape := market.NewLevel3Tape("BTC/USD", time.Unix(1_700_000_000, 0))
		entity := NewLevel3()

		Convey("No message is ever rejected as an error", func() {
			for index, message := range tape.Messages {
				measurement := entity.Step(message)

				if measurement == nil {
					continue
				}

				So(measurement.Err, ShouldBeNil)

				// A published touch must match the book that really exists.
				bid, hasBid := measurement.Metrics["best_price:bid"]
				ask, hasAsk := measurement.Metrics["best_price:ask"]

				if !hasBid || !hasAsk {
					continue
				}

				So(bid.Raw, ShouldEqual, tape.TrueBid[index])
				So(ask.Raw, ShouldEqual, tape.TrueAsk[index])
			}
		})
	})

	Convey("Given a book that is entirely withdrawn", t, func() {
		tape := market.NewLevel3DeleteOnlyTape("BTC/USD", time.Unix(1_700_000_000, 0))
		entity := NewLevel3()

		Convey("Withdrawn liquidity is never published as a touch", func() {
			for index, message := range tape.Messages {
				measurement := entity.Step(message)

				if measurement == nil {
					continue
				}

				So(measurement.Err, ShouldBeNil)

				if tape.TrueBid[index] != 0 {
					continue
				}

				// The book is empty here; nothing may claim a resting bid.
				_, hasBid := measurement.Metrics["best_price:bid"]
				So(hasBid, ShouldBeFalse)
			}
		})
	})

	Convey("Given a long churn tape", t, func() {
		tape := market.NewLevel3ChurnTape("BTC/USD", time.Unix(1_700_000_000, 0), 400)
		entity := NewLevel3()

		Convey("Every message is survived and every touch matches the book", func() {
			published := 0

			for index, message := range tape.Messages {
				measurement := entity.Step(message)

				if measurement == nil {
					continue
				}

				So(measurement.Err, ShouldBeNil)

				bid, hasBid := measurement.Metrics["best_price:bid"]
				ask, hasAsk := measurement.Metrics["best_price:ask"]

				if !hasBid || !hasAsk {
					continue
				}

				So(bid.Raw, ShouldEqual, tape.TrueBid[index])
				So(ask.Raw, ShouldEqual, tape.TrueAsk[index])
				So(bid.Raw, ShouldBeLessThan, ask.Raw)

				published++
			}

			// The tape must actually exercise the signal, not pass vacuously.
			So(published, ShouldBeGreaterThan, 0)
		})
	})
}

func BenchmarkLevel3Step(b *testing.B) {
	entity := NewLevel3()
	message := toxicityMessage(
		"BTC/USD",
		time.Unix(1_700_000_000, 0),
		99,
		10,
		101,
		12,
	)
	entity.Step(message)
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		message.Timestamp = message.Timestamp.Add(time.Nanosecond)
		entity.Step(message)
	}
}
