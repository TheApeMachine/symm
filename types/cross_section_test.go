package types

import (
	"fmt"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/nomagique/correlation"
	"github.com/theapemachine/symm/kraken"

	. "github.com/smartystreets/goconvey/convey"
)

func liquidityTestRow(
	symbol string, bid, ask, bidQty, askQty, volume, vwap float64,
) kraken.TickerData {
	return kraken.TickerData{
		Symbol:    symbol,
		Bid:       decimal.NewFromFloat64(bid),
		BidQty:    bidQty,
		Ask:       decimal.NewFromFloat64(ask),
		AskQty:    askQty,
		Last:      decimal.NewFromFloat64((bid + ask) / 2),
		Volume:    volume,
		Vwap:      vwap,
		Timestamp: time.Now(),
	}
}

func TestQuoteNotional(t *testing.T) {
	Convey("Given a row with a reported vwap", t, func() {
		row := liquidityTestRow("BTC/USD", 99, 101, 1, 1, 10, 100)

		Convey("When QuoteNotional values it", func() {
			notional := QuoteNotional(row)

			Convey("Then it multiplies volume by vwap, not the mid price", func() {
				So(notional, ShouldEqual, 1000)
			})
		})
	})

	Convey("Given a row with no vwap reported yet", t, func() {
		row := liquidityTestRow("BTC/USD", 99, 101, 1, 1, 10, 0)

		Convey("When QuoteNotional values it", func() {
			notional := QuoteNotional(row)

			Convey("Then it falls back to the last trade price", func() {
				So(notional, ShouldEqual, 10*row.Last.Float64())
			})
		})
	})

	Convey("Given a row with no volume", t, func() {
		row := liquidityTestRow("BTC/USD", 99, 101, 1, 1, 0, 100)

		Convey("When QuoteNotional values it", func() {
			Convey("Then it is zero rather than dividing by an absent quantity", func() {
				So(QuoteNotional(row), ShouldEqual, 0)
			})
		})
	})
}

func TestExecutableDepth(t *testing.T) {
	Convey("Given a two-sided quote with asymmetric quantities", t, func() {
		row := liquidityTestRow("BTC/USD", 99, 101, 5, 2, 10, 100)

		Convey("When ExecutableDepth values it", func() {
			depth := ExecutableDepth(row)

			Convey("Then it uses the smaller side, valued at the mid price", func() {
				So(depth, ShouldEqual, 2*100)
			})
		})
	})

	Convey("Given a one-sided quote with no bid", t, func() {
		row := kraken.TickerData{
			Symbol: "BTC/USD",
			Bid:    decimal.NewFromFloat64(0),
			BidQty: 0,
			Ask:    decimal.NewFromFloat64(101),
			AskQty: 5,
		}

		Convey("When ExecutableDepth values it", func() {
			Convey("Then it is zero rather than pricing a one-sided book", func() {
				So(ExecutableDepth(row), ShouldEqual, 0)
			})
		})
	})
}

func TestCrossSectionObserveDedupesRepeatedTimestamp(t *testing.T) {
	Convey("Given a cross-section that already observed a symbol at a given timestamp", t, func() {
		crossSection, err := NewCrossSection(DefaultCrossSectionConfig())
		So(err, ShouldBeNil)

		at := time.Now()
		row := liquidityTestRow("BTC/USD", 999, 1001, 2, 2, 1, 1000)
		row.Timestamp = at

		crossSection.ProcessUpdates([]kraken.TickerData{row})

		Convey("When another signal observes the same timestamp again this tick", func() {
			crossSection.ProcessUpdates([]kraken.TickerData{row})

			Convey("Then the repeated observation is not appended twice", func() {
				dst := make([]correlation.Sample, crossSection.MaxReturnWindow()+1)
				So(crossSection.SymbolSamples("BTC/USD", dst), ShouldEqual, 1)
			})
		})

		Convey("When a genuinely later timestamp arrives", func() {
			later := row
			later.Timestamp = at.Add(time.Second)
			crossSection.ProcessUpdates([]kraken.TickerData{later})

			Convey("Then it is appended as a second observation", func() {
				dst := make([]correlation.Sample, crossSection.MaxReturnWindow()+1)
				So(crossSection.SymbolSamples("BTC/USD", dst), ShouldEqual, 2)
			})
		})
	})
}

/*
BenchmarkCrossSectionTick replays one tick against a warm 200-symbol
universe: every symbol's ring is already full, and the subject symbol
scores every peer's returns and samples, mirroring what Section.Scores
does once per symbol every tick in correlation. This is the steady-state
hot path the ring buffers and cached float64 conversions exist for.
*/
func BenchmarkCrossSectionTick(b *testing.B) {
	crossSection, err := NewCrossSection(DefaultCrossSectionConfig())
	if err != nil {
		b.Fatal(err)
	}

	symbolCount := 200
	symbols := make([]string, symbolCount)

	for index := range symbolCount {
		symbols[index] = fmt.Sprintf("SYM%d/USD", index)
	}

	start := time.Now()
	window := crossSection.MaxReturnWindow()

	for bar := range window + 8 {
		rows := make([]kraken.TickerData, symbolCount)

		for index, symbol := range symbols {
			price := 100 + float64(bar) + float64(index)*0.01
			rows[index] = liquidityTestRow(symbol, price-1, price+1, 5, 5, 10, price)
			rows[index].Timestamp = start.Add(time.Duration(bar) * time.Second)
			rows[index].ChangePct = float64(index%7) - 3
		}

		crossSection.ProcessUpdates(rows)
	}

	subject := symbols[0]
	returns := make([]float64, window)
	samples := make([]correlation.Sample, window+1)
	tickRows := make([]kraken.TickerData, symbolCount)

	// Decimals are built once; only the timestamp (a plain time.Time) moves
	// per iteration, so the loop below measures ProcessUpdates and the
	// return/sample reads, not decimal construction.
	for index, symbol := range symbols {
		price := 200 + float64(index)*0.01
		tickRows[index] = liquidityTestRow(symbol, price-1, price+1, 5, 5, 10, price)
	}

	tick := 0

	b.ReportAllocs()

	for b.Loop() {
		tick++

		for index := range tickRows {
			tickRows[index].Timestamp = start.Add(time.Duration(window+100+tick) * time.Second)
		}

		crossSection.ProcessUpdates(tickRows)

		for _, peer := range symbols {
			if peer == subject {
				continue
			}

			crossSection.SymbolReturns(peer, returns)
			crossSection.SymbolSamples(peer, samples)
		}
	}
}

func TestCrossSectionQuoteNotionalsAndExecutableDepths(t *testing.T) {
	Convey("Given a cross-section observing three symbols", t, func() {
		crossSection, err := NewCrossSection(DefaultCrossSectionConfig())
		So(err, ShouldBeNil)

		rows := []kraken.TickerData{
			liquidityTestRow("BTC/USD", 999, 1001, 2, 2, 1, 1000),
			liquidityTestRow("ETH/USD", 99, 101, 5, 5, 100, 100),
			liquidityTestRow("SOL/USD", 99, 101, 5, 5, 100, 100),
		}
		crossSection.ProcessUpdates(rows)

		Convey("When the summary is read", func() {
			metrics := crossSection.ReadView().Metrics

			Convey("Then every observed symbol contributes one row with both axes", func() {
				So(metrics, ShouldHaveLength, 3)

				for _, metric := range metrics {
					So(metric.QuoteNotional, ShouldBeGreaterThan, 0)
					So(metric.ExecutableDepth, ShouldBeGreaterThan, 0)
				}
			})
		})
	})
}
