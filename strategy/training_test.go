package strategy

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/nomagique/equation"
	"github.com/theapemachine/nomagique/market"
	"github.com/theapemachine/nomagique/runtime"
	"github.com/theapemachine/nomagique/store"
	"github.com/theapemachine/symm/broker"
)

func TestTrainingStepCut(t *testing.T) {
	Convey("stepCut steps the brightest action region", t, func() {
		training := &Training{
			System:     runtime.NewSystem(context.Background(), "training"),
			sequencers: &sync.Map{},
		}

		regions := []*runtime.Region{
			{Key: 3, Brightness: 0.8},
			{Key: 5, Brightness: 0.2},
			{Key: 17, Brightness: 0.9}, // veto region; ignored (>8)
		}

		first := training.stepCut("BTC/USD", regions)
		second := training.stepCut("BTC/USD", regions)
		So(first.String(), ShouldEqual, "R03")
		So(first.Len(), ShouldEqual, 1)
		So(second.String(), ShouldEqual, "R03")
		So(second.Len(), ShouldEqual, 1)
	})

	Convey("stepCut returns an empty cut when no action regions lit", t, func() {
		training := &Training{sequencers: &sync.Map{}}

		cut := training.stepCut("ETH/USD", []*runtime.Region{
			{Key: 17, Brightness: 1},
		})

		So(cut, ShouldNotBeNil)
		So(cut.Len(), ShouldEqual, 0)
	})
}

func TestTrainingSequencer(t *testing.T) {
	Convey("Sequencer allocates one engine per symbol", t, func() {
		training := &Training{sequencers: &sync.Map{}}

		btc := training.Sequencer("BTC/USD")
		So(training.Sequencer("BTC/USD"), ShouldEqual, btc)

		eth := training.Sequencer("ETH/USD")
		_, _ = btc.Step("R01", 0.5)
		_, _ = eth.Step("R02", 0.5)
		So(btc.State().Root, ShouldNotResemble, eth.State().Root)
	})
}

func TestTrainingRestoreSequencer(t *testing.T) {
	Convey("restoreSequencer hydrates saved grammar", t, func() {
		training := &Training{sequencers: &sync.Map{}}

		live := training.Sequencer("SYM/USD")
		_, _ = live.Step("R01", 0.5)
		state := live.State()

		training.restoreSequencer("SYM/USD", state)

		restored := training.Sequencer("SYM/USD")
		next, mutated := restored.Step("R02", 0.4)
		So(mutated, ShouldBeTrue)
		So(next.String(), ShouldContainSubstring, "R01")
	})
}

func TestTrainingStepNilEnvelope(t *testing.T) {
	Convey("Step returns nil for a nil envelope", t, func() {
		training := &Training{
			System:     runtime.NewSystem(context.Background(), "training"),
			sequencers: &sync.Map{},
		}

		So(training.Step(nil), ShouldBeNil)
	})
}

func TestTrainingStepEnvelope(t *testing.T) {
	Convey("Step observes metrics and publishes regions and grammar paths", t, func() {
		symbol := "TRN/USD"
		ctx := context.Background()
		desk, price := broker.PaperBook(ctx, symbol, nil)
		at := time.Now().UTC()

		for i := range 48 {
			desk.Update([]*market.TradeData{{
				Symbol:    symbol,
				Side:      market.SideBuy,
				Price:     decimal.NewFromInt(100 + int64(i)),
				Qty:       decimal.NewFromInt(50),
				Timestamp: at.Add(time.Duration(i) * time.Second),
			}})
		}

		training := &Training{
			System:     runtime.NewSystem(ctx, "training"),
			price:      price,
			desk:       desk,
			grid:       store.NewGrid(ctx),
			sequencers: &sync.Map{},
		}

		quiet := runtime.NewEnvelope(nil)
		quiet.CVD[symbol] = &equation.FlowOutput{Drive: 0.05}
		quiet.Pumpdump[symbol] = &equation.IgnitionOutput{Compression: 0.05}
		training.Step(quiet)

		signals := runtime.NewEnvelope(nil)
		signals.CVD[symbol] = &equation.FlowOutput{Drive: 1}
		signals.Pumpdump[symbol] = &equation.IgnitionOutput{Compression: 1.5, Ignition: 0.85}

		last := training.Step(signals)

		So(last, ShouldEqual, signals)
		So(len(last.Regions[symbol]), ShouldEqual, 11)

		var action *runtime.Region
		for _, region := range last.Regions[symbol] {
			if region == nil || region.Key > 8 || region.Brightness <= 0 {
				continue
			}

			if action == nil || region.Brightness > action.Brightness {
				action = region
			}
		}

		So(action, ShouldNotBeNil)
		So(last.Paths[symbol], ShouldNotBeNil)
		So(last.Paths[symbol].Len(), ShouldBeGreaterThan, 0)
		So(last.Paths[symbol].String(), ShouldContainSubstring, fmt.Sprintf("R%02d", action.Key))
	})

	Convey("Flush persists every warmed sequencer without a store client", t, func() {
		ctx := context.Background()
		desk, price := broker.PaperBook(ctx, "FLU/USD", nil)
		training := NewTraining(ctx, price, desk, nil)

		seq := training.Sequencer("FLU/USD")
		_, mutated := seq.Step("R02", 0.6)
		So(mutated, ShouldBeTrue)

		So(training.Flush(), ShouldBeNil)
	})
}
