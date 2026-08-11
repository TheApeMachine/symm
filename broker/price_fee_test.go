package broker

import (
	"testing"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/tests/mock"
)

/* priceFeeConn returns the requested TradeVolume fixture. */
type priceFeeConn struct {
	*mock.Conn
	result *kraken.TradeVolumeResult
}

func (conn *priceFeeConn) TradeVolume(
	symbols []string,
) (*kraken.TradeVolumeResult, error) {
	return conn.result, nil
}

/* newTradeVolumePrice builds the ambiguous compact-pair fixture. */
func newTradeVolumePrice(
	testCase testing.TB,
	result *kraken.TradeVolumeResult,
) *Price {
	testCase.Helper()
	conn := &priceFeeConn{Conn: mock.NewConn(), result: result}
	api := websocket.NewAPI(testCase.Context(), conn, conn)
	api.Normalizer().Update(&spot.AssetsManagerUpdate{
		NewAssets: map[string]spot.AssetInfo{
			"BTC":     {AltName: "XBT"},
			"ME":      {AltName: "ME"},
			"MERL":    {AltName: "MERL"},
			"MERLUSD": {AltName: "MERLUSD"},
			"RLUSD":   {AltName: "RLUSD"},
			"USD":     {AltName: "USD"},
		},
		OldAssets: map[string]spot.AssetInfo{
			"MERL": {AltName: "MERL"},
			"XXBT": {AltName: "XBT"},
			"ZUSD": {AltName: "USD"},
		},
		NewPairs: map[string]spot.AssetPair{
			"BTC/USD": {
				AltName: "XBTUSD", WSName: "XBT/USD",
				Base: "BTC", Quote: "USD",
			},
			"MERL/USD": {
				AltName: "MERLUSD", WSName: "MERL/USD",
				Base: "MERL", Quote: "USD",
			},
		},
		OldPairs: map[string]spot.AssetPair{
			"XXBTZUSD": {
				AltName: "XBTUSD", WSName: "XBT/USD",
				Base: "XXBT", Quote: "ZUSD",
			},
			"MERLUSD": {
				AltName: "MERLUSD", WSName: "MERL/USD",
				Base: "MERL", Quote: "ZUSD",
			},
		},
	})

	return NewPrice(api)
}

func TestPriceFee(t *testing.T) {
	Convey("Setup", t, func() {
		price, _ := newPriceSurface(t, "TEST6")

		Convey("When the fee is requested for a symbol", func() {
			fee := price.Fee("TEST6")

			Convey("It should return the taker fee for that symbol", func() {
				So(fee.Fee.String(), ShouldEqual, "0.25")
			})
		})
	})
}

func TestPriceGetFees(t *testing.T) {
	Convey("Given compact fee keys that collide with asset names", t, func() {
		fee := kraken.TradeVolumeFee{Fee: decimal.NewFromFloat64(0.26)}
		price := newTradeVolumePrice(t, &kraken.TradeVolumeResult{
			Fees: map[string]kraken.TradeVolumeFee{
				"MERLUSD":  fee,
				"XXBTZUSD": fee,
			},
		})

		Convey("When the requested pair fees are loaded", func() {
			err := price.GetFees([]string{"MERL/USD", "BTC/USD"})

			Convey("It should store each row under the requested websocket symbol", func() {
				So(err, ShouldBeNil)
				So(price.Fee("MERL/USD"), ShouldResemble, &fee)
				So(price.Fee("BTC/USD"), ShouldResemble, &fee)
			})
		})
	})

	Convey("Given an incomplete fee response", t, func() {
		fee := kraken.TradeVolumeFee{Fee: decimal.NewFromFloat64(0.26)}
		price := newTradeVolumePrice(t, &kraken.TradeVolumeResult{
			Fees: map[string]kraken.TradeVolumeFee{"XXBTZUSD": fee},
		})

		Convey("When the requested pair fee is absent", func() {
			err := price.GetFees([]string{"BTC/USD", "MERL/USD"})
			_, stored := price.fees.Load("BTC/USD")

			Convey("It should reject the batch without storing partial state", func() {
				So(err, ShouldNotBeNil)
				So(err.Error(), ShouldContainSubstring, "MERL/USD")
				So(stored, ShouldBeFalse)
			})
		})
	})
}

func TestPriceWithFee(t *testing.T) {
	Convey("Setup", t, func() {
		price, _ := newPriceSurface(t, "TEST5")

		Convey("Given some ticker data", func() {
			ticker := &kraken.TickerData{
				Symbol: "TEST5",
				Ask:    decimal.NewFromFloat64(70000.00),
				Bid:    decimal.NewFromFloat64(69950.00),
			}

			price.Update(ticker)

			Convey("When the price with fee is calculated for buying", func() {
				priceWithFee := price.WithFee("TEST5", ticker.Ask, BUY)

				Convey("It should return the price with the taker fee applied", func() {
					So(priceWithFee.Float64(), ShouldAlmostEqual, 70175, 1e-12)
				})
			})

			Convey("When the price with fee is calculated for selling", func() {
				priceWithFee := price.WithFee("TEST5", ticker.Bid, SELL)

				Convey("It should return the price with the taker fee applied", func() {
					So(priceWithFee.Float64(), ShouldAlmostEqual, 69775.125, 1e-12)
				})
			})
		})
	})
}

func BenchmarkPriceFee(b *testing.B) {
	price, _ := newPriceSurface(b, "TEST13")

	b.ResetTimer()

	for b.Loop() {
		price.Fee("TEST13")
	}
}

func BenchmarkPriceResolveFee(b *testing.B) {
	fee := kraken.TradeVolumeFee{Fee: decimal.NewFromFloat64(0.26)}
	fees := map[string]kraken.TradeVolumeFee{
		"MERLUSD":  fee,
		"XXBTZUSD": fee,
	}
	price := newTradeVolumePrice(b, &kraken.TradeVolumeResult{Fees: fees})

	b.ResetTimer()

	for b.Loop() {
		if _, err := price.resolveFee("MERL/USD", fees); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkPriceWithFee(b *testing.B) {
	price, _ := newPriceSurface(b, "TEST12")
	ticker := &kraken.TickerData{
		Symbol: "TEST12",
		Ask:    decimal.NewFromFloat64(130000.00),
		Bid:    decimal.NewFromFloat64(129950.00),
	}
	price.Update(ticker)

	b.ResetTimer()

	for b.Loop() {
		price.WithFee("TEST12", ticker.Ask, BUY)
	}
}
