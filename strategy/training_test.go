package strategy

import (
	"context"
	"sync"
	"testing"

	"github.com/shopspring/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/nomagique/market"
	"github.com/theapemachine/nomagique/runtime"
	"github.com/theapemachine/nomagique/store"
	"github.com/theapemachine/symm/broker"
)

func trainingBook(t *testing.T, symbol string) *broker.Price {
	t.Helper()

	bidPx := decimal.NewFromInt(100)
	bidQty := decimal.NewFromInt(1_000)
	askPx := decimal.NewFromInt(101)
	askQty := decimal.NewFromInt(1_000)

	books := broker.NewBookManager(nil)
	books.Update(&market.Level3{
		Data: []*market.Level3Data{{
			Symbol: symbol,
			Bids:   []*market.Level3Order{{LimitPrice: &bidPx, OrderQty: &bidQty}},
			Asks:   []*market.Level3Order{{LimitPrice: &askPx, OrderQty: &askQty}},
		}},
	})

	return &broker.Price{Books: books}
}

func TestNewTraining(t *testing.T) {
	Convey("Setup", t, func() {
		ctx := context.Background()

		Convey("When NewTraining is called without a store client", func() {
			training := NewTraining(ctx, trainingBook(t, "BTC/USD"), &broker.Desk{}, nil)

			Convey("Training owns a fresh grammar catalog", func() {
				So(training, ShouldNotBeNil)
				So(training.catalog, ShouldNotBeNil)
				So(training.sequencers, ShouldNotBeNil)
			})
		})
	})
}

func TestTrainingStep(t *testing.T) {
	Convey("Setup", t, func() {
		ctx := context.Background()
		training := &Training{
			System:     runtime.NewSystem(ctx, "training"),
			sequencers: &sync.Map{},
		}

		Convey("When Step receives a nil envelope", func() {
			Convey("Step returns nil", func() {
				So(training.Step(nil), ShouldBeNil)
			})
		})

		Convey("When the envelope has no per-symbol metrics", func() {
			training := &Training{
				System:     runtime.NewSystem(ctx, "training"),
				price:      trainingBook(t, "TRN/USD"),
				desk:       &broker.Desk{},
				grid:       store.NewGrid(ctx),
				sequencers: &sync.Map{},
			}
			env := runtime.NewEnvelope(nil)

			out := training.Step(env)

			Convey("Step returns the same envelope without paths", func() {
				So(out, ShouldEqual, env)
				So(out.Paths["TRN/USD"], ShouldBeNil)
			})
		})
	})
}

func TestSequencer(t *testing.T) {
	Convey("Setup", t, func() {
		training := &Training{sequencers: &sync.Map{}}

		Convey("When the same symbol is requested twice", func() {
			btc := training.Sequencer("BTC/USD")

			Convey("Sequencer returns the same instance", func() {
				So(training.Sequencer("BTC/USD"), ShouldEqual, btc)
			})
		})

		Convey("When two symbols each step once", func() {
			btc := training.Sequencer("BTC/USD")
			eth := training.Sequencer("ETH/USD")
			_, _ = btc.Step("R01", 0.5)
			_, _ = eth.Step("R02", 0.5)

			Convey("Grammar state diverges per symbol", func() {
				So(btc.State().Root, ShouldNotResemble, eth.State().Root)
			})
		})

		Convey("When symbol is empty", func() {
			seq := training.Sequencer("")

			Convey("Sequencer is not stored in the map", func() {
				So(seq, ShouldNotBeNil)
				_, loaded := training.sequencers.Load("")
				So(loaded, ShouldBeFalse)
			})
		})
	})
}

func TestRestoreSequencer(t *testing.T) {
	Convey("Setup", t, func() {
		training := &Training{sequencers: &sync.Map{}}

		Convey("When restoreSequencer hydrates saved grammar", func() {
			live := training.Sequencer("SYM/USD")
			_, _ = live.Step("R01", 0.5)
			state := live.State()

			training.restoreSequencer("SYM/USD", state)

			restored := training.Sequencer("SYM/USD")
			next, mutated := restored.Step("R02", 0.4)

			Convey("Continued stepping appends to restored root", func() {
				So(mutated, ShouldBeTrue)
				So(next.String(), ShouldEqual, "R01/R02")
			})
		})
	})
}

func TestResetSequencer(t *testing.T) {
	Convey("Setup", t, func() {
		ctx := context.Background()
		training := NewTraining(ctx, trainingBook(t, "RST/USD"), &broker.Desk{}, nil)

		Convey("When ResetSequencer clears a warmed symbol", func() {
			seq := training.Sequencer("RST/USD")
			_, _ = seq.Step("R03", 0.7)
			So(len(seq.State().Root), ShouldEqual, 1)

			training.ResetSequencer("RST/USD")

			Convey("Live sentence resets", func() {
				So(seq.State().LastEmitted, ShouldEqual, "")
				So(len(seq.State().Root), ShouldEqual, 0)
			})
		})

		Convey("When the symbol was never warmed", func() {
			Convey("ResetSequencer is a no-op", func() {
				So(func() { training.ResetSequencer("MISSING/USD") }, ShouldNotPanic)
			})
		})
	})
}

func TestFlush(t *testing.T) {
	Convey("Setup", t, func() {
		ctx := context.Background()
		training := NewTraining(ctx, trainingBook(t, "FLU/USD"), &broker.Desk{}, nil)

		Convey("When sequencers are warmed", func() {
			seq := training.Sequencer("FLU/USD")
			_, mutated := seq.Step("R02", 0.6)

			Convey("Flush persists without error when client is nil", func() {
				So(mutated, ShouldBeTrue)
				So(training.Flush(), ShouldBeNil)
			})
		})
	})
}

func TestStepCut(t *testing.T) {
	Convey("Setup", t, func() {
		training := &Training{
			System:     runtime.NewSystem(context.Background(), "training"),
			sequencers: &sync.Map{},
		}

		Convey("When the brightest action region is R03", func() {
			regions := []*runtime.Region{
				{Key: 3, Brightness: 0.8},
				{Key: 5, Brightness: 0.2},
				{Key: 17, Brightness: 0.9},
			}

			first := training.stepCut("BTC/USD", regions)
			second := training.stepCut("BTC/USD", regions)

			Convey("stepCut emits R03 without growing on repeat", func() {
				So(first.String(), ShouldEqual, "R03")
				So(first.Len(), ShouldEqual, 1)
				So(second.String(), ShouldEqual, "R03")
				So(second.Len(), ShouldEqual, 1)
			})
		})

		Convey("When only veto regions are lit", func() {
			cut := training.stepCut("ETH/USD", []*runtime.Region{
				{Key: 17, Brightness: 1},
			})

			Convey("stepCut returns an empty cut", func() {
				So(cut, ShouldNotBeNil)
				So(cut.Len(), ShouldEqual, 0)
			})
		})
	})
}
