package broker

import (
	"math/big"
	"testing"
	"time"

	spotbook "github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/types"
)

/* pricingFixture shares the actual resident L3 book across quote and execution tests. */
func pricingFixture(t testing.TB) (*Pricing, *Price, *mockConn) {
	t.Helper()
	conn := newMockConn()
	price := newTestPrice(t, websocket.NewAPI(t.Context(), conn, conn))
	price.fees.Store("EDGE/USD", kraken.TradeVolumeFee{Fee: mustDecimal("1")})
	price.Update(&kraken.TickerData{Symbol: "EDGE/USD", Ask: mustDecimal("101"), Bid: mustDecimal("100"), AskQty: 100})
	conn.ApplyLevel3(kraken.Level3Data{Symbol: "EDGE/USD", Type: "snapshot",
		Bids: []kraken.Level3Order{mustDecimalOrder("bid", "100", "10")},
		Asks: []kraken.Level3Order{mustDecimalOrder("ask", "101", "3"), mustDecimalOrder("deep", "102", "10")},
	})
	pricing := &Pricing{}

	if err := pricing.Configure(kraken.InstrumentPair{QtyIncrement: mustDecimal("1"), QtyMin: mustDecimal("1"), CostMin: mustDecimal("1")}, mustDecimal("1")); err != nil {
		t.Fatal(err)
	}
	return pricing, price, conn
}

func TestPricingSetFee(t *testing.T) {
	Convey("Given an explicit percentage schedule", t, func() {
		var pricing Pricing

		for _, invalid := range []*decimal.Decimal{nil, mustDecimal("-0.1"), mustDecimal("100")} {
			So(pricing.SetFee(invalid), ShouldNotBeNil)
		}
		So(pricing.SetFee(mustDecimal("0")), ShouldBeNil)
		So(pricing.Rate.Sign(), ShouldEqual, 0)
		So(pricing.SetFee(mustDecimal("0.8")), ShouldBeNil)
		So(pricing.Rate.RatString(), ShouldEqual, "1/125")
	})
}

func TestPricingTotal(t *testing.T) {
	Convey("Given exact fees on a tiny notional", t, func() {
		var pricing Pricing
		So(pricing.SetFee(mustDecimal("0.8")), ShouldBeNil)
		gross := mustDecimal("0.000000000000011").Rat()
		entry := pricing.Total(new(big.Rat), gross, true)
		exit := pricing.Total(new(big.Rat), gross, false)
		So(entry.Cmp(mustDecimal("0.000000000000011088").Rat()), ShouldEqual, 0)
		So(exit.Cmp(mustDecimal("0.000000000000010912").Rat()), ShouldEqual, 0)

		Convey("Aliased input and output retain the same result", func() {
			pricing.Total(gross, gross, false)
			So(gross.Cmp(exit), ShouldEqual, 0)
		})

		Convey("The live surface uses identical arithmetic", func() {
			price, _ := newPriceSurface(t, "EDGE/USD")
			price.fees.Store("EDGE/USD", kraken.TradeVolumeFee{Fee: mustDecimal("0.8")})
			So(price.WithFee("EDGE/USD", PriceDecimal(gross), BUY).Rat().Cmp(entry), ShouldEqual, 0)
			So(price.WithFee("EDGE/USD", PriceDecimal(gross), SELL).Rat().Cmp(exit), ShouldEqual, 0)
			So(price.WithFee("EDGE/USD", PriceDecimal(gross), Direction("invalid")), ShouldBeNil)
		})
	})
}

func TestPricingSweep(t *testing.T) {
	Convey("Given shared live depth and fee-inclusive budget", t, func() {
		pricing, price, conn := pricingFixture(t)
		conn.Book("EDGE/USD", func(book *spotbook.Book) {
			requested := big.NewRat(5, 1)
			quantity, gross := pricing.Sweep(book, requested, big.NewRat(1000, 1), true, nil, nil)
			So(quantity.Cmp(requested), ShouldEqual, 0)
			So(gross.RatString(), ShouldEqual, "507")
			cost, err := price.EntryCost("EDGE/USD", mustDecimal("5"))
			So(err, ShouldBeNil)
			So(cost.GrossNotional.Rat().Cmp(gross), ShouldEqual, 0)
			So(cost.EntryFee.Rat().Cmp(pricing.Fee(new(big.Rat), gross)), ShouldEqual, 0)

			Convey("A repeating VWAP cannot change the underlying notional or fee", func() {
				cost, err := price.EntryCost("EDGE/USD", mustDecimal("13"))
				So(err, ShouldBeNil)
				So(cost.GrossNotional.Rat().RatString(), ShouldEqual, "1323")
				So(cost.EntryFee.Rat().RatString(), ShouldEqual, "1323/100")
			})

			Convey("Exactly one fee-inclusive lot can be purchased", func() {
				quantity, gross = pricing.Sweep(book, requested, big.NewRat(10201, 100), true, nil, nil)
				So(quantity.RatString(), ShouldEqual, "1")
				So(gross.RatString(), ShouldEqual, "101")
				quantity, _ = pricing.Sweep(book, requested, big.NewRat(102009, 1000), true, nil, nil)
				So(quantity.Sign(), ShouldEqual, 0)
			})

			Convey("A sale reports partial depth and a complete surface refuses it", func() {
				quantity, gross = pricing.Sweep(book, big.NewRat(11, 1), nil, false, nil, nil)
				So(quantity.RatString(), ShouldEqual, "10")
				So(gross.RatString(), ShouldEqual, "1000")
				surface := &types.ExecutionSurface{}
				pricing.Surface(book, mustDecimal("11"), mustDecimal("99"), surface)
				So(surface.FullyExecutable, ShouldBeFalse)
				So(surface.ExecutableVWAP, ShouldBeNil)
				pricing.Surface(book, mustDecimal("5"), mustDecimal("99"), surface)
				So(surface.ExecutableValue.Rat().RatString(), ShouldEqual, "495")
			})
		})
	})
}

func TestPriceDecimal(t *testing.T) {
	Convey("Given finite amounts beyond the SDK default scale", t, func() {
		for _, amount := range []string{"0.000000000000000125", "-0.000000000000000125", "123456789.000000000000000125"} {
			original := mustDecimal(amount)
			So(PriceDecimal(original.Rat()).Rat().Cmp(original.Rat()), ShouldEqual, 0)
		}

		Convey("Repeating quotients use documented SDK bankers rounding", func() {
			So(PriceDecimal(big.NewRat(1, 3)).Cmp(mustDecimal("0.333333333333")), ShouldEqual, 0)
		})
	})
}

func TestProrate(t *testing.T) {
	Convey("Given a tiny authoritative fee reduced across several sales", t, func() {
		fee := mustDecimal("0.000000000000000125")
		remaining := Prorate(fee, mustDecimal("3"), mustDecimal("5"))
		So(remaining.Rat().Cmp(mustDecimal("0.000000000000000075").Rat()), ShouldEqual, 0)
		remaining = Prorate(remaining, mustDecimal("1"), mustDecimal("3"))
		So(remaining.Rat().Cmp(mustDecimal("0.000000000000000025").Rat()), ShouldEqual, 0)
	})
}

func TestUnitPrice(t *testing.T) {
	Convey("Given different price and quantity scales", t, func() {
		unit, quantity := mustDecimal("0.0000000000000011"), mustDecimal("0.00001")
		cost := Notional(unit, quantity)
		So(cost.Sign(), ShouldEqual, 1)
		So(UnitPrice(cost, quantity).Rat().Cmp(unit.Rat()), ShouldEqual, 0)
	})
}

func TestOrderQuantity(t *testing.T) {
	Convey("Given the REST pair lot multiplier and precision", t, func() {
		pair := &spot.AssetPair{LotDecimals: 8, LotMultiplier: 5}
		quantity, err := OrderQuantity(mustDecimal("0.999999999999999999"), mustDecimal("1"), pair)
		So(err, ShouldBeNil)
		So(quantity.String(), ShouldEqual, "0.99999995")

		Convey("A missing lot rule fails instead of guessing a unit lot", func() {
			pair.LotMultiplier = 0
			_, err := OrderQuantity(mustDecimal("1"), mustDecimal("1"), pair)
			So(err, ShouldNotBeNil)
		})
	})
}

func BenchmarkPricingSweep(b *testing.B) {
	pricing, _, conn := pricingFixture(b)
	requested, cash := big.NewRat(5, 1), big.NewRat(1000, 1)
	conn.Book("EDGE/USD", func(book *spotbook.Book) {
		b.ReportAllocs()
		b.ResetTimer()

		for b.Loop() {
			pricing.Sweep(book, requested, cash, true, nil, nil)
		}
	})
}

func BenchmarkProrate(b *testing.B) {
	amount, remaining, original := mustDecimal("0.000000000000000125"), mustDecimal("3"), mustDecimal("5")
	b.ReportAllocs()

	for b.Loop() {
		Prorate(amount, remaining, original)
	}
}

func BenchmarkPricingSurface(b *testing.B) {
	pricing, _, conn := pricingFixture(b)
	quantity, floor := mustDecimal("5"), mustDecimal("99")
	surface := &types.ExecutionSurface{At: time.Now()}
	conn.Book("EDGE/USD", func(book *spotbook.Book) {
		b.ReportAllocs()
		b.ResetTimer()

		for b.Loop() {
			pricing.Surface(book, quantity, floor, surface)
		}
	})
}
