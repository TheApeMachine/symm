package types

import (
	"fmt"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
)

func TestThesisMarketTickers(t *testing.T) {
	Convey("Given ticker observations not yet incorporated by a signal", t, func() {
		thesis := NewThesis(t.Context(), nil)
		bitcoinAt := time.Unix(10, 0).UTC()
		altAt := time.Unix(9, 0).UTC()
		thesis.AppendTicker(kraken.TickerData{Symbol: "BTC/USD", Timestamp: bitcoinAt})
		thesis.AppendTicker(kraken.TickerData{Symbol: "ALT/USD", Timestamp: altAt})

		first := thesis.MarketTickers(SourceCorrelation)
		second := thesis.MarketTickers(SourceCorrelation)

		Convey("Then reading alone should not consume the observations", func() {
			So(first, ShouldHaveLength, 2)
			So(second, ShouldHaveLength, 2)
		})

		thesis.AppendMeasurements(SourceCorrelation, []*Measurement{{
			Source: SourceCorrelation,
			Symbol: "BTC/USD",
			At:     bitcoinAt,
		}}, true)
		thesis.AppendTicker(kraken.TickerData{
			Symbol:    "ALT/USD",
			Timestamp: altAt.Add(time.Second),
		})

		Convey("Then only a symbol named by the artifact should be committed", func() {
			unseen := thesis.MarketTickers(SourceCorrelation)
			So(unseen, ShouldHaveLength, 2)
			So(unseen[0].Symbol, ShouldEqual, "ALT/USD")
			So(unseen[1].Symbol, ShouldEqual, "ALT/USD")
			So(thesis.MarketTickers(SourceSentiment), ShouldHaveLength, 3)
		})

		Convey("Then a peer named by a cross-symbol artifact should also be committed", func() {
			So(thesis.MarketTickers(SourceLeadLag), ShouldHaveLength, 3)
			thesis.AppendMeasurements(SourceLeadLag, []*Measurement{{
				Source: SourceLeadLag,
				Symbol: "BTC/USD",
				Peer:   "ALT/USD",
				At:     bitcoinAt,
			}}, true)
			So(thesis.MarketTickers(SourceLeadLag), ShouldBeEmpty)
		})
	})
}

func TestThesisMarketTrades(t *testing.T) {
	Convey("Given distinct trades sharing one exchange timestamp", t, func() {
		thesis := NewThesis(t.Context(), nil)
		observedAt := time.Unix(20, 0).UTC()
		firstTrade := kraken.TradeData{
			Symbol: "BTC/USD", TradeID: 1, Timestamp: observedAt,
		}
		thesis.AppendTrade(firstTrade)

		first := thesis.MarketTrades(SourceHawkes)
		second := thesis.MarketTrades(SourceHawkes)

		Convey("Then an uncommitted trade should remain available", func() {
			So(first, ShouldHaveLength, 1)
			So(second, ShouldHaveLength, 1)
			So(second[0].TradeID, ShouldEqual, firstTrade.TradeID)
		})

		thesis.AppendMeasurements(SourceHawkes, []*Measurement{{
			Source: SourceHawkes,
			Symbol: "BTC/USD",
			At:     observedAt,
		}}, false)
		thesis.AppendTrade(kraken.TradeData{
			Symbol: "BTC/USD", TradeID: 2, Timestamp: observedAt,
		})
		thesis.AppendTrade(kraken.TradeData{
			Symbol: "BTC/USD", TradeID: 3, Timestamp: observedAt.Add(time.Second),
		})

		Convey("Then only unseen IDs and later epochs should remain", func() {
			unseen := thesis.MarketTrades(SourceHawkes)
			So(unseen, ShouldHaveLength, 2)
			So(unseen[0].TradeID, ShouldEqual, 2)
			So(unseen[1].TradeID, ShouldEqual, 3)
			So(thesis.MarketTrades(SourceCVD), ShouldHaveLength, 3)
		})
	})
}

func BenchmarkThesisMarketTickers(b *testing.B) {
	const (
		// The benchmark exercises a cross-section large enough to expose the
		// per-symbol sorting and cursor work without modeling exchange traffic.
		benchmarkSymbols       = 16
		benchmarkRowsPerSymbol = 64
	)

	thesis := NewThesis(b.Context(), nil)
	observedAt := time.Unix(1_700_000_000, 0).UTC()

	for symbolIndex := range benchmarkSymbols {
		symbol := fmt.Sprintf("SIM%d/USD", symbolIndex)

		for rowIndex := range benchmarkRowsPerSymbol {
			thesis.AppendTicker(kraken.TickerData{
				Symbol:    symbol,
				Timestamp: observedAt.Add(time.Duration(rowIndex) * time.Millisecond),
			})
		}
	}

	b.ReportAllocs()

	for b.Loop() {
		if len(thesis.MarketTickers(SourceCorrelation)) == 0 {
			b.Fatal("expected uncommitted benchmark tickers")
		}
	}
}

func BenchmarkThesisMarketTrades(b *testing.B) {
	const (
		// The benchmark exercises a cross-section large enough to expose the
		// per-symbol sorting and cursor work without modeling exchange traffic.
		benchmarkSymbols       = 16
		benchmarkRowsPerSymbol = 64
	)

	thesis := NewThesis(b.Context(), nil)
	observedAt := time.Unix(1_700_000_000, 0).UTC()

	for symbolIndex := range benchmarkSymbols {
		symbol := fmt.Sprintf("SIM%d/USD", symbolIndex)

		for rowIndex := range benchmarkRowsPerSymbol {
			thesis.AppendTrade(kraken.TradeData{
				Symbol:    symbol,
				TradeID:   int64(rowIndex + 1),
				Timestamp: observedAt.Add(time.Duration(rowIndex) * time.Millisecond),
			})
		}
	}

	b.ReportAllocs()

	for b.Loop() {
		if len(thesis.MarketTrades(SourceHawkes)) == 0 {
			b.Fatal("expected uncommitted benchmark trades")
		}
	}
}
