package trader

import (
	"fmt"
	"testing"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
)

func TestNewLiquidityRanker(t *testing.T) {
	Convey("Given a ticker without a complete two-sided quote", t, func() {
		_, err := NewLiquidityRanker([]kraken.TickerData{{Symbol: "BTC/USD"}})

		Convey("Then candidate construction fails visibly", func() {
			So(err, ShouldNotBeNil)
		})
	})
}

func TestLiquidityRankerRank(t *testing.T) {
	Convey("Given peers with different weakest liquidity dimensions", t, func() {
		ranker, err := NewLiquidityRanker([]kraken.TickerData{
			liquidityTicker("BTC/USD", 90, 110, 10, 10),
			liquidityTicker("ETH/USD", 99, 101, 5, 6),
			liquidityTicker("SOL/USD", 99.99, 100.01, 1, 1),
		})
		So(err, ShouldBeNil)

		Convey("When maximin percentile liquidity is ranked", func() {
			symbols := ranker.Rank(3)

			Convey("Then the balanced peer outranks peers with one severe weakness", func() {
				So(symbols[0], ShouldEqual, "ETH/USD")
				So(symbols, ShouldHaveLength, 3)
			})
		})
	})
}

func BenchmarkLiquidityRankerRank(b *testing.B) {
	tickers := make([]kraken.TickerData, 641)

	for index := range tickers {
		mid := 10 + float64(index)
		spread := 0.01 + float64(index%11)/1000
		tickers[index] = liquidityTicker(
			fmt.Sprintf("ASSET-%03d/USD", index),
			mid-spread,
			mid+spread,
			1+float64(index%17),
			100+float64(index%29),
		)
	}

	b.ReportAllocs()

	for b.Loop() {
		ranker, err := NewLiquidityRanker(tickers)

		if err != nil {
			b.Fatal(err)
		}

		if symbols := ranker.Rank(40); len(symbols) != 40 {
			b.Fatal("incorrect trading tier size")
		}
	}
}

func liquidityTicker(
	symbol string,
	bid float64,
	ask float64,
	quantity float64,
	volume float64,
) kraken.TickerData {
	mid := (bid + ask) / 2

	return kraken.TickerData{
		Symbol: symbol,
		Bid:    decimal.NewFromFloat64(bid),
		BidQty: quantity,
		Ask:    decimal.NewFromFloat64(ask),
		AskQty: quantity,
		Last:   decimal.NewFromFloat64(mid),
		Volume: volume,
		Vwap:   mid,
	}
}
