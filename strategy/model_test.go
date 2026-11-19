package strategy

import (
	"context"
	"encoding/json"
	"math"
	"sync/atomic"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/nomagique/runtime"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/network"
	"github.com/theapemachine/symm/system"
)

type modelTransportStub struct{}

func (*modelTransportStub) Read(...string) ([]byte, error) { return nil, nil }
func (*modelTransportStub) Write([]byte) error             { return nil }
func (*modelTransportStub) Post(string, any) ([]byte, error) {
	return []byte(`{"error":[],"result":{"trades":{}}}`), nil
}
func (*modelTransportStub) Watch(string, any, json.Unmarshaler, func()) {}

func TestNewModel(t *testing.T) {
	Convey("Setup", t, func() {
		ctx := context.Background()
		desk := &broker.Desk{}

		Convey("When NewModel is called", func() {
			model := NewModel(ctx, desk, nil, func(string) {}, nil, nil)

			Convey("Model wires desk board and tree", func() {
				So(model, ShouldNotBeNil)
				So(model.desk, ShouldEqual, desk)
				So(model.board, ShouldNotBeNil)
				So(model.tree.Load(), ShouldNotBeNil)
			})
		})
	})
}

func TestGateReason(t *testing.T) {
	Convey("Setup", t, func() {
		model := &Model{}
		learn := system.Cfg.Learning

		Convey("When resonance is not calibrated", func() {
			rez := map[string]float64{"calibrated": 0}
			So(model.gateReason(rez, 2, true, 1, 1, 0.35), ShouldEqual, "resonance not calibrated")
		})

		Convey("When supported horizon is cold", func() {
			rez := map[string]float64{
				"calibrated":        1,
				"supported_horizon": 0,
			}
			So(model.gateReason(rez, 0, true, 1, 1, 0.35),
				ShouldEqual, "supported horizon below minimum")
		})

		Convey("When task precision is missing", func() {
			rez := map[string]float64{
				"calibrated":        1,
				"supported_horizon": float64(learn.MinSupportedHorizon),
			}
			So(model.gateReason(rez, learn.MinSupportedHorizon, false, 0, 1, 0.35),
				ShouldEqual, "task-relative precision not ready")
		})

		Convey("When precision is below floor", func() {
			rez := map[string]float64{"calibrated": 1}
			floor := learn.MinTaskRelativePrecision
			So(model.gateReason(rez, learn.MinSupportedHorizon, true, floor-0.1, 1, 0.35),
				ShouldEqual, "task-relative precision below threshold")
		})

		Convey("When surprise is below floor", func() {
			rez := map[string]float64{"calibrated": 1}
			floor := learn.MinEnterZ
			So(model.gateReason(rez, learn.MinSupportedHorizon, true, 1, floor-0.1, floor),
				ShouldEqual, "surprise below threshold")
		})
	})
}

func TestExit(t *testing.T) {
	Convey("Setup", t, func() {
		tree := emptyTree()
		exitPath := "R06.R00/R07.R01"
		enterPath := "R06.R00/R07.R01/R08.R02"
		holdPath := "R06.R00/R07.R01/R08.R03"
		put(tree, exitPath, leaf(ActionExit, seedResistance()))
		put(tree, enterPath, leaf(ActionEnter, seedResistance()))
		put(tree, holdPath, leaf(ActionHold, seedResistance()))

		model := &Model{tree: &atomic.Pointer[Tree]{}}
		model.tree.Store(tree)

		Convey("When Exit is nil-safe", func() {
			So(func() { model.Exit(nil, exitPath) }, ShouldNotPanic)
		})

		Convey("When position already settled", func() {
			closed := time.Now().UTC()
			position := &broker.Position{
				System:   runtime.NewSystem(context.Background(), "position"),
				ClosedAt: &closed,
			}
			model.Exit(position, exitPath)

			Convey("Exit does not mutate the tree", func() {
				leaf, ok := get(tree, exitPath)
				So(ok, ShouldBeTrue)
				So(leaf.Resistance, ShouldEqual, seedResistance())
			})
		})

		Convey("When an open position arms an exit path", func() {
			position := &broker.Position{
				System:    runtime.NewSystem(context.Background(), "position"),
				Symbol:    "AKE/USD",
				EntryPath: enterPath,
				HoldPaths: []string{holdPath, holdPath},
			}
			model.Exit(position, exitPath)

			Convey("ExitPath stamps but tree waits for ClosedAt", func() {
				So(position.ExitPath, ShouldEqual, exitPath)
				So(position.TreeApplied, ShouldBeFalse)
				enter, ok := get(tree, enterPath)
				So(ok, ShouldBeTrue)
				So(enter.Resistance, ShouldEqual, seedResistance())

				model.creditExit(position)
				So(position.TreeApplied, ShouldBeFalse)
				So(enter.Resistance, ShouldEqual, seedResistance())
			})
		})

		Convey("When creditExit runs after ClosedAt", func() {
			closed := time.Now().UTC()
			position := &broker.Position{
				System:    runtime.NewSystem(context.Background(), "position"),
				Symbol:    "AKE/USD",
				EntryPath: enterPath,
				HoldPaths: []string{holdPath, holdPath},
				ExitPath:  exitPath,
				ClosedAt:  &closed,
			}
			model.creditExit(position)

			Convey("Entry, hold, and exit leaves all nudge once", func() {
				So(position.TreeApplied, ShouldBeTrue)
				want := seedResistance() + learnStep()
				enter, ok := get(tree, enterPath)
				So(ok, ShouldBeTrue)
				So(enter.Resistance, ShouldEqual, want)
				hold, ok := get(tree, holdPath)
				So(ok, ShouldBeTrue)
				So(hold.Resistance, ShouldEqual, want)
				exit, ok := get(tree, exitPath)
				So(ok, ShouldBeTrue)
				So(exit.Resistance, ShouldEqual, want)

				model.creditExit(position)
				So(enter.Resistance, ShouldEqual, want)
			})
		})
	})
}

func learnStep() float64 {
	return system.Cfg.Learning.ThresholdStep
}

func TestResistanceDelta(t *testing.T) {
	Convey("Setup", t, func() {
		model := &Model{}
		step := system.Cfg.Learning.ThresholdStep
		loss := decimal.NewFromFloat(-0.1)

		Convey("When PnL is zero or negative", func() {
			Convey("resistanceDelta equals ThresholdStep", func() {
				So(model.resistanceDelta(&broker.Position{PnL: &loss}), ShouldEqual, step)
				So(model.resistanceDelta(&broker.Position{}), ShouldEqual, step)
			})
		})

		Convey("When a short hold wins with fat PnL%", func() {
			win := decimal.NewFromFloat(1)
			closed := time.Now().UTC()
			halfLife := system.Cfg.Learning.ResistanceHoldHalfLifeSec
			pnlRef := system.Cfg.Learning.ResistancePnLRefPct

			fat := decimal.NewFromFloat(2.0)
			short := &broker.Position{
				PnL:      &win,
				PnLPct:   &fat,
				OpenedAt: closed.Add(-time.Second),
				ClosedAt: &closed,
			}

			thin := decimal.NewFromFloat(0.2)
			long := &broker.Position{
				PnL:      &win,
				PnLPct:   &thin,
				OpenedAt: closed.Add(-2 * time.Duration(halfLife) * time.Second),
				ClosedAt: &closed,
			}

			shortDelta := model.resistanceDelta(short)
			longDelta := model.resistanceDelta(long)
			wantShort := -step * (1.0 / (1.0 + 1.0/halfLife)) * math.Tanh(2.0/pnlRef)
			wantLong := -step * (1.0 / (1.0 + 2.0)) * math.Tanh(0.2/pnlRef)

			Convey("Short fat wins lower resistance more than long thin wins", func() {
				So(shortDelta, ShouldEqual, wantShort)
				So(longDelta, ShouldEqual, wantLong)
			})
		})
	})
}

func TestOracleAllows(t *testing.T) {
	Convey("Setup", t, func() {
		model := &Model{}

		Convey("When desk approves are lit and veto is dormant", func() {
			regions := []*runtime.Region{
				{Key: 10, Brightness: 1},
				{Key: 12, Brightness: 1},
				{Key: 14, Brightness: 1},
				{Key: 16, Brightness: 1},
				{Key: 17, Brightness: 0},
				{Key: 20, Brightness: 1},
				{Key: 22, Brightness: 1},
				{Key: 24, Brightness: 1},
			}

			Convey("oracleAllows is true", func() {
				So(model.oracleAllows(regions), ShouldBeTrue)
			})
		})

		Convey("When a lit veto is present", func() {
			regions := []*runtime.Region{
				{Key: 10, Brightness: 1},
				{Key: 17, Brightness: 0.6},
			}

			Convey("oracleAllows is false", func() {
				So(model.oracleAllows(regions), ShouldBeFalse)
			})
		})
	})
}

func TestStepRecordsDecision(t *testing.T) {
	Convey("Setup", t, func() {
		tree := emptyTree()
		model := &Model{
			desk:      &broker.Desk{},
			tree:      &atomic.Pointer[Tree]{},
			resetPath: func(string) {},
		}
		model.tree.Store(tree)

		cut := tokensAt("R06.R00/R07.R01", 1)
		envelope := runtime.NewEnvelope(nil)
		envelope.Paths["BTC/USD"] = &cut
		envelope.Regions["BTC/USD"] = []*runtime.Region{
			{Key: 17, Brightness: 0.9},
		}

		out := model.Step(envelope)

		Convey("Step records a decision on the envelope", func() {
			So(out, ShouldNotBeNil)
			got := out.Decisions["BTC/USD/virtual"]
			So(got, ShouldNotBeNil)
			So(got.Symbol, ShouldEqual, "BTC/USD")
			So(got.Tier, ShouldEqual, "virtual")
		})
	})
}

func TestStep(t *testing.T) {
	Convey("Setup", t, func() {
		Convey("When Step sees an oracle veto on a flat symbol", func() {
			ctx := context.Background()
			price := broker.NewPrice(ctx, nil, &broker.Instrument{}, nil, nil, nil)
			desk := broker.NewDesk(ctx, map[string]network.Client{
				"public":  &modelTransportStub{},
				"private": &modelTransportStub{},
			}, price, nil)

			tree := emptyTree()
			model := &Model{
				desk:      desk,
				tree:      &atomic.Pointer[Tree]{},
				resetPath: func(string) {},
			}
			model.tree.Store(tree)

			cut := tokensAt("R06.R00/R07.R01", 1)
			envelope := runtime.NewEnvelope(nil)
			envelope.Paths["BTC/USD"] = &cut
			envelope.Regions["BTC/USD"] = []*runtime.Region{
				{Key: 17, Brightness: 0.9},
			}

			out := model.Step(envelope)
			got := out.Decisions["BTC/USD/virtual"]

			Convey("Decision is an oracle skip", func() {
				So(got, ShouldNotBeNil)
				So(got.Action, ShouldEqual, "skip")
				So(got.Gate, ShouldEqual, "oracle")
			})
		})

		Convey("When Step receives a nil envelope", func() {
			model := &Model{tree: &atomic.Pointer[Tree]{}}
			model.tree.Store(emptyTree())

			Convey("Step returns nil", func() {
				So(model.Step(nil), ShouldBeNil)
			})
		})
	})
}

func TestOpenPosition(t *testing.T) {
	Convey("Setup", t, func() {
		ctx := context.Background()
		price := broker.NewPrice(ctx, nil, &broker.Instrument{}, nil, nil, nil)
		desk := broker.NewDesk(ctx, map[string]network.Client{
			"public":  &modelTransportStub{},
			"private": &modelTransportStub{},
			"paper":   &modelTransportStub{},
			"virtual": &modelTransportStub{},
		}, price, nil)
		model := &Model{desk: desk}

		Convey("When no tier holds the symbol", func() {
			So(model.openPosition(""), ShouldBeNil)
			So(model.openPosition("BTC/USD"), ShouldBeNil)
			So(model.openTierKey("BTC/USD"), ShouldEqual, "")
		})

		Convey("When routeTier picks paper for a known low-resistance path", func() {
			So(model.routeTier(0.0, true), ShouldEqual, broker.TierTypePaper)
			So(model.decisionTier("BTC/USD", 0.0), ShouldEqual, "paper")
			So(desk.Position(broker.TierTypeVirtual, "BTC/USD"), ShouldBeNil)
			So(desk.Position(broker.TierTypePaper, "BTC/USD"), ShouldBeNil)
		})

		Convey("When routeTier sees a cold explore at resistance 0", func() {
			So(model.routeTier(0.0, false), ShouldEqual, broker.TierTypeVirtual)
		})

		Convey("When the model has no desk", func() {
			So((&Model{}).openPosition("BTC/USD"), ShouldBeNil)
		})
	})
}

func TestStepSkipsIdleMatchedActions(t *testing.T) {
	Convey("Setup", t, func() {
		ctx := context.Background()
		price := broker.NewPrice(ctx, nil, &broker.Instrument{}, nil, nil, nil)
		desk := broker.NewDesk(ctx, map[string]network.Client{
			"public":  &modelTransportStub{},
			"private": &modelTransportStub{},
			"paper":   &modelTransportStub{},
			"virtual": &modelTransportStub{},
		}, price, nil)

		tree := emptyTree()
		put(tree, "R06.R00/R07.R01/R08.R02", leaf(ActionHold, 0.15))
		resets := 0
		model := &Model{
			desk: desk,
			tree: &atomic.Pointer[Tree]{},
			resetPath: func(string) {
				resets++
			},
		}
		model.tree.Store(tree)

		Convey("When ActionHold matches on a flat book", func() {
			cut := tokensAt("R06.R00/R07.R01/R08.R02", 1)
			envelope := calibratedEnvelope("ETH/USD", &cut)
			out := model.Step(envelope)

			Convey("No decision and no sequencer reset", func() {
				So(out.Decisions["ETH/USD/virtual"], ShouldBeNil)
				So(out.Decisions["ETH/USD/paper"], ShouldBeNil)
				So(resets, ShouldEqual, 0)
			})
		})
	})
}

func TestExitSkipsUnprofitable(t *testing.T) {
	Convey("Setup", t, func() {
		ctx := context.Background()
		price := broker.NewPrice(ctx, nil, &broker.Instrument{}, nil, nil, nil)
		desk := broker.NewDesk(ctx, map[string]network.Client{
			"public":  &modelTransportStub{},
			"private": &modelTransportStub{},
			"paper":   &modelTransportStub{},
			"virtual": &modelTransportStub{},
		}, price, nil)
		resets := 0
		model := &Model{
			desk: desk,
			tree: &atomic.Pointer[Tree]{},
			resetPath: func(string) { resets++ },
		}
		model.tree.Store(emptyTree())

		Convey("When ProfitableExit is false", func() {
			position := broker.NewPosition(
				ctx, "ETH/USD", "virtual", "enter", price, nil, func(*broker.Position) {},
			)
			ok := model.Exit(position, "R06.R00/R07.R01")

			Convey("Exit returns false and does not reset the path", func() {
				So(ok, ShouldBeFalse)
				So(resets, ShouldEqual, 0)
				So(position.ExitPath, ShouldEqual, "")
			})
		})
	})
}

func calibratedEnvelope(symbol string, cut *runtime.Tokens) *runtime.Envelope {
	learn := system.Cfg.Learning
	envelope := runtime.NewEnvelope(nil)
	envelope.Paths[symbol] = cut
	envelope.Regions[symbol] = []*runtime.Region{
		{Key: 10, Brightness: 1},
		{Key: 12, Brightness: 1},
		{Key: 14, Brightness: 1},
		{Key: 16, Brightness: 1},
		{Key: 17, Brightness: 0},
	}
	rez := map[string]float64{
		"calibrated":              1,
		"supported_horizon":       float64(learn.MinSupportedHorizon),
		"task_relative_precision": learn.MinTaskRelativePrecision,
		"surprise":                learn.MinSurprise + 1,
		"surprise_regime_z":       learn.MinEnterZ + 1,
		"forward_curve_0":         1,
		"forward_curve_1":         1,
		"forward_curve_2":         0,
		"forward_curve_3":         0,
		"forward_curve_4":         0,
		"forward_curve_5":         0,
		"forward_curve_6":         0.5,
		"forward_curve_7":         0.5,
		"forward_curve_8":         0,
		"forward_curve_9":         0,
		"forward_curve_10":        0,
		"forward_curve_11":        0,
		"forward_curve_12":        0,
		"forward_curve_13":        0,
		"forward_curve_14":        0,
	}
	envelope.Resonance[symbol] = rez
	return envelope
}

func TestPredictIntegration(t *testing.T) {
	Convey("Setup", t, func() {
		tree := paperPromotedTree()
		model := &Model{tree: &atomic.Pointer[Tree]{}}
		model.tree.Store(tree)

		cut := tokensAt("R06.R00/R07.R01/R08.R02", 1)
		regions := []*runtime.Region{
			{Key: 10, Brightness: 1},
			{Key: 12, Brightness: 1},
			{Key: 14, Brightness: 1},
			{Key: 16, Brightness: 1},
			{Key: 17, Brightness: 0},
		}

		Convey("When oracle approves a promoted paper path", func() {
			So(model.oracleAllows(regions), ShouldBeTrue)

			action, resistance, matched := tree.Predict(cut)

			Convey("Tree predicts enter within paper resistance", func() {
				So(matched, ShouldBeTrue)
				So(action, ShouldEqual, ActionEnter)
				So(resistance, ShouldEqual, 0.15)
			})
		})
	})
}

func paperPromotedTree() *Tree {
	tree := emptyTree()

	for path, decision := range map[string]*runtime.Decision{
		"R06.R00":                 leaf(ActionEnter, 0.15),
		"R06.R00/R07.R01":         leaf(ActionEnter, 0.15),
		"R06.R00/R07.R01/R08.R02": leaf(ActionEnter, 0.15),
		"R07.R01":                 leaf(ActionHold, 0.15),
		"R07.R01/R08.R02":         leaf(ActionHold, 0.15),
		"R07.R01/R08.R02/R09.R03": leaf(ActionHold, 0.15),
		"R08.R01":                 leaf(ActionExit, 0.15),
		"R08.R01/R09.R02":         leaf(ActionExit, 0.15),
		"R08.R01/R09.R02/R10.R03": leaf(ActionExit, 0.15),
	} {
		put(tree, path, decision)
	}

	return tree
}

func BenchmarkNewModel(b *testing.B) {
	ctx := context.Background()
	desk := &broker.Desk{}

	b.ReportAllocs()

	for b.Loop() {
		_ = NewModel(ctx, desk, nil, func(string) {}, nil, nil)
	}
}

func BenchmarkExit(b *testing.B) {
	tree := emptyTree()
	exitPath := "R06.R00/R07.R01/R08.R02"
	put(tree, exitPath, leaf(ActionExit, 0.75))

	position := &broker.Position{
		System: runtime.NewSystem(context.Background(), "position"),
		Symbol: "AKE/USD",
	}
	model := &Model{tree: &atomic.Pointer[Tree]{}}
	model.tree.Store(tree)

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		position.ExitPath = ""
		model.Exit(position, exitPath)
	}
}

func BenchmarkResistanceDelta(b *testing.B) {
	model := &Model{}
	win := decimal.NewFromFloat(1)
	closed := time.Now().UTC()
	fat := decimal.NewFromFloat(2.0)
	short := &broker.Position{
		PnL:      &win,
		PnLPct:   &fat,
		OpenedAt: closed.Add(-time.Second),
		ClosedAt: &closed,
	}

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		_ = model.resistanceDelta(short)
	}
}

func BenchmarkOracleAllows(b *testing.B) {
	model := &Model{}
	regions := []*runtime.Region{
		{Key: 10, Brightness: 1},
		{Key: 12, Brightness: 1},
		{Key: 14, Brightness: 1},
		{Key: 16, Brightness: 1},
		{Key: 17, Brightness: 0},
		{Key: 20, Brightness: 1},
		{Key: 22, Brightness: 1},
		{Key: 24, Brightness: 1},
	}

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		_ = model.oracleAllows(regions)
	}
}

func BenchmarkStep(b *testing.B) {
	ctx := context.Background()
	price := broker.NewPrice(ctx, nil, &broker.Instrument{}, nil, nil, nil)
	desk := broker.NewDesk(ctx, map[string]network.Client{
		"public":  &modelTransportStub{},
		"private": &modelTransportStub{},
	}, price, nil)
	tree := emptyTree()
	model := &Model{
		desk:      desk,
		tree:      &atomic.Pointer[Tree]{},
		resetPath: func(string) {},
	}
	model.tree.Store(tree)
	cut := tokensAt("R06.R00/R07.R01", 1)
	envelope := runtime.NewEnvelope(nil)
	envelope.Paths["BTC/USD"] = &cut
	envelope.Regions["BTC/USD"] = []*runtime.Region{{Key: 17, Brightness: 0.9}}

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		_ = model.Step(envelope)
	}
}

func BenchmarkPredictIntegration(b *testing.B) {
	tree := paperPromotedTree()
	cut := tokensAt("R06.R00/R07.R01/R08.R02", 1)
	regions := []*runtime.Region{
		{Key: 10, Brightness: 1},
		{Key: 12, Brightness: 1},
		{Key: 14, Brightness: 1},
		{Key: 16, Brightness: 1},
		{Key: 17, Brightness: 0},
	}
	model := &Model{}

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		_ = model.oracleAllows(regions)
		tree.Predict(cut)
	}
}