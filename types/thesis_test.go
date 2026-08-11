package types

import (
	"sync/atomic"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/api-go/v2/pkg/decimal"
	"github.com/theapemachine/symm/kraken"
)

func TestThesisAppendMeasurements(t *testing.T) {
	Convey("Given one ready signal measurement", t, func() {
		thesis := NewThesis(t.Context(), nil)
		analyzer := make(chan struct{}, 1)
		measurement := &Measurement{
			ID: "correlation", Source: SourceCorrelation, Symbol: "BTC/USD",
		}
		incoming := []*Measurement{measurement}

		err := thesis.AppendMeasurements(
			SourceCorrelation, incoming, true,
		)

		Convey("Then it should admit the batch without mutating caller storage", func() {
			So(err, ShouldBeNil)
			So(incoming, ShouldResemble, []*Measurement{measurement})
			So(incoming[0], ShouldNotBeNil)
			So(len(analyzer), ShouldEqual, 0)
			storedSymbol, found := thesis.Symbols.Load("BTC/USD")
			So(found, ShouldBeTrue)
			symbol := storedSymbol.(*Symbol)
			So(symbol.Status, ShouldEqual, READY)
			So(symbol.Measurements, ShouldResemble, []*Measurement{measurement})
			stored, found := thesis.Measurements.Load(SourceCorrelation)
			So(found, ShouldBeTrue)
			So(stored, ShouldBeEmpty)
		})
	})

	Convey("Given an empty measurement pass", t, func() {
		thesis := NewThesis(t.Context(), nil)
		correlation := make(chan struct{}, 1)
		cvd := make(chan struct{}, 1)

		thesis.AppendMeasurements(SourceCorrelation, nil, false)

		Convey("Then it should not create an indirect notification cycle", func() {
			So(len(correlation), ShouldEqual, 0)
			So(len(cvd), ShouldEqual, 0)
		})
	})

	Convey("Given an empty pass that claims readiness", t, func() {
		thesis := NewThesis(t.Context(), nil)
		correlation := make(chan struct{}, 1)
		cvd := make(chan struct{}, 1)
		categories := make(chan struct{}, 1)

		thesis.AppendMeasurements(SourceCorrelation, nil, true)

		Convey("Then it should keep readiness and downstream work pending", func() {
			So(len(correlation), ShouldEqual, 0)
			So(len(cvd), ShouldEqual, 0)
			So(len(categories), ShouldEqual, 0)
		})
	})

	Convey("Given multiple independent symbols in the first ready batch", t, func() {
		thesis := NewThesis(t.Context(), nil)
		measurements := []*Measurement{
			{ID: "btc", Source: SourceLeadLag, Symbol: "BTC/USD", At: time.Unix(1, 0)},
			{ID: "eth", Source: SourceLeadLag, Symbol: "ETH/USD", At: time.Unix(2, 0)},
			{ID: "sol", Source: SourceLeadLag, Symbol: "SOL/USD", At: time.Unix(3, 0)},
		}

		thesis.AppendMeasurements(SourceLeadLag, measurements, true)

		Convey("Then it should admit every row exactly once without skipping", func() {
			stored, found := thesis.Measurements.Load(SourceLeadLag)
			So(found, ShouldBeTrue)
			So(stored, ShouldBeEmpty)
			So(measurements[0], ShouldNotBeNil)
			So(measurements[1], ShouldNotBeNil)
			So(measurements[2], ShouldNotBeNil)

			for _, measurement := range measurements {
				storedSymbol, found := thesis.Symbols.Load(measurement.Symbol)
				So(found, ShouldBeTrue)
				So(storedSymbol.(*Symbol).Status, ShouldEqual, READY)
				So(storedSymbol.(*Symbol).Measurements, ShouldHaveLength, 1)
			}
		})
	})

	Convey("Given one symbol measured by every ready signal", t, func() {
		thesis := NewThesis(t.Context(), nil)
		analyzer := make(chan struct{}, 1)
		sources := []SourceType{
			SourceCorrelation,
			SourceCVD,
			SourceDepthFlow,
			SourceExhaustion,
			SourceHawkes,
			SourceLeadLag,
			SourceLiquidity,
			SourcePumpDump,
			SourceSentiment,
			SourceToxicity,
		}

		for index, source := range sources {
			err := thesis.AppendMeasurements(source, []*Measurement{{
				ID: string(source), Source: source, Symbol: "BTC/USD", At: time.Unix(1, 0),
			}}, true)
			So(err, ShouldBeNil)

			if index < len(sources)-1 {
				stored, _ := thesis.Symbols.Load("BTC/USD")
				So(stored.(*Symbol).Status, ShouldEqual, READY)
				So(len(analyzer), ShouldEqual, 0)
			}
		}

		Convey("Then the complete cut should lock and notify the analyzer once", func() {
			pending := 0
			thesis.Measurements.Range(func(_, value any) bool {
				pending += len(value.([]*Measurement))
				return true
			})
			So(pending, ShouldEqual, 0)
			stored, found := thesis.Symbols.Load("BTC/USD")
			So(found, ShouldBeTrue)
			symbol := stored.(*Symbol)
			So(symbol.Status, ShouldEqual, BUSY)
			So(symbol.Measurements, ShouldHaveLength, len(sources))
			So(len(analyzer), ShouldEqual, 1)
		})
	})

	Convey("Given new measurements while a symbol is busy", t, func() {
		thesis := NewThesis(t.Context(), nil)
		analyzer := make(chan struct{}, 1)
		sources := []SourceType{
			SourceCorrelation, SourceCVD, SourceDepthFlow, SourceExhaustion, SourceHawkes,
			SourceLeadLag, SourceLiquidity, SourcePumpDump, SourceSentiment, SourceToxicity,
		}

		for _, source := range sources {
			So(thesis.AppendMeasurements(source, []*Measurement{{
				ID: string(source), Source: source, Symbol: "BTC/USD",
			}}, true), ShouldBeNil)
		}

		<-analyzer
		pending := &Measurement{
			ID: "correlation-pending", Source: SourceCorrelation, Symbol: "BTC/USD",
		}
		So(thesis.AppendMeasurements(
			SourceCorrelation, []*Measurement{pending}, true,
		), ShouldBeNil)

		Convey("Then it should retain the new row for the next cut without another wake-up", func() {
			stored, _ := thesis.Symbols.Load("BTC/USD")
			symbol := stored.(*Symbol)
			So(symbol.Measurements, ShouldHaveLength, len(sources))
			queued, found := thesis.Measurements.Load(SourceCorrelation)
			So(found, ShouldBeTrue)
			So(queued, ShouldResemble, []*Measurement{pending})
			So(len(analyzer), ShouldEqual, 0)
		})
	})

	Convey("Given provisional measurements for independent symbols", t, func() {
		thesis := NewThesis(t.Context(), nil)
		bitcoin := &Measurement{
			ID:     "bitcoin-1",
			Source: SourceHawkes,
			Symbol: "BTC/USD",
			At:     time.Unix(1, 0),
		}
		ether := &Measurement{
			ID:     "ether-1",
			Source: SourceHawkes,
			Symbol: "ETH/USD",
			At:     time.Unix(1, 0),
		}
		updated := &Measurement{
			ID:     "bitcoin-2",
			Source: SourceHawkes,
			Symbol: "BTC/USD",
			At:     time.Unix(2, 0),
		}
		thesis.AppendMeasurements(SourceHawkes, []*Measurement{bitcoin, ether}, false)
		thesis.AppendMeasurements(SourceHawkes, []*Measurement{updated}, false)

		Convey("Then it should retain the latest row for each independent identity", func() {
			stored, found := thesis.Measurements.Load(SourceHawkes)
			So(found, ShouldBeTrue)
			So(stored, ShouldResemble, []*Measurement{ether, updated})
			_, bitcoinFound := thesis.Symbols.Load("BTC/USD")
			_, etherFound := thesis.Symbols.Load("ETH/USD")
			So(bitcoinFound, ShouldBeFalse)
			So(etherFound, ShouldBeFalse)
		})
	})

	Convey("Given multiple peer measurements sharing one symbol", t, func() {
		thesis := NewThesis(t.Context(), nil)
		ether := &Measurement{
			ID: "btc-eth-1", Source: SourceLeadLag,
			Symbol: "BTC/USD", Peer: "ETH/USD", At: time.Unix(1, 0),
		}
		solana := &Measurement{
			ID: "btc-sol-1", Source: SourceLeadLag,
			Symbol: "BTC/USD", Peer: "SOL/USD", At: time.Unix(1, 0),
		}
		updated := &Measurement{
			ID: "btc-eth-2", Source: SourceLeadLag,
			Symbol: "BTC/USD", Peer: "ETH/USD", At: time.Unix(2, 0),
		}
		thesis.AppendMeasurements(SourceLeadLag, []*Measurement{ether, solana}, false)
		thesis.AppendMeasurements(SourceLeadLag, []*Measurement{updated}, false)

		Convey("Then it should retain the unprocessed peer measurements", func() {
			stored, found := thesis.Measurements.Load(SourceLeadLag)
			So(found, ShouldBeTrue)
			So(stored, ShouldResemble, []*Measurement{solana, updated})
		})
	})

	Convey("Given a provisional measurement repeated exactly", t, func() {
		thesis := NewThesis(t.Context(), nil)
		at := time.Unix(1, 0).UTC()
		trade := kraken.TradeData{
			Symbol: "BTC/USD", TradeID: 1, Timestamp: at,
		}
		measurement := &Measurement{
			ID:     "cvd-1",
			Source: SourceCVD,
			Symbol: trade.Symbol,
			At:     at,
		}

		thesis.AppendTrade(trade)
		So(thesis.MarketTrades(SourceCVD), ShouldHaveLength, 1)
		So(thesis.AppendMeasurements(
			SourceCVD, []*Measurement{measurement}, false,
		), ShouldBeNil)

		Convey("Then replaying it should retain one prior rather than duplicate it", func() {
			err := thesis.AppendMeasurements(
				SourceCVD, []*Measurement{measurement}, false,
			)

			So(err, ShouldBeNil)
			stored, found := thesis.Measurements.Load(SourceCVD)
			So(found, ShouldBeTrue)
			So(stored, ShouldResemble, []*Measurement{measurement})
		})
	})

	Convey("Given distinct measurements sharing one timestamp", t, func() {
		thesis := NewThesis(t.Context(), nil)
		at := time.Unix(1, 0).UTC()
		first := &Measurement{
			ID:     "hawkes-1",
			Source: SourceHawkes,
			Symbol: "BTC/USD",
			At:     at,
		}
		updated := &Measurement{
			ID:       "hawkes-2",
			Source:   SourceHawkes,
			Symbol:   "BTC/USD",
			At:       at,
			Maturity: 1,
		}

		So(thesis.AppendMeasurements(
			SourceHawkes, []*Measurement{first}, false,
		), ShouldBeNil)

		Convey("Then the new ID should remain queued behind the active cut", func() {
			err := thesis.AppendMeasurements(
				SourceHawkes, []*Measurement{updated}, false,
			)

			So(err, ShouldBeNil)
			stored, found := thesis.Measurements.Load(SourceHawkes)
			So(found, ShouldBeTrue)
			So(stored, ShouldResemble, []*Measurement{updated})
		})
	})

	Convey("Given a reader holding the prior measurement slice", t, func() {
		thesis := NewThesis(t.Context(), nil)
		bitcoin := &Measurement{
			ID:     "bitcoin-1",
			Source: SourceHawkes,
			Symbol: "BTC/USD",
			At:     time.Unix(1, 0),
		}
		ether := &Measurement{
			ID: "ether-1", Source: SourceHawkes,
			Symbol: "BTC/USD", Peer: "ETH/USD", At: time.Unix(1, 0),
		}
		updated := &Measurement{
			ID:     "bitcoin-2",
			Source: SourceHawkes,
			Symbol: "BTC/USD",
			At:     time.Unix(2, 0),
		}
		thesis.AppendMeasurements(
			SourceHawkes, []*Measurement{bitcoin, ether}, false,
		)
		stored, _ := thesis.Measurements.Load(SourceHawkes)
		prior := stored.([]*Measurement)
		thesis.AppendMeasurements(
			SourceHawkes, []*Measurement{updated}, false,
		)

		Convey("Then every pointer in the prior view should remain valid", func() {
			So(prior, ShouldResemble, []*Measurement{bitcoin, ether})

			for _, measurement := range prior {
				So(measurement, ShouldNotBeNil)
			}
		})
	})

	Convey("Given a provisional row followed by its completed replacement", t, func() {
		thesis := NewThesis(t.Context(), nil)
		provisional := &Measurement{
			ID: "cvd-prior", Source: SourceCVD, Symbol: "BTC/USD", At: time.Unix(1, 0),
		}
		completed := &Measurement{
			ID: "cvd-final", Source: SourceCVD, Symbol: "BTC/USD", At: time.Unix(2, 0),
		}
		So(thesis.AppendMeasurements(
			SourceCVD, []*Measurement{provisional}, false,
		), ShouldBeNil)

		Convey("Then only the completed row should enter the symbol cut", func() {
			incoming := []*Measurement{completed}
			So(thesis.AppendMeasurements(SourceCVD, incoming, true), ShouldBeNil)
			stored, found := thesis.Symbols.Load("BTC/USD")
			So(found, ShouldBeTrue)
			So(stored.(*Symbol).Measurements, ShouldResemble, []*Measurement{completed})
			So(incoming, ShouldResemble, []*Measurement{completed})
			queued, found := thesis.Measurements.Load(SourceCVD)
			So(found, ShouldBeTrue)
			So(queued, ShouldBeEmpty)
		})
	})

	Convey("Given multiple peer rows from the signal that completes the cut", t, func() {
		thesis := NewThesis(t.Context(), nil)
		analyzer := make(chan struct{}, 1)
		sources := []SourceType{
			SourceCorrelation, SourceCVD, SourceDepthFlow, SourceExhaustion, SourceHawkes,
			SourceLiquidity, SourcePumpDump, SourceSentiment, SourceToxicity,
		}

		for _, source := range sources {
			So(thesis.AppendMeasurements(source, []*Measurement{{
				ID: string(source), Source: source, Symbol: "BTC/USD",
			}}, true), ShouldBeNil)
		}

		peers := []*Measurement{
			{ID: "eth", Source: SourceLeadLag, Symbol: "BTC/USD", Peer: "ETH/USD"},
			{ID: "sol", Source: SourceLeadLag, Symbol: "BTC/USD", Peer: "SOL/USD"},
		}
		So(thesis.AppendMeasurements(SourceLeadLag, peers, true), ShouldBeNil)

		Convey("Then every peer row should be admitted before analysis locks the symbol", func() {
			stored, found := thesis.Symbols.Load("BTC/USD")
			So(found, ShouldBeTrue)
			symbol := stored.(*Symbol)
			So(symbol.Status, ShouldEqual, BUSY)
			So(symbol.Measurements, ShouldHaveLength, len(sources)+len(peers))
			So(symbol.Measurements[len(symbol.Measurements)-2:], ShouldResemble, peers)
			So(len(analyzer), ShouldEqual, 1)
		})
	})
}

func TestThesisFanout(t *testing.T) {
	Convey("Given repeated work for one analyzer", t, func() {
		analyzer := make(chan struct{}, 1)
		status := &atomic.Value{}
		status.Store(READY)

		Convey("Then it should retain one coalesced wake-up", func() {
			So(len(analyzer), ShouldEqual, 1)
			So(status.Load(), ShouldEqual, BUSY)
		})

		Convey("Then it should accept the next wake-up after becoming ready", func() {
			<-analyzer
			status.Store(READY)

			So(len(analyzer), ShouldEqual, 1)
			So(status.Load(), ShouldEqual, BUSY)
		})
	})

	Convey("Given signal and downstream subscribers", t, func() {
		correlation := make(chan struct{}, 1)
		cvd := make(chan struct{}, 1)
		categories := make(chan struct{}, 1)

		Convey("Then signal output should wake every other subscriber", func() {
			So(len(cvd), ShouldEqual, 1)
			So(len(categories), ShouldEqual, 1)
		})

		Convey("Then market input should wake every signal", func() {
			So(len(correlation), ShouldEqual, 1)
			So(len(cvd), ShouldEqual, 1)
		})
	})
}

func TestThesisStamp(t *testing.T) {
	Convey("Given the final logic stamp for a measured symbol", t, func() {
		thesis := NewThesis(t.Context(), nil)
		auditRecords := 0
		thesis.Audit = func(any) error {
			auditRecords++
			return nil
		}
		symbol := NewSymbol("BTC/USD", nil)
		thesis.Symbols.Store("BTC/USD", symbol)

		symbol.Status = BUSY
		auditBefore := auditRecords

		Convey("Then it should release the cut without reopening stale signal work", func() {
			So(symbol.Status, ShouldEqual, READY)
			So(auditRecords, ShouldEqual, auditBefore+1)
		})
	})
}

func TestThesisAppendTicker(t *testing.T) {
	Convey("Given the first ticker observed for a symbol", t, func() {
		thesis := NewThesis(t.Context(), nil)
		ticker := kraken.TickerData{
			Symbol:    "BTC/USD",
			Timestamp: time.Unix(1, 0),
		}
		thesis.AppendTicker(ticker)

		Convey("Then it should retain the ticker and its observation time", func() {
			stored, found := thesis.Tickers.Load("BTC/USD")

			So(found, ShouldBeTrue)
			So(stored, ShouldResemble, []kraken.TickerData{ticker})
			So(thesis.LastTickerAt, ShouldResemble, ticker.Timestamp)
		})
	})
}

func TestThesisAppendTrade(t *testing.T) {
	Convey("Given the first trade observed for a symbol", t, func() {
		thesis := NewThesis(t.Context(), nil)
		trade := kraken.TradeData{
			Symbol:    "BTC/USD",
			Timestamp: time.Unix(1, 0),
		}
		thesis.AppendTrade(trade)

		Convey("Then it should retain the trade and its observation time", func() {
			stored, found := thesis.Trades.Load("BTC/USD")

			So(found, ShouldBeTrue)
			So(stored, ShouldResemble, []kraken.TradeData{trade})
			So(thesis.LastTradeAt, ShouldResemble, trade.Timestamp)
		})
	})
}

func TestThesisMarketTickers(t *testing.T) {
	Convey("Given one ticker frame shared by ticker consumers", t, func() {
		thesis := NewThesis(t.Context(), nil)
		thesis.AppendTicker(kraken.TickerData{
			Symbol: "BTC/USD", Timestamp: time.Unix(1, 0),
		})

		correlation := thesis.MarketTickers(SourceCorrelation)
		thesis.AppendTicker(kraken.TickerData{
			Symbol: "BTC/USD", Timestamp: time.Unix(2, 0),
		})
		correlationNext := thesis.MarketTickers(SourceCorrelation)
		cvd := thesis.MarketTickers(SourceCVD)

		Convey("Then each signal should advance only its own cache cursor", func() {
			So(correlation, ShouldHaveLength, 1)
			So(correlationNext, ShouldHaveLength, 2)
			So(correlationNext[1].Timestamp, ShouldResemble, time.Unix(2, 0))
			So(cvd, ShouldHaveLength, 2)
		})
	})
}

func TestThesisMarketTrades(t *testing.T) {
	Convey("Given one trade frame shared by trade consumers", t, func() {
		thesis := NewThesis(t.Context(), nil)
		thesis.AppendTrade(kraken.TradeData{
			Symbol: "BTC/USD", TradeID: 1, Timestamp: time.Unix(1, 0),
		})

		cvd := thesis.MarketTrades(SourceCVD)
		thesis.AppendTrade(kraken.TradeData{
			Symbol: "BTC/USD", TradeID: 2, Timestamp: time.Unix(2, 0),
		})
		cvdNext := thesis.MarketTrades(SourceCVD)
		hawkes := thesis.MarketTrades(SourceHawkes)

		Convey("Then each signal should advance only its own cache cursor", func() {
			So(cvd, ShouldHaveLength, 1)
			So(cvdNext, ShouldHaveLength, 2)
			So(cvdNext[1].TradeID, ShouldEqual, 2)
			So(hawkes, ShouldHaveLength, 2)
		})
	})
}

func TestThesisAppendEquity(t *testing.T) {
	Convey("Given a complete positive account valuation", t, func() {
		thesis := NewThesis(t.Context(), nil)
		logic := make(chan struct{}, 1)
		regulator := make(chan struct{}, 1)
		equity := kraken.TradeBalanceResult{
			Equity:        decimal.NewFromInt64(200),
			UnrealizedPnL: decimal.NewFromInt64(-3),
		}

		err := thesis.AppendEquity(equity)
		stored, exists := thesis.Equity()

		Convey("Then it should retain the valuation and wake only the regulator", func() {
			So(err, ShouldBeNil)
			So(exists, ShouldBeTrue)
			So(stored.Equity.Float64(), ShouldEqual, 200.0)
			So(stored.UnrealizedPnL.Float64(), ShouldEqual, -3.0)
			So(len(logic), ShouldEqual, 0)
			So(len(regulator), ShouldEqual, 1)
		})
	})

	Convey("Given an incomplete account valuation", t, func() {
		thesis := NewThesis(t.Context(), nil)

		Convey("Then it should reject the update instead of fanning out zero equity", func() {
			So(thesis.AppendEquity(kraken.TradeBalanceResult{}), ShouldNotBeNil)
			_, exists := thesis.Equity()
			So(exists, ShouldBeFalse)
		})
	})
}

func TestThesisReset(t *testing.T) {
	Convey("Given a Thesis with prior measurement and evaluation state", t, func() {
		thesis := NewThesis(t.Context(), nil)
		symbol := NewSymbol("BTC/USD", nil)
		thesis.Symbols.Store("BTC/USD", symbol)
		measurements := []*Measurement{{
			Source: SourceToxicity,
			Symbol: "BTC/USD",
			At:     time.Unix(1, 0),
		}}
		consumed := kraken.TradeData{
			Symbol: "BTC/USD", TradeID: 1, Timestamp: time.Unix(1, 0),
		}
		thesis.AppendTrade(consumed)
		So(thesis.MarketTrades(SourceToxicity), ShouldHaveLength, 1)
		thesis.AppendMeasurements(SourceToxicity, measurements, false)
		symbol.AddMeasurement(measurements[0])
		symbol.Categories.Store("BTC/USD", []Category{{Symbol: "BTC/USD"}})
		thesis.Reset()

		Convey("Then the next epoch should retain only the prior measurements", func() {
			_, found := thesis.Measurements.Load(SourceToxicity)
			So(found, ShouldBeTrue)
			_, found = symbol.Categories.Load("BTC/USD")
			So(found, ShouldBeFalse)

			thesis.AppendTrade(consumed)
			thesis.AppendTrade(kraken.TradeData{
				Symbol: "BTC/USD", TradeID: 2, Timestamp: time.Unix(2, 0),
			})
			unseen := thesis.MarketTrades(SourceToxicity)
			So(unseen, ShouldHaveLength, 2)
			So(unseen[1].TradeID, ShouldEqual, 2)
		})
	})

	Convey("Given one planned symbol and one still-active symbol", t, func() {
		thesis := NewThesis(t.Context(), nil)
		bitcoin := NewSymbol("BTC/USD", nil)
		ether := NewSymbol("ETH/USD", nil)
		thesis.Symbols.Store("BTC/USD", bitcoin)
		thesis.Symbols.Store("ETH/USD", ether)
		bitcoin.Categories.Store("BTC/USD", []Category{{Symbol: "BTC/USD"}})
		ether.Categories.Store("ETH/USD", []Category{{Symbol: "ETH/USD"}})
		bitcoin.Graphs.Store("market_graph", struct{}{})

		thesis.Reset("BTC/USD")

		Convey("Then it should clear only that symbol's evaluation state", func() {
			_, bitcoinCategory := bitcoin.Categories.Load("BTC/USD")
			_, etherCategory := ether.Categories.Load("ETH/USD")
			_, graph := bitcoin.Graphs.Load("market_graph")
			So(bitcoinCategory, ShouldBeFalse)
			So(etherCategory, ShouldBeTrue)
			So(graph, ShouldBeFalse)
		})
	})
}

func BenchmarkThesisAppendMeasurements(b *testing.B) {
	b.Run("empty pass", func(b *testing.B) {
		thesis := NewThesis(b.Context(), nil)

		b.ReportAllocs()
		b.ResetTimer()

		for b.Loop() {
			if err := thesis.AppendMeasurements(SourceCorrelation, nil, false); err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("replacement", func(b *testing.B) {
		thesis := NewThesis(b.Context(), nil)
		epochs := []*Measurement{
			{ID: "correlation-1", Source: SourceCorrelation, Symbol: "BTC/USD", At: time.Unix(1, 0)},
			{ID: "correlation-2", Source: SourceCorrelation, Symbol: "BTC/USD", At: time.Unix(2, 0)},
		}
		epoch := 0
		incoming := make([]*Measurement, 1)
		thesis.Measurements.Store(SourceCorrelation, []*Measurement{epochs[epoch]})

		b.ReportAllocs()
		b.ResetTimer()

		for b.Loop() {
			epoch = 1 - epoch
			incoming[0] = epochs[epoch]

			if err := thesis.AppendMeasurements(
				SourceCorrelation, incoming, true,
			); err != nil {
				b.Fatal(err)
			}
		}
	})
}
func BenchmarkThesisAppendTicker(b *testing.B) {
	ticker := kraken.TickerData{
		Symbol: "BTC/USD", Timestamp: time.Unix(1, 0),
	}
	b.ReportAllocs()

	for b.Loop() {
		b.StopTimer()
		thesis := NewThesis(b.Context(), nil)
		b.StartTimer()
		thesis.AppendTicker(ticker)
	}
}

func BenchmarkThesisAppendTrade(b *testing.B) {
	trade := kraken.TradeData{
		Symbol: "BTC/USD", TradeID: 1, Timestamp: time.Unix(1, 0),
	}
	b.ReportAllocs()

	for b.Loop() {
		b.StopTimer()
		thesis := NewThesis(b.Context(), nil)
		b.StartTimer()
		thesis.AppendTrade(trade)
	}
}
