package types

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
	pmanifold "github.com/theapemachine/symm/nomagique/physics/sensorium"
	"github.com/theapemachine/symm/nomagique/transport"
	nmtypes "github.com/theapemachine/symm/nomagique/types"
	wire "github.com/theapemachine/symm/telemetry/generated/telemetry"
)

func TestThesisAdvanceTick(t *testing.T) {
	Convey("Given successive real market observation times", t, func() {
		dashboard := transport.NewConsumer[*UIFrame]("dashboard", func() {})
		ui := transport.NewMapReduce[*UIFrame](
			[]*transport.Consumer[*UIFrame]{dashboard}, nil, nil,
		)
		thesis := NewThesis(t.Context(), ui)
		firstAt := time.Unix(1, 0)
		secondAt := time.Unix(2, 0)

		firstTick := thesis.AdvanceTick(firstAt)
		secondTick := thesis.AdvanceTick(secondAt)
		firstFrame, firstFound := ui.Pop(dashboard)
		secondFrame, secondFound := ui.Pop(dashboard)

		Convey("It should advance exactly once per observation and publish each transition", func() {
			So(firstTick, ShouldEqual, int64(1))
			So(secondTick, ShouldEqual, int64(2))
			So(thesis.Tick, ShouldEqual, int64(2))
			So(thesis.At, ShouldResemble, secondAt)
			So(firstFound, ShouldBeTrue)
			So(secondFound, ShouldBeTrue)
			So(firstFrame.Type, ShouldEqual, wire.FrameTickFrame)
			So(firstFrame.Value.(*wire.TickFrameT).Count, ShouldEqual, int64(1))
			So(secondFrame.Value.(*wire.TickFrameT).Count, ShouldEqual, int64(2))
		})
	})
}

func TestThesisWork(t *testing.T) {
	Convey("Given a stage waiting on its MapReduce work queue", t, func() {
		thesis := NewThesis(t.Context(), nil)
		ready := make(chan struct{}, 1)
		consumer := transport.NewConsumer[*Symbol]("liquidity", func() {
			ready <- struct{}{}
		})
		thesis.Work(SourceLiquidity).Register(consumer)
		symbol := thesis.Symbol("BTC/USD")
		symbol.AppendTicker(kraken.TickerData{})
		<-ready
		var scheduled *Symbol

		for candidate := range thesis.Work(SourceLiquidity).Drain(consumer, nil) {
			scheduled = candidate
		}

		Convey("It should deliver the owning symbol directly to that reader", func() {
			So(scheduled, ShouldEqual, symbol)
		})
	})

	Convey("Given a fresh measurement for the manifold consumer", t, func() {
		thesis := NewThesis(t.Context(), nil)
		ready := make(chan struct{}, 1)
		consumer := transport.NewConsumer[*Symbol]("manifold", func() {
			ready <- struct{}{}
		})
		thesis.Work(SourceManifold).Register(consumer)
		symbol := thesis.Symbol("BTC/USD")
		symbol.AppendMeasurement(nmtypes.NewMeasurement(
			"hawkes", string(SourceHawkes), time.Now().UnixNano(), 0,
		))
		<-ready
		var scheduled *Symbol

		for candidate := range thesis.Work(SourceManifold).Drain(consumer, nil) {
			scheduled = candidate
		}

		Convey("It should wake the manifold worker with the owning symbol", func() {
			So(scheduled, ShouldEqual, symbol)
		})
	})

}

func TestThesisHoldWork(t *testing.T) {
	Convey("Given a derived stage held across an external observation cut", t, func() {
		thesis := NewThesis(t.Context(), nil)
		ready := make(chan struct{}, 1)
		consumer := transport.NewConsumer[*Symbol]("category", func() {
			ready <- struct{}{}
		})
		thesis.Work(SourceCategory).Register(consumer)
		thesis.HoldWork(SourceCategory)
		symbol := thesis.Symbol("BTC/USD")

		for observed := int64(1); observed <= 3; observed++ {
			symbol.AppendMeasurement(nmtypes.NewMeasurement(
				"measurement", string(SourceHawkes), observed, 0,
			))
		}

		Convey("It should retain the complete input cut without waking the stage", func() {
			So(thesis.Work(SourceCategory).Length(consumer), ShouldEqual, uint64(0))
			So(symbol.Measurements.Length(
				symbol.MeasurementConsumers[MeasurementConsumerCategory],
			), ShouldEqual, uint64(3))

			select {
			case <-ready:
				t.Fatal("held category stage was scheduled")
			default:
			}
		})

		Convey("It should schedule the owning symbol exactly once when released", func() {
			thesis.ReleaseWork(SourceCategory)
			<-ready
			var scheduled []*Symbol

			for candidate := range thesis.Work(SourceCategory).Drain(consumer, nil) {
				scheduled = append(scheduled, candidate)
			}

			So(scheduled, ShouldResemble, []*Symbol{symbol})

			thesis.HoldWork(SourceCategory)
			thesis.ReleaseWork(SourceCategory)

			select {
			case <-ready:
				t.Fatal("category stage reused a cleared deferred wake")
			default:
			}
		})
	})

	Convey("Given planner clock wakes while planner work is held", t, func() {
		thesis := NewThesis(t.Context(), nil)
		ready := make(chan struct{}, 1)
		consumer := transport.NewConsumer[*Symbol]("planner", func() {
			ready <- struct{}{}
		})
		thesis.Work(SourcePlanner).Register(consumer)
		thesis.HoldWork(SourcePlanner)
		thesis.ScheduleWork(SourcePlanner, nil)
		thesis.ScheduleWork(SourcePlanner, nil)

		Convey("It should coalesce them into one clock pass on release", func() {
			thesis.ReleaseWork(SourcePlanner)
			<-ready
			var scheduled []*Symbol

			for candidate := range thesis.Work(SourcePlanner).Drain(consumer, nil) {
				scheduled = append(scheduled, candidate)
			}

			So(scheduled, ShouldResemble, []*Symbol{nil})
		})
	})

	Convey("Given graph-backed planner work and a deferred clock wake", t, func() {
		thesis := NewThesis(t.Context(), nil)
		ready := make(chan struct{}, 1)
		consumer := transport.NewConsumer[*Symbol]("planner", func() {
			ready <- struct{}{}
		})
		thesis.Work(SourcePlanner).Register(consumer)
		thesis.HoldWork(SourcePlanner)
		symbol := thesis.Symbol("BTC/USD")
		symbol.Graphs.Push(NewGraph(time.Unix(1, 0)))
		thesis.ScheduleWork(SourcePlanner, nil)

		Convey("It should schedule one cross-sectional pass without an extra clock pass", func() {
			thesis.ReleaseWork(SourcePlanner)
			<-ready
			var scheduled []*Symbol

			for candidate := range thesis.Work(SourcePlanner).Drain(consumer, nil) {
				scheduled = append(scheduled, candidate)
			}

			So(scheduled, ShouldResemble, []*Symbol{nil})
			So(symbol.HasPendingWork(SourcePlanner), ShouldBeTrue)
		})
	})

	Convey("Given manifold measurements retained for multiple symbols", t, func() {
		thesis := NewThesis(t.Context(), nil)
		ready := make(chan struct{}, 1)
		consumer := transport.NewConsumer[*Symbol]("manifold", func() {
			ready <- struct{}{}
		})
		thesis.Work(SourceManifold).Register(consumer)
		thesis.HoldWork(SourceManifold)
		bitcoin := thesis.Symbol("BTC/USD")
		ether := thesis.Symbol("ETH/USD")
		bitcoin.AppendMeasurement(nmtypes.NewMeasurement(
			"bitcoin", string(SourceHawkes), 1, 0,
		))
		ether.AppendMeasurement(nmtypes.NewMeasurement(
			"ether", string(SourceHawkes), 2, 0,
		))

		Convey("It should expose the complete universe cut before one wake", func() {
			thesis.ReleaseWork(SourceManifold)
			<-ready
			var scheduled []*Symbol

			for candidate := range thesis.Work(SourceManifold).Drain(consumer, nil) {
				scheduled = append(scheduled, candidate)
			}

			So(scheduled, ShouldResemble, []*Symbol{nil})
			So(bitcoin.HasPendingWork(SourceManifold), ShouldBeTrue)
			So(ether.HasPendingWork(SourceManifold), ShouldBeTrue)
		})
	})

	Convey("Given per-symbol derived work registered in creation order", t, func() {
		thesis := NewThesis(t.Context(), nil)
		consumer := transport.NewConsumer[*Symbol]("category", func() {})
		thesis.Work(SourceCategory).Register(consumer)
		thesis.HoldWork(SourceCategory)
		bitcoin := thesis.Symbol("BTC/USD")
		ether := thesis.Symbol("ETH/USD")
		solana := thesis.Symbol("SOL/USD")

		for index, symbol := range []*Symbol{bitcoin, ether, solana} {
			symbol.AppendMeasurement(nmtypes.NewMeasurement(
				symbol.Symbol, string(SourceHawkes), int64(index+1), 0,
			))
		}

		Convey("It should release the retained symbols in stable order", func() {
			thesis.ReleaseWork(SourceCategory)
			var scheduled []*Symbol

			for symbol := range thesis.Work(SourceCategory).Drain(consumer, nil) {
				scheduled = append(scheduled, symbol)
			}

			So(scheduled, ShouldResemble, []*Symbol{bitcoin, ether, solana})
		})
	})

	Convey("Given every raw analytical source held across one replay cut", t, func() {
		thesis := NewThesis(t.Context(), nil)
		sources := append([]SourceType(nil), SignalSources...)
		sources = append(sources, SourceResonance)
		consumers := make(map[SourceType]*transport.Consumer[*Symbol], len(sources))

		for _, source := range sources {
			consumer := transport.NewConsumer[*Symbol](string(source), func() {})
			thesis.Work(source).Register(consumer)
			consumers[source] = consumer
		}

		thesis.HoldWork(sources...)
		symbol := thesis.Symbol("BTC/USD")
		symbol.AppendTicker(kraken.TickerData{})
		symbol.AppendTrade(kraken.TradeData{})
		symbol.AppendLevel3(kraken.Level3Data{})
		symbol.AppendFuturesTicker(kraken.FuturesTickerData{})

		Convey("It should retain every source cursor and release one wake per source", func() {
			for _, source := range sources {
				consumer := consumers[source]
				So(thesis.Work(source).Length(consumer), ShouldEqual, uint64(0))
				So(symbol.HasPendingWork(source), ShouldBeTrue)

				thesis.ReleaseWork(source)
				var scheduled []*Symbol

				for candidate := range thesis.Work(source).Drain(consumer, nil) {
					scheduled = append(scheduled, candidate)
				}

				So(scheduled, ShouldResemble, []*Symbol{symbol})
			}
		})
	})
}

func TestThesisWaitForQuiescence(t *testing.T) {
	Convey("Given work that schedules another worker before it completes", t, func() {
		thesis := NewThesis(t.Context(), nil)
		upstreamStarted := make(chan struct{})
		releaseUpstream := make(chan struct{})
		downstreamStarted := make(chan struct{})
		releaseDownstream := make(chan struct{})
		var upstream *transport.Consumer[*Symbol]
		var downstream *transport.Consumer[*Symbol]

		upstream = transport.NewConsumer[*Symbol]("liquidity", func() {
			go func() {
				for symbol := range thesis.Work(SourceLiquidity).Drain(upstream, nil) {
					close(upstreamStarted)
					<-releaseUpstream
					thesis.ScheduleWork(SourceCategory, symbol)
				}
			}()
		})
		downstream = transport.NewConsumer[*Symbol]("category", func() {
			go func() {
				for range thesis.Work(SourceCategory).Drain(downstream, nil) {
					close(downstreamStarted)
					<-releaseDownstream
				}
			}()
		})
		thesis.Work(SourceLiquidity).Register(upstream)
		thesis.Work(SourceCategory).Register(downstream)
		thesis.ScheduleWork(SourceLiquidity, thesis.Symbol("BTC/USD"))
		<-upstreamStarted

		waitCtx, cancel := context.WithTimeout(t.Context(), time.Second)
		defer cancel()
		waited := make(chan error, 1)

		go func() {
			waited <- thesis.WaitForQuiescence(waitCtx)
		}()

		Convey("It should not report idle while dequeued upstream work is active", func() {
			select {
			case err := <-waited:
				t.Fatalf("quiescence returned during upstream work: %v", err)
			default:
			}

			close(releaseUpstream)
			<-downstreamStarted

			select {
			case err := <-waited:
				t.Fatalf("quiescence returned during downstream work: %v", err)
			default:
			}

			close(releaseDownstream)
			So(<-waited, ShouldBeNil)
		})
	})
}

func TestThesisMarshalState(t *testing.T) {
	Convey("Given a thesis carrying symbol and equity state", t, func() {
		thesis := NewThesis(t.Context(), nil)
		thesis.Tick = 7
		So(thesis.AppendEquity(kraken.TradeBalanceResult{
			Equity: decimal.NewFromInt64(200),
		}), ShouldBeNil)
		symbol := thesis.Symbol("BTC/USD")
		decision := NewDecision(ActionEnter, "BTC/USD")
		symbol.Decisions.Push(*decision)
		symbol.Graphs.Push(NewGraph(time.Unix(1, 0).UTC()))
		symbol.Categories.Push([]Category{{Symbol: "BTC/USD"}})
		symbol.Phase.Push(PhaseReading{})
		symbol.Cognition.Push(Cognition{Symbol: "BTC/USD"})
		symbol.Causal.Push(map[string]any{"precision": 0.5})

		state, err := thesis.MarshalState()
		var checkpoint map[string]any
		unmarshalErr := json.Unmarshal(state, &checkpoint)

		Convey("It should materialize durable thesis state", func() {
			So(err, ShouldBeNil)
			So(unmarshalErr, ShouldBeNil)
			So(checkpoint["tick"], ShouldEqual, float64(7))
			So(checkpoint["status"], ShouldEqual, "ready")
			symbols := checkpoint["symbols"].(map[string]any)
			bitcoin := symbols["BTC/USD"].(map[string]any)
			So(bitcoin["causal"].([]any), ShouldHaveLength, 1)
			So(bitcoin["graphs"].([]any), ShouldHaveLength, 1)
			So(checkpoint["equity"], ShouldNotBeNil)
		})
	})
}

func TestThesisSymbol(t *testing.T) {
	Convey("Given an empty thesis", t, func() {
		thesis := NewThesis(t.Context(), nil)

		first := thesis.Symbol("BTC/USD")
		second := thesis.Symbol("BTC/USD")

		Convey("It should return the canonical symbol state", func() {
			So(first, ShouldEqual, second)
			So(first.Symbol, ShouldEqual, "BTC/USD")
		})
	})
}

func TestThesisForSymbol(t *testing.T) {
	Convey("Given a thesis containing independent symbol state", t, func() {
		thesis := NewThesis(t.Context(), nil)
		bitcoin := thesis.Symbol("BTC/USD")
		thesis.Symbol("ETH/USD")

		scoped, err := thesis.ForSymbol("BTC/USD")

		Convey("It should share only the selected symbol with incremental solvers", func() {
			So(err, ShouldBeNil)
			selected, found := scoped.Symbols.Load("BTC/USD")
			So(found, ShouldBeTrue)
			So(selected, ShouldEqual, bitcoin)
			_, found = scoped.Symbols.Load("ETH/USD")
			So(found, ShouldBeFalse)
			So(scoped.CrossSection, ShouldEqual, thesis.CrossSection)
			So(scoped.workRevision, ShouldEqual, thesis.workRevision)
		})

		Convey("It should reject a symbol absent from the parent thesis", func() {
			_, err = thesis.ForSymbol("MISSING/USD")
			So(err, ShouldNotBeNil)
		})
	})
}

func TestThesisForSymbols(t *testing.T) {
	Convey("Given a thesis containing independent symbol state", t, func() {
		thesis := NewThesis(t.Context(), nil)
		bitcoin := thesis.Symbol("BTC/USD")
		ether := thesis.Symbol("ETH/USD")
		thesis.Symbol("SOL/USD")

		scoped, err := thesis.ForSymbols([]string{"ETH/USD", "BTC/USD"})

		Convey("It should share exactly the requested symbols", func() {
			So(err, ShouldBeNil)
			selected, found := scoped.Symbols.Load("BTC/USD")
			So(found, ShouldBeTrue)
			So(selected, ShouldEqual, bitcoin)
			selected, found = scoped.Symbols.Load("ETH/USD")
			So(found, ShouldBeTrue)
			So(selected, ShouldEqual, ether)
			_, found = scoped.Symbols.Load("SOL/USD")
			So(found, ShouldBeFalse)
			So(scoped.workRevision, ShouldEqual, thesis.workRevision)
		})

		Convey("It should reject an empty scope and a missing name", func() {
			_, err = thesis.ForSymbols(nil)
			So(err, ShouldNotBeNil)
			_, err = thesis.ForSymbols([]string{"BTC/USD", "MISSING/USD"})
			So(err, ShouldNotBeNil)
		})
	})
}

func TestThesisStoreManifold(t *testing.T) {
	Convey("Given a fluid reading published to a thesis", t, func() {
		thesis := NewThesis(t.Context(), nil)
		reading := pmanifold.Reading{GuidanceSpeed: 0.75}

		thesis.StoreManifold(reading)
		stored, found := thesis.ManifoldSnapshot()

		Convey("It should expose one immutable atomic snapshot", func() {
			So(found, ShouldBeTrue)
			So(stored, ShouldResemble, reading)
		})

		Convey("It should preserve that snapshot in a symbol scope", func() {
			thesis.Symbol("BTC/USD")
			scoped, err := thesis.ForSymbol("BTC/USD")
			scopedReading, scopedFound := scoped.ManifoldSnapshot()

			So(err, ShouldBeNil)
			So(scopedFound, ShouldBeTrue)
			So(scopedReading, ShouldResemble, reading)
		})
	})
}

func TestThesisStoreInterventions(t *testing.T) {
	Convey("Given crystallization scores published to a thesis", t, func() {
		thesis := NewThesis(t.Context(), nil)
		scores := []InterventionScore{{
			Action: "enter_buy",
			Score:  0.4,
		}}

		thesis.StoreInterventions(scores)
		stored, found := thesis.InterventionSnapshot()

		Convey("It should expose one immutable atomic snapshot", func() {
			So(found, ShouldBeTrue)
			So(stored, ShouldResemble, scores)
		})
	})
}

func TestThesisStorePhase(t *testing.T) {
	Convey("Given a universe phase sweep published to a thesis", t, func() {
		thesis := NewThesis(t.Context(), nil)
		reading := PhaseReading{Ready: true, Reason: "retaining history"}

		thesis.StorePhase(reading)
		stored, found := thesis.PhaseSnapshot()

		Convey("It should expose one immutable atomic snapshot", func() {
			So(found, ShouldBeTrue)
			So(stored, ShouldResemble, reading)
		})

		Convey("It should preserve that snapshot in a symbol scope", func() {
			thesis.Symbol("BTC/USD")
			scoped, err := thesis.ForSymbol("BTC/USD")
			scopedReading, scopedFound := scoped.PhaseSnapshot()

			So(err, ShouldBeNil)
			So(scopedFound, ShouldBeTrue)
			So(scopedReading, ShouldResemble, reading)
		})
	})
}

func TestThesisNewThesis(t *testing.T) {
	Convey("Given a thesis without market evidence", t, func() {
		thesis := NewThesis(t.Context(), nil)

		Convey("Its event clock should remain unset", func() {
			So(thesis.At.IsZero(), ShouldBeTrue)
		})
	})
}

func TestThesisAppendEquity(t *testing.T) {
	Convey("Given positive account equity", t, func() {
		thesis := NewThesis(t.Context(), nil)

		err := thesis.AppendEquity(kraken.TradeBalanceResult{
			Equity: decimal.NewFromInt64(200),
		})
		equity, found := thesis.Equity()

		Convey("It should retain the latest account summary", func() {
			So(err, ShouldBeNil)
			So(found, ShouldBeTrue)
			So(equity.Equity.Float64(), ShouldEqual, 200.0)
		})
	})

	Convey("Given an incomplete account valuation", t, func() {
		thesis := NewThesis(t.Context(), nil)

		err := thesis.AppendEquity(kraken.TradeBalanceResult{})

		Convey("It should reject the observation without dereferencing missing equity", func() {
			So(err, ShouldNotBeNil)
		})
	})
}

func TestThesisEquitySnapshot(t *testing.T) {
	Convey("Given successive complete account valuations", t, func() {
		thesis := NewThesis(t.Context(), nil)
		So(thesis.AppendEquity(kraken.TradeBalanceResult{
			Equity: decimal.NewFromInt64(200),
		}), ShouldBeNil)
		_, firstRevision, firstFound := thesis.EquitySnapshot()
		So(thesis.AppendEquity(kraken.TradeBalanceResult{
			Equity: decimal.NewFromInt64(201),
		}), ShouldBeNil)
		equity, secondRevision, secondFound := thesis.EquitySnapshot()

		Convey("It should identify each broker observation exactly once", func() {
			So(firstFound, ShouldBeTrue)
			So(secondFound, ShouldBeTrue)
			So(secondRevision, ShouldEqual, firstRevision+1)
			So(equity.Equity.Float64(), ShouldEqual, 201.0)
		})
	})
}

func BenchmarkThesisStoreManifold(b *testing.B) {
	thesis := NewThesis(b.Context(), nil)
	reading := pmanifold.Reading{GuidanceSpeed: 0.75}

	for b.Loop() {
		thesis.StoreManifold(reading)
	}
}

func BenchmarkThesisManifoldSnapshot(b *testing.B) {
	thesis := NewThesis(b.Context(), nil)
	thesis.StoreManifold(pmanifold.Reading{GuidanceSpeed: 0.75})

	for b.Loop() {
		_, _ = thesis.ManifoldSnapshot()
	}
}

func BenchmarkThesisStorePhase(b *testing.B) {
	thesis := NewThesis(b.Context(), nil)
	reading := PhaseReading{Ready: true}

	for b.Loop() {
		thesis.StorePhase(reading)
	}
}

func BenchmarkThesisPhaseSnapshot(b *testing.B) {
	thesis := NewThesis(b.Context(), nil)
	thesis.StorePhase(PhaseReading{Ready: true})

	for b.Loop() {
		_, _ = thesis.PhaseSnapshot()
	}
}

func BenchmarkThesisAdvanceTick(b *testing.B) {
	dashboard := transport.NewConsumer[*UIFrame]("dashboard", func() {})
	ui := transport.NewMapReduce[*UIFrame](
		[]*transport.Consumer[*UIFrame]{dashboard}, nil, nil,
	)
	thesis := NewThesis(b.Context(), ui)
	observedAt := time.Unix(1, 0)

	for b.Loop() {
		thesis.AdvanceTick(observedAt)
		_, _ = ui.Pop(dashboard)
	}
}

func BenchmarkThesisScheduleWork(b *testing.B) {
	thesis := NewThesis(b.Context(), nil)
	var consumer *transport.Consumer[*Symbol]
	consumer = transport.NewConsumer[*Symbol]("liquidity", func() {
		for range thesis.Work(SourceLiquidity).Drain(consumer, nil) {
		}
	})
	thesis.Work(SourceLiquidity).Register(consumer)
	symbol := thesis.Symbol("BTC/USD")
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		thesis.ScheduleWork(SourceLiquidity, symbol)
	}
}

func BenchmarkThesisReleaseWork(b *testing.B) {
	thesis := NewThesis(b.Context(), nil)

	for index := 0; index < 620; index++ {
		thesis.Symbol(fmt.Sprintf("%d/USD", index))
	}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		thesis.HoldWork(SourceCategory)
		thesis.ReleaseWork(SourceCategory)
	}
}
