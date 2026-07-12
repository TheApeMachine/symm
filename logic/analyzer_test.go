package logic

import (
	"strconv"
	"testing"
	"time"

	"github.com/bytedance/sonic"
	"github.com/spf13/viper"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/logic/manifold"
	"github.com/theapemachine/symm/strategy"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/types"

	. "github.com/smartystreets/goconvey/convey"
)

type analyzerFixture struct {
	ready bool
}

func newAnalyzerFixture() *analyzerFixture {
	return &analyzerFixture{ready: true}
}

func (fixture *analyzerFixture) Ready(system.StageType) bool {
	return fixture.ready
}

func (fixture *analyzerFixture) Analyzer(uiHub chan []byte) *Analyzer {
	return NewAnalyzer(fixture, uiHub)
}

type testLevel3Book struct {
	bid     float64
	ask     float64
	invalid bool
}

func (book testLevel3Book) Apply(kraken.Level3Data, int, int) bool { return !book.invalid }

func (book testLevel3Book) InvalidReason(string) manifold.InvalidReason {
	if book.invalid {
		return manifold.ChecksumFailed
	}

	return manifold.Valid
}

func (book testLevel3Book) TopOfBook(string) (float64, float64, bool) {
	if book.bid <= 0 || book.ask <= 0 {
		return 0, 0, false
	}

	return book.bid, book.ask, true
}

func init() {
	viper.Set("market.l3_depth", 10)
	viper.Set("trading.edge.forward_return_horizon", 5*time.Minute)
	viper.Set("market.forecast.rls.initial_variance", 1.0)
	viper.Set("market.forecast.rls.forgetting_factor", 1.0)
	viper.Set("market.manifold.lifetime_capacity", 256)
}

func TestAnalyzerRejectsEmptySymbol(t *testing.T) {
	Convey("Given level3 rows without a symbol", t, func() {
		analyzer := newAnalyzerFixture().Analyzer(nil)
		book := testLevel3Book{bid: 99, ask: 101}

		Convey("When the analyzer ingests the row", func() {
			analyzer.IngestLevel3(kraken.Level3Data{}, 1, 8, book)
			thesis := analyzer.PendingThesis()

			Convey("Then no thesis is produced", func() {
				So(len(thesis.Symbols()), ShouldEqual, 0)
			})
		})
	})
}

func TestAnalyzerIngestLevel3(t *testing.T) {
	Convey("Given an analyzer and valid level3 snapshot row", t, func() {
		analyzer := newAnalyzerFixture().Analyzer(nil)
		book := testLevel3Book{bid: 99, ask: 101}
		row := kraken.Level3Data{
			Symbol:    "BTC/USD",
			Type:      "snapshot",
			Timestamp: time.Unix(1, 0),
			Bids: []kraken.Level3Order{{
				OrderID: "bid-1", LimitPrice: 99, OrderQty: 2,
				Timestamp: time.Unix(1, 0),
			}},
			Asks: []kraken.Level3Order{{
				OrderID: "ask-1", LimitPrice: 101, OrderQty: 3,
				Timestamp: time.Unix(1, 0),
			}},
		}

		analyzer.IngestLevel3(row, 1, 8, book)
		theses := map[string]*strategy.Thesis{"BTCUSD": analyzer.PendingThesis()}

		Convey("It should admit a field engine slot for the symbol", func() {
			So(theses, ShouldHaveLength, 1)
			_, ok := analyzer.engine.Slot("BTC/USD")
			So(ok, ShouldBeTrue)
		})
	})
}

func TestAnalyzerObserveLevel3(t *testing.T) {
	Convey("Given an analyzer and one valid L3 snapshot", t, func() {
		ui := make(chan []byte, 8)
		analyzer := newAnalyzerFixture().Analyzer(ui)
		defer analyzer.Close()
		book := testLevel3Book{bid: 99, ask: 101}
		row := kraken.Level3Data{
			Symbol: "BTC/USD", Type: "snapshot", Timestamp: time.Unix(1, 0),
			Bids: []kraken.Level3Order{{
				OrderID: "bid-1", LimitPrice: 99, OrderQty: 2, Timestamp: time.Unix(1, 0),
			}},
			Asks: []kraken.Level3Order{{
				OrderID: "ask-1", LimitPrice: 101, OrderQty: 3, Timestamp: time.Unix(1, 0),
			}},
		}

		Convey("When the row is observed without advancing", func() {
			result := analyzer.ObserveLevel3(row, 1, 8, book)
			_, stateProduced := analyzer.PendingThesis().Evidence("BTC/USD", "manifold")

			Convey("Then it schedules the symbol without publishing a computed state", func() {
				So(result.AdvanceReady, ShouldBeTrue)
				So(result.State.InvalidReason, ShouldEqual, manifold.Valid)
				So(stateProduced, ShouldBeFalse)
				So(len(ui), ShouldEqual, 0)
			})
		})
	})

	Convey("Given a checksum-diverged L3 row", t, func() {
		analyzer := newAnalyzerFixture().Analyzer(nil)
		defer analyzer.Close()
		book := testLevel3Book{bid: 99, ask: 101, invalid: true}
		row := kraken.Level3Data{Symbol: "BTC/USD", Type: "update"}

		Convey("When the row is observed", func() {
			result := analyzer.ObserveLevel3(row, 1, 8, book)

			Convey("Then the typed checksum failure is retained for recovery", func() {
				So(result.AdvanceReady, ShouldBeFalse)
				So(result.State.InvalidReason, ShouldEqual, manifold.ChecksumFailed)
			})
		})
	})
}

func TestAnalyzerAdvanceLevel3(t *testing.T) {
	Convey("Given an analyzer with a schedulable L3 population", t, func() {
		ui := make(chan []byte, 8)
		analyzer := newAnalyzerFixture().Analyzer(ui)
		defer analyzer.Close()
		book := testLevel3Book{bid: 99, ask: 101}
		row := kraken.Level3Data{
			Symbol: "BTC/USD", Type: "snapshot", Timestamp: time.Unix(1, 0),
			Bids: []kraken.Level3Order{{
				OrderID: "bid-1", LimitPrice: 99, OrderQty: 2, Timestamp: time.Unix(1, 0),
			}},
			Asks: []kraken.Level3Order{{
				OrderID: "ask-1", LimitPrice: 101, OrderQty: 3, Timestamp: time.Unix(1, 0),
			}},
		}
		So(analyzer.ObserveLevel3(row, 1, 8, book).AdvanceReady, ShouldBeTrue)

		Convey("When the scheduled symbol advances", func() {
			analyzer.AdvanceLevel3(row.Symbol)
			evidence, stateProduced := analyzer.PendingThesis().Evidence("BTC/USD", "manifold")
			state, decoded := manifold.StateFromEvidence(evidence)

			Convey("Then one typed state is produced from the pending observation", func() {
				So(stateProduced, ShouldBeTrue)
				So(decoded, ShouldBeTrue)
				So(state.At, ShouldEqual, row.Timestamp)
				So(len(ui), ShouldBeGreaterThan, 0)
			})
		})
	})
}

func BenchmarkAnalyzerIngestLevel3ColdSymbols(b *testing.B) {
	analyzer := newAnalyzerFixture().Analyzer(nil)
	book := testLevel3Book{}

	for index := 0; index < b.N; index++ {
		analyzer.IngestLevel3(kraken.Level3Data{
			Symbol:    "BTC/USD-" + strconv.Itoa(index),
			Type:      "snapshot",
			Timestamp: time.Unix(int64(index)+1, 0),
			Bids: []kraken.Level3Order{{
				OrderID: "bid", LimitPrice: 99, OrderQty: 1,
				Timestamp: time.Unix(int64(index)+1, 0),
			}},
			Asks: []kraken.Level3Order{{
				OrderID: "ask", LimitPrice: 101, OrderQty: 1,
				Timestamp: time.Unix(int64(index)+1, 0),
			}},
		}, 1, 8, book)
	}
}

func TestAnalyzerUsesManifoldPackage(t *testing.T) {
	Convey("Given the analyzer engine", t, func() {
		analyzer := newAnalyzerFixture().Analyzer(nil)

		Convey("It should construct a manifold.Engine", func() {
			So(analyzer.engine, ShouldHaveSameTypeAs, manifold.NewEngine())
		})
	})
}

func TestAnalyzerStatus(t *testing.T) {
	Convey("Given an analyzer whose synchronous dependencies are constructed", t, func() {
		analyzer := newAnalyzerFixture().Analyzer(nil)

		Convey("When its boot status is requested", func() {
			status := analyzer.Status()

			Convey("Then the analyzer reports that it is ready to accept evidence", func() {
				So(status, ShouldEqual, types.READY)
			})
		})
	})
}

func TestAnalyzerPendingThesis(t *testing.T) {
	Convey("Given an analyzer shared by asynchronous pipeline stages", t, func() {
		analyzer := newAnalyzerFixture().Analyzer(nil)
		first := analyzer.PendingThesis()
		first.AddEvidence("BTC/USD", "ticker", 1.0)

		Convey("When the trading loop reads the thesis again", func() {
			second := analyzer.PendingThesis()

			Convey("Then every stage still owns the same lifecycle", func() {
				So(second, ShouldEqual, first)
				value, ok := second.Evidence("BTC/USD", "ticker")
				So(ok, ShouldBeTrue)
				So(value, ShouldEqual, 1.0)
			})
		})
	})

	Convey("Given an analyzer before the runtime reaches its ready stage", t, func() {
		analyzer := NewAnalyzer(&analyzerFixture{}, nil)

		Convey("When an earlier stage asks for the in-process Thesis", func() {
			thesis := analyzer.PendingThesis()

			Convey("Then the lifecycle carrier is already available", func() {
				So(thesis, ShouldNotBeNil)
			})
		})
	})
}

func TestAnalyzerPublish(t *testing.T) {
	Convey("Given a typed manifold state and the UI channel", t, func() {
		ui := make(chan []byte, 1)
		analyzer := newAnalyzerFixture().Analyzer(ui)
		state := causalState(1, 100)

		Convey("When the analyzer publishes the state", func() {
			analyzer.publish(state)
			frame := <-ui
			decoded := struct {
				Manifold manifold.State `json:"manifold"`
			}{}
			err := sonic.Unmarshal(frame, &decoded)

			Convey("Then the domain object is sent unchanged under its route key", func() {
				So(err, ShouldBeNil)
				So(decoded.Manifold.Symbol, ShouldEqual, state.Symbol)
				So(decoded.Manifold.Epoch, ShouldEqual, state.Epoch)
				So(decoded.Manifold.MidPrice, ShouldEqual, state.MidPrice)
			})
		})
	})
}
