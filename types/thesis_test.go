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
		peerSignal := make(chan struct{}, 1)
		thesis.Subscribe(SourceAnalyzer, analyzer)
		thesis.Subscribe(SourceCVD, peerSignal)
		measurement := &Measurement{
			ID: "correlation", Source: SourceCorrelation, Symbol: "BTC/USD",
		}

		err := thesis.AppendMeasurements(
			SourceCorrelation, []*Measurement{measurement}, true,
		)

		Convey("Then it should create the ready symbol and queue its first batch", func() {
			So(err, ShouldBeNil)
			So(len(analyzer), ShouldEqual, 0)
			So(len(peerSignal), ShouldEqual, 0)
			storedSymbol, found := thesis.Symbols.Load("BTC/USD")
			So(found, ShouldBeTrue)
			symbol := storedSymbol.(*Symbol)
			So(symbol.Status, ShouldEqual, READY)
			So(symbol.Measurements, ShouldBeEmpty)
			So(symbol.Stamped(SourceCorrelation), ShouldBeFalse)
			stored, found := thesis.Measurements.Load(SourceCorrelation)
			So(found, ShouldBeTrue)
			So(stored, ShouldResemble, []*Measurement{measurement})
		})
	})

	Convey("Given an empty measurement pass", t, func() {
		thesis := NewThesis(t.Context(), nil)
		correlation := make(chan struct{}, 1)
		cvd := make(chan struct{}, 1)
		thesis.Subscribe(SourceCorrelation, correlation)
		thesis.Subscribe(SourceCVD, cvd)

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
		thesis.Subscribe(SourceCorrelation, correlation)
		thesis.Subscribe(SourceCVD, cvd)
		thesis.Subscribe(SourceCategory, categories)

		thesis.AppendMeasurements(SourceCorrelation, nil, true)

		Convey("Then it should keep readiness and downstream work pending", func() {
			So(thesis.Stamped("BTC/USD", SourceCorrelation), ShouldBeFalse)
			So(len(correlation), ShouldEqual, 0)
			So(len(cvd), ShouldEqual, 0)
			So(len(categories), ShouldEqual, 0)
		})
	})

	Convey("Given multiple measurements from one signal", t, func() {
		thesis := NewThesis(t.Context(), nil)
		measurements := []*Measurement{
			{ID: "btc", Source: SourceLeadLag, Symbol: "BTC/USD", At: time.Unix(1, 0)},
			{ID: "eth", Source: SourceLeadLag, Symbol: "ETH/USD", At: time.Unix(2, 0)},
			{ID: "sol", Source: SourceLeadLag, Symbol: "SOL/USD", At: time.Unix(3, 0)},
		}

		thesis.AppendMeasurements(SourceLeadLag, measurements, true)

		Convey("Then it should create every ready symbol and retain the first batch", func() {
			stored, found := thesis.Measurements.Load(SourceLeadLag)
			So(found, ShouldBeTrue)
			So(stored, ShouldResemble, measurements)

			for _, measurement := range measurements {
				storedSymbol, found := thesis.Symbols.Load(measurement.Symbol)
				So(found, ShouldBeTrue)
				So(storedSymbol.(*Symbol).Status, ShouldEqual, READY)
				So(storedSymbol.(*Symbol).Measurements, ShouldBeEmpty)
			}
		})
	})

	Convey("Given one symbol measured by every ready signal", t, func() {
		thesis := NewThesis(t.Context(), nil)
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

		for _, source := range sources {
			err := thesis.AppendMeasurements(source, []*Measurement{{
				ID: string(source), Source: source, Symbol: "BTC/USD", At: time.Unix(1, 0),
			}}, true)
			So(err, ShouldBeNil)
		}

		Convey("Then each sender's first batch should remain queued", func() {
			pending := 0
			thesis.Measurements.Range(func(_, value any) bool {
				pending += len(value.([]*Measurement))
				return true
			})
			So(pending, ShouldEqual, len(sources))
			So(thesis.Stamped("BTC/USD", SourceCorrelation), ShouldBeFalse)
		})
	})

	Convey("Given new measurements while a symbol is busy", t, func() {
		thesis := NewThesis(t.Context(), nil)
		analyzer := make(chan struct{}, 1)
		thesis.Subscribe(SourceAnalyzer, analyzer)
		symbol := NewSymbol("BTC/USD", nil)
		symbol.Reset()
		thesis.Symbols.Store("BTC/USD", symbol)
		first := &Measurement{
			ID: "correlation", Source: SourceCorrelation, Symbol: "BTC/USD",
		}
		pending := &Measurement{
			ID: "correlation-pending", Source: SourceCorrelation, Symbol: "BTC/USD",
		}
		So(thesis.AppendMeasurements(
			SourceCorrelation, []*Measurement{first}, true,
		), ShouldBeNil)
		So(thesis.AppendMeasurements(
			SourceCorrelation, []*Measurement{pending}, true,
		), ShouldBeNil)

		Convey("Then it should cut prior work and retain the new batch", func() {
			So(symbol.Measurements, ShouldResemble, []*Measurement{first})
			queued, found := thesis.Measurements.Load(SourceCorrelation)
			So(found, ShouldBeTrue)
			So(queued, ShouldResemble, []*Measurement{pending})
			So(len(analyzer), ShouldEqual, 1)
		})
	})

	Convey("Given a new measurement for one previously observed symbol", t, func() {
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

		Convey("Then it should retain only work that arrived while the symbol was busy", func() {
			stored, found := thesis.Measurements.Load(SourceHawkes)
			So(found, ShouldBeTrue)
			So(stored, ShouldResemble, []*Measurement{updated})
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

	Convey("Given a measurement epoch whose market input is already committed", t, func() {
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

		Convey("Then replaying that measurement should queue it behind the active cut", func() {
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
}

func TestThesisFanout(t *testing.T) {
	Convey("Given repeated work for one analyzer", t, func() {
		thesis := NewThesis(t.Context(), nil)
		analyzer := make(chan struct{}, 1)
		status := &atomic.Value{}
		status.Store(READY)
		thesis.Subscribe(SourceAnalyzer, analyzer, status)

		thesis.Fanout(SourceCorrelation, SourceAnalyzer)
		thesis.Fanout(SourceCVD, SourceAnalyzer)

		Convey("Then it should retain one coalesced wake-up", func() {
			So(len(analyzer), ShouldEqual, 1)
			So(status.Load(), ShouldEqual, BUSY)
		})

		Convey("Then it should accept the next wake-up after becoming ready", func() {
			<-analyzer
			status.Store(READY)
			thesis.Fanout(SourceCVD, SourceAnalyzer)

			So(len(analyzer), ShouldEqual, 1)
			So(status.Load(), ShouldEqual, BUSY)
		})
	})

	Convey("Given signal and downstream subscribers", t, func() {
		thesis := NewThesis(t.Context(), nil)
		correlation := make(chan struct{}, 1)
		cvd := make(chan struct{}, 1)
		categories := make(chan struct{}, 1)
		thesis.Subscribe(SourceCorrelation, correlation)
		thesis.Subscribe(SourceCVD, cvd)
		thesis.Subscribe(SourceCategory, categories)

		thesis.Fanout(SourceCorrelation)

		Convey("Then signal output should wake every other subscriber", func() {
			So(len(correlation), ShouldEqual, 0)
			So(len(cvd), ShouldEqual, 1)
			So(len(categories), ShouldEqual, 1)
		})

		Convey("Then market input should wake every signal", func() {
			thesis.Fanout(SourceTrader)

			So(len(correlation), ShouldEqual, 1)
			So(len(cvd), ShouldEqual, 1)
		})
	})

	Convey("Given explicitly targeted subscribers", t, func() {
		thesis := NewThesis(t.Context(), nil)
		correlation := make(chan struct{}, 1)
		categories := make(chan struct{}, 1)
		regulator := make(chan struct{}, 1)
		thesis.Subscribe(SourceCorrelation, correlation)
		thesis.Subscribe(SourceCategory, categories)
		thesis.Subscribe(SourceRegulator, regulator)

		thesis.Fanout(SourceEquity, SourceCorrelation, SourceRegulator)

		Convey("Then only the named receiver group should wake", func() {
			So(len(correlation), ShouldEqual, 1)
			So(len(categories), ShouldEqual, 0)
			So(len(regulator), ShouldEqual, 1)
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
		thesis.Subscribe(SourceCategory, logic)
		thesis.Subscribe(SourceRegulator, regulator)
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
	Convey("Given a completed Thesis with ready measurement evidence", t, func() {
		thesis := NewThesis(t.Context(), nil)
		symbol := NewSymbol("BTC/USD", nil)
		thesis.Symbols.Store("BTC/USD", symbol)
		notified := make(chan struct{}, 1)
		thesis.Subscribe(SourceAnalyzer, notified)
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
		thesis.AppendMeasurements(SourceToxicity, measurements, true)
		So(len(notified), ShouldEqual, 0)
		symbol.Categories.Store("BTC/USD", []Category{{Symbol: "BTC/USD"}})

		for _, source := range []SourceType{
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
			SourceCategory,
			SourceCognition,
			SourceManifold,
			SourceResonance,
			SourceCausal,
			SourceGraph,
			SourcePlanner,
		} {
			thesis.Stamp("BTC/USD", source)
		}

		So(thesis.Stamped("BTC/USD"), ShouldBeTrue)
		thesis.Reset()

		Convey("Then the next epoch should retain only the prior measurements", func() {
			_, found := thesis.Measurements.Load(SourceToxicity)
			So(found, ShouldBeTrue)
			So(thesis.Stamped("BTC/USD"), ShouldBeFalse)
			So(len(notified), ShouldEqual, 0)
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
		thesis.Stamp("BTC/USD", SourcePlanner)
		thesis.Stamp("ETH/USD", SourcePlanner)
		bitcoin.Categories.Store("BTC/USD", []Category{{Symbol: "BTC/USD"}})
		ether.Categories.Store("ETH/USD", []Category{{Symbol: "ETH/USD"}})
		bitcoin.Graphs.Store("market_graph", struct{}{})

		thesis.Reset("BTC/USD")

		Convey("Then it should clear only that symbol's evaluation state", func() {
			So(thesis.Stamped("BTC/USD", SourcePlanner), ShouldBeFalse)
			So(thesis.Stamped("ETH/USD", SourcePlanner), ShouldBeTrue)
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

func BenchmarkThesisFanout(b *testing.B) {
	thesis := NewThesis(b.Context(), nil)
	thesis.Symbols.Store("BTC/USD", NewSymbol("BTC/USD", nil))
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
		SourceCategory,
		SourceCognition,
		SourceManifold,
		SourceResonance,
		SourceCausal,
		SourceGraph,
		SourcePlanner,
	}

	for b.Loop() {
		for _, source := range sources {
			thesis.Stamp("BTC/USD", source)
		}

		thesis.Fanout(SourcePlanner)
		thesis.Reset()
	}
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
