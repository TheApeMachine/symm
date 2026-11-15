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
func (*modelTransportStub) Watch(string, any, json.Unmarshaler) {}

func TestNewModel(t *testing.T) {
	Convey("Setup", t, func() {
		ctx := context.Background()
		desk := &broker.Desk{}

		Convey("When NewModel is called", func() {
			model := NewModel(ctx, desk, nil, func(string) {}, nil)

			Convey("Model wires desk board rotation and tree", func() {
				So(model, ShouldNotBeNil)
				So(model.desk, ShouldEqual, desk)
				So(model.board, ShouldNotBeNil)
				So(model.rotation, ShouldNotBeNil)
				So(model.tree.Load(), ShouldNotBeNil)
			})
		})
	})
}

func TestNoteSettled(t *testing.T) {
	Convey("Setup", t, func() {
		ctx := context.Background()
		model := NewModel(ctx, &broker.Desk{}, nil, func(string) {}, nil)
		win := decimal.NewFromFloat(1.0)

		Convey("When a profitable position settles", func() {
			model.NoteSettled(&broker.Position{Symbol: "BTC/USD", PnLPct: &win})

			want := system.Cfg.Selection.RotationCloseBoost

			Convey("Rotation score matches one win close", func() {
				So(model.rotation.Score("BTC/USD"), ShouldEqual, want)
			})
		})
	})
}

func TestHold(t *testing.T) {
	Convey("Setup", t, func() {
		model := &Model{}
		position := &broker.Position{EntryPath: "R00/R01"}

		Convey("When Hold records a distinct path", func() {
			model.Hold(position, "R00/R02")

			Convey("HoldPaths carries the latest path", func() {
				So(position.HoldPaths, ShouldResemble, []string{"R00/R02"})
			})
		})

		Convey("When Hold is called with the entry path", func() {
			model.Hold(position, "R00/R01")

			Convey("HoldPaths stays empty", func() {
				So(len(position.HoldPaths), ShouldEqual, 0)
			})
		})

		Convey("When Hold repeats the same path", func() {
			model.Hold(position, "R00/R02")
			model.Hold(position, "R00/R02")

			Convey("HoldPaths is not duplicated", func() {
				So(position.HoldPaths, ShouldResemble, []string{"R00/R02"})
			})
		})
	})
}

func TestExit(t *testing.T) {
	Convey("Setup", t, func() {
		tree := emptyTree()
		model := &Model{tree: &atomic.Pointer[Tree]{}}
		model.tree.Store(tree)

		Convey("When policy applyAction sees a loss position", func() {
			loss := decimal.NewFromFloat(-0.07)
			position := &broker.Position{
				Symbol:    "XLM/USD",
				TierKey:   broker.TierTypeVirtual.String(),
				EntryPath: "R00/R01",
				PnLPct:    &loss,
			}

			var got *runtime.Decision
			decide := func(d *runtime.Decision) { got = d }

			model.applyAction(
				decide, func(EnterCandidate) {}, "XLM/USD", "R00/M1", ActionExit, 0.5, false, false,
				[]*broker.Position{position},
			)

			Convey("Decision is hold at the pnl gate", func() {
				So(got.Action, ShouldEqual, "hold")
				So(got.Gate, ShouldEqual, "pnl")
				So(position.ExitPath, ShouldEqual, "")
			})
		})

		Convey("When policy applyAction sees profit at the floor", func() {
			model := &Model{
				tree:      &atomic.Pointer[Tree]{},
				resetPath: func(string) {},
			}
			model.tree.Store(emptyTree())

			win := decimal.NewFromFloat(0.5)
			position := &broker.Position{
				Symbol:    "XLM/USD",
				TierKey:   broker.TierTypeVirtual.String(),
				EntryPath: "R00/R01",
				PnLPct:    &win,
			}

			var got *runtime.Decision
			decide := func(d *runtime.Decision) { got = d }

			model.applyAction(
				decide, func(EnterCandidate) {}, "XLM/USD", "R00/M1", ActionExit, 0.5, false, false,
				[]*broker.Position{position},
			)

			Convey("ExitPath waits until desk.Exit succeeds", func() {
				So(got.Action, ShouldEqual, "exit")
				So(position.ExitPath, ShouldEqual, "")
			})
		})

		Convey("When a settled position trains the tree", func() {
			entry := "R06.R00/R07.R01/R08.R02"
			exit := "R09.R03/R10.R04/R11.R05"
			pnl := decimal.NewFromFloat(-0.04)
			closed := time.Now().UTC()

			position := &broker.Position{
				Symbol:    "AKE/USD",
				EntryPath: entry,
				HoldPaths: []string{exit},
				PnL:       &pnl,
				ClosedAt:  &closed,
			}

			path := model.exitCut(nil, position.Symbol, position)
			model.Exit(position, path)

			enterLeaf, ok := get(tree, entry)
			exitLeaf, okExit := get(tree, exit)

			Convey("Ensure seeds enter and exit leaves", func() {
				So(path, ShouldEqual, exit)
				So(ok, ShouldBeTrue)
				So(enterLeaf.Action, ShouldEqual, "enter")
				So(enterLeaf.Resistance, ShouldEqual, seedResistance()+0.05)
				So(okExit, ShouldBeTrue)
				So(exitLeaf.Action, ShouldEqual, "exit")
				So(position.TreeApplied, ShouldBeTrue)
			})
		})

		Convey("When exitCut would reuse EntryPath on an open position", func() {
			cut := tokensAt("R06.R00/R07.R01", 1.0)
			envelope := &runtime.Envelope{
				Paths: map[string]*runtime.Tokens{
					"AKE/USD": &cut,
				},
			}
			position := &broker.Position{
				Symbol:    "AKE/USD",
				EntryPath: "R06.R00/R07.R01",
			}

			Convey("exitCut returns empty", func() {
				So(model.exitCut(envelope, "AKE/USD", position), ShouldEqual, "")
			})
		})
	})
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

func TestStep(t *testing.T) {
	Convey("Setup", t, func() {
		Convey("When the live model sees an oracle veto on a flat symbol", func() {
			prev := system.Cfg.Market.Model
			system.Cfg.Market.Model = "real"
			defer func() { system.Cfg.Market.Model = prev }()

			desk := broker.NewDesk(context.Background(), map[string]network.Client{
				"public":  &modelTransportStub{},
				"private": &modelTransportStub{},
			}, nil, nil)

			tree := emptyTree()
			model := &Model{
				desk:      desk,
				tree:      &atomic.Pointer[Tree]{},
				resetPath: func(string) {},
				live:      true,
			}
			model.tree.Store(tree)

			cut := runtime.Tokens{
				{Symbols: []string{"R17"}, Brightness: 1},
			}
			envelope := runtime.NewEnvelope(nil)
			envelope.Paths["BTC/USD"] = &cut
			envelope.Regions["BTC/USD"] = []*runtime.Region{
				{Key: 17, Brightness: 0.9},
			}

			out := model.Step(envelope)
			got := out.Decisions["BTC/USD/real"]

			Convey("Decision is a real-tier oracle skip", func() {
				So(got, ShouldNotBeNil)
				So(got.Tier, ShouldEqual, "real")
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

func TestBypassReading(t *testing.T) {
	Convey("Setup", t, func() {
		model := &Model{}

		Convey("When bypassReading fabricates lab resonance", func() {
			rez := model.bypassReading(system.Cfg.Learning)

			Convey("Calibrated gates pass defaults", func() {
				So(rez["calibrated"], ShouldEqual, 1)
				So(rez["surprise"], ShouldEqual, 1)
				So(rez["task_relative_precision"], ShouldEqual, 1)
				horizon := float64(max(system.Cfg.Learning.MinSupportedHorizon, 1))
				So(rez["supported_horizon"], ShouldEqual, horizon)
			})
		})
	})
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

			action, resistance, disagreed := tree.Predict(cut)

			Convey("Tree predicts enter within paper resistance", func() {
				So(disagreed, ShouldBeFalse)
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
		_ = NewModel(ctx, desk, nil, func(string) {}, nil)
	}
}

func BenchmarkNoteSettled(b *testing.B) {
	ctx := context.Background()
	model := NewModel(ctx, &broker.Desk{}, nil, func(string) {}, nil)
	win := decimal.NewFromFloat(1.0)
	pos := &broker.Position{Symbol: "BTC/USD", PnLPct: &win}

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		model.NoteSettled(pos)
	}
}

func BenchmarkHold(b *testing.B) {
	model := &Model{}
	position := &broker.Position{EntryPath: "R00/R01"}

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		model.Hold(position, "R00/R02")
		position.HoldPaths = nil
	}
}

func BenchmarkExit(b *testing.B) {
	tree := emptyTree()
	entry := "R06.R00/R07.R01/R08.R02"
	exit := "R09.R03/R10.R04/R11.R05"
	put(tree, entry, leaf(ActionEnter, 0.75))
	put(tree, exit, leaf(ActionExit, 0.75))

	pnl := decimal.NewFromFloat(-0.04)
	closed := time.Now().UTC()
	position := &broker.Position{
		Symbol:    "AKE/USD",
		EntryPath: entry,
		HoldPaths: []string{exit},
		PnL:       &pnl,
		ClosedAt:  &closed,
	}
	model := &Model{tree: &atomic.Pointer[Tree]{}}
	model.tree.Store(tree)

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		path := model.exitCut(nil, position.Symbol, position)
		model.Exit(position, path)
		position.TreeApplied = false
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
	prev := system.Cfg.Market.Model
	system.Cfg.Market.Model = "real"
	defer func() { system.Cfg.Market.Model = prev }()

	desk := broker.NewDesk(context.Background(), map[string]network.Client{
		"public":  &modelTransportStub{},
		"private": &modelTransportStub{},
	}, nil, nil)
	tree := emptyTree()
	model := &Model{
		desk:      desk,
		tree:      &atomic.Pointer[Tree]{},
		resetPath: func(string) {},
		live:      true,
	}
	model.tree.Store(tree)
	cut := runtime.Tokens{{Symbols: []string{"R17"}, Brightness: 1}}
	envelope := runtime.NewEnvelope(nil)
	envelope.Paths["BTC/USD"] = &cut
	envelope.Regions["BTC/USD"] = []*runtime.Region{{Key: 17, Brightness: 0.9}}

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		_ = model.Step(envelope)
	}
}

func BenchmarkBypassReading(b *testing.B) {
	model := &Model{}
	cfg := system.Cfg.Learning

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		_ = model.bypassReading(cfg)
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
