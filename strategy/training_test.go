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

func TestTrainingStepCut(t *testing.T) {
	Convey("stepCut steps the brightest action region", t, func() {
		training := &Training{
			System:     runtime.NewSystem(context.Background(), "training"),
			sequencers: &sync.Map{},
		}

		regions := []*runtime.Region{
			{Key: 3, Brightness: 0.8},
			{Key: 5, Brightness: 0.2},
			{Key: 17, Brightness: 0.9},
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
	Convey("Step returns the envelope unchanged when there are no symbol metrics", t, func() {
		ctx := context.Background()

		training := &Training{
			System:     runtime.NewSystem(ctx, "training"),
			price:      trainingBook(t, "TRN/USD"),
			desk:       &broker.Desk{},
			grid:       store.NewGrid(ctx),
			sequencers: &sync.Map{},
		}

		env := runtime.NewEnvelope(nil)

		out := training.Step(env)
		So(out, ShouldEqual, env)
		So(out.Paths["TRN/USD"], ShouldBeNil)
	})

	Convey("Flush persists every warmed sequencer without a store client", t, func() {
		ctx := context.Background()
		training := NewTraining(ctx, trainingBook(t, "FLU/USD"), &broker.Desk{}, nil)

		seq := training.Sequencer("FLU/USD")
		_, mutated := seq.Step("R02", 0.6)
		So(mutated, ShouldBeTrue)

		So(training.Flush(), ShouldBeNil)
	})
}
