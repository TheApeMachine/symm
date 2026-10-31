package broker

import (
	"testing"
	"time"

	spotbook "github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/system"
)

func TestBackingRatioRejectsAskHeavyBand(t *testing.T) {
	Convey("bid cash thinner than ask cash inside the mid band fails the ratio", t, func() {
		managed := newBookFixture(t, "THIN/USD", 2, 0)
		So(managed.Create("THIN/USD", 10), ShouldBeNil)

		at := time.Now().UTC()
		payload := &kraken.Level3{Data: []kraken.Level3Data{{
			Symbol: "THIN/USD",
			Bids: []kraken.Level3Order{{
				OrderID: "b1", LimitPrice: decimal.NewFromFloat64(1.00),
				OrderQty: decimal.NewFromInt64(50), Timestamp: at,
			}},
			Asks: []kraken.Level3Order{{
				OrderID: "a1", LimitPrice: decimal.NewFromFloat64(1.01),
				OrderQty: decimal.NewFromInt64(400), Timestamp: at,
			}},
		}}}
		So(managed.Update(payload), ShouldBeNil)

		price := &Price{Books: managed}
		ratio, ok := price.BackingRatio("THIN/USD")
		So(ok, ShouldBeTrue)
		So(ratio, ShouldBeLessThan, system.Cfg.Risk.MinBackingRatio)
	})

	Convey("bid cash at least matching ask cash clears the ratio", t, func() {
		managed := newBookFixture(t, "FIRM/USD", 2, 0)
		So(managed.Create("FIRM/USD", 10), ShouldBeNil)

		at := time.Now().UTC()
		payload := &kraken.Level3{Data: []kraken.Level3Data{{
			Symbol: "FIRM/USD",
			Bids: []kraken.Level3Order{
				{
					OrderID: "b1", LimitPrice: decimal.NewFromFloat64(1.00),
					OrderQty: decimal.NewFromInt64(300), Timestamp: at,
				},
				{
					OrderID: "b2", LimitPrice: decimal.NewFromFloat64(0.99),
					OrderQty: decimal.NewFromInt64(200), Timestamp: at,
				},
			},
			Asks: []kraken.Level3Order{{
				OrderID: "a1", LimitPrice: decimal.NewFromFloat64(1.01),
				OrderQty: decimal.NewFromInt64(200), Timestamp: at,
			}},
		}}}
		So(managed.Update(payload), ShouldBeNil)

		price := &Price{Books: managed}
		ratio, ok := price.BackingRatio("FIRM/USD")
		So(ok, ShouldBeTrue)
		So(ratio, ShouldBeGreaterThanOrEqualTo, system.Cfg.Risk.MinBackingRatio)
	})
}

func TestBumpToVenueMin(t *testing.T) {
	Convey("bumps to qty_min when budget and touch-bid carry allow it", t, func() {
		managed := newBookFixture(t, "UAI/USD", 5, 5)
		So(managed.Create("UAI/USD", 10), ShouldBeNil)

		at := time.Now().UTC()
		So(managed.Update(&kraken.Level3{Data: []kraken.Level3Data{{
			Symbol: "UAI/USD",
			Bids: []kraken.Level3Order{{
				OrderID: "b1", LimitPrice: decimal.NewFromFloat64(1.00),
				OrderQty: decimal.NewFromInt64(60), Timestamp: at,
			}},
			Asks: []kraken.Level3Order{{
				OrderID: "a1", LimitPrice: decimal.NewFromFloat64(1.01),
				OrderQty: decimal.NewFromInt64(40), Timestamp: at,
			}},
		}}}), ShouldBeNil)

		price := &Price{normalizer: spot.NewNormalizer()}
		pair := kraken.InstrumentPair{
			QtyMin:  decimal.NewFromInt64(17),
			CostMin: decimal.NewFromFloat64(1),
		}
		fee := decimal.NewFromFloat64(0.4)
		budget := decimal.NewFromFloat64(50)
		touchCap := decimal.NewFromInt64(20)

		var filled *decimal.Decimal
		var why string

		managed.Book("UAI/USD", func(book *spotbook.Book) {
			var gross *decimal.Decimal
			filled, gross, why = price.bumpToVenueMin(
				book, "UAI/USD", pair, fee, budget, touchCap,
			)
			So(gross, ShouldNotBeNil)
		})

		So(why, ShouldEqual, "")
		So(filled, ShouldNotBeNil)
		So(filled.Cmp(pair.QtyMin), ShouldBeGreaterThanOrEqualTo, 0)
	})

	Convey("cost_min with coarse scale does not panic on a fine ask", t, func() {
		managed := newBookFixture(t, "DNA/USD", 8, 5)
		So(managed.Create("DNA/USD", 10), ShouldBeNil)

		at := time.Now().UTC()
		So(managed.Update(&kraken.Level3{Data: []kraken.Level3Data{{
			Symbol: "DNA/USD",
			Bids: []kraken.Level3Order{{
				OrderID: "b1", LimitPrice: decimal.NewFromFloat64(0.00042),
				OrderQty: decimal.NewFromInt64(1_000_000), Timestamp: at,
			}},
			Asks: []kraken.Level3Order{{
				OrderID: "a1", LimitPrice: decimal.NewFromFloat64(0.00043),
				OrderQty: decimal.NewFromInt64(1_000_000), Timestamp: at,
			}},
		}}}), ShouldBeNil)

		price := &Price{normalizer: spot.NewNormalizer()}
		pair := kraken.InstrumentPair{
			QtyMin:         decimal.NewFromInt64(1),
			CostMin:        decimal.NewFromInt64(1),
			PricePrecision: 8,
			QtyPrecision:   5,
		}

		So(func() {
			managed.Book("DNA/USD", func(book *spotbook.Book) {
				_, _, _ = price.bumpToVenueMin(
					book, "DNA/USD", pair,
					decimal.NewFromFloat64(0.4),
					decimal.NewFromFloat64(50),
					decimal.NewFromInt64(300_000),
				)
			})
		}, ShouldNotPanic)
	})

	Convey("refuses bump when qty_min exceeds the touch-bid carry cap", t, func() {
		managed := newBookFixture(t, "ARB/USD", 5, 5)
		So(managed.Create("ARB/USD", 10), ShouldBeNil)

		at := time.Now().UTC()
		So(managed.Update(&kraken.Level3{Data: []kraken.Level3Data{{
			Symbol: "ARB/USD",
			Bids: []kraken.Level3Order{{
				OrderID: "b1", LimitPrice: decimal.NewFromFloat64(1.00),
				OrderQty: decimal.NewFromInt64(30), Timestamp: at,
			}},
			Asks: []kraken.Level3Order{{
				OrderID: "a1", LimitPrice: decimal.NewFromFloat64(1.01),
				OrderQty: decimal.NewFromInt64(40), Timestamp: at,
			}},
		}}}), ShouldBeNil)

		price := &Price{normalizer: spot.NewNormalizer()}
		pair := kraken.InstrumentPair{
			QtyMin:  decimal.NewFromInt64(25),
			CostMin: decimal.NewFromFloat64(1),
		}

		var why string

		managed.Book("ARB/USD", func(book *spotbook.Book) {
			_, _, why = price.bumpToVenueMin(
				book, "ARB/USD", pair,
				decimal.NewFromFloat64(0.4),
				decimal.NewFromFloat64(100),
				decimal.NewFromInt64(10),
			)
		})

		So(why, ShouldContainSubstring, "touch-bid carry cap")
	})
}
