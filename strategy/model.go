package strategy

import (
	"context"
	"strconv"
	"sync/atomic"

	"github.com/theapemachine/nomagique/data"
	"github.com/theapemachine/nomagique/runtime"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/logic/resonance"
	"github.com/theapemachine/symm/store"
	"github.com/theapemachine/symm/system"
)

type Model struct {
	*runtime.System
	desk      *broker.Desk
	price     *broker.Price
	store     *store.Client
	tree      *atomic.Pointer[Tree]
	resetPath func(string)
}

func NewModel(
	ctx context.Context,
	desk *broker.Desk,
	price *broker.Price,
	resetPath func(string),
) *Model {
	client := store.NewClient(ctx, system.Cfg.Storage.S3.Bucket)

	model := &Model{
		System:    runtime.NewSystem(ctx, "model"),
		desk:      desk,
		price:     price,
		store:     client,
		tree:      &atomic.Pointer[Tree]{},
		resetPath: resetPath,
	}

	tree := NewTree(ctx, "model/paths/")
	model.tree.Store(tree)

	return model
}

func (model *Model) Step(envelope *runtime.Envelope) *runtime.Envelope {
	if envelope == nil {
		return nil
	}

	for symbol, cut := range envelope.Paths {
		path := cut.String()

		if pending, ok := model.desk.TakePending(symbol); ok {
			model.Exit(pending, path)
		}

		rez := envelope.Resonance[symbol]

		surprise := rez["surprise"]
		supported := int(rez["supported_horizon"])
		confidence := rez["task_relative_precision"]
		minHorizon := system.Cfg.Learning.MinSupportedHorizon

		if rez["calibrated"] < 1 || supported < minHorizon || confidence < 0.5 || surprise < 0.5 {
			return nil
		}

		curve := make([]float64, 0, supported)

		for i := range supported {
			value, ok := rez["forward_curve_"+strconv.Itoa(i)]

			if !ok {
				break
			}

			curve = append(curve, value)
		}

		enterNet, holdNet, exitNet, risk, ok := resonance.ForwardStoryNets(curve)

		if !ok {
			return nil
		}

		peak := enterNet

		if holdNet > peak {
			peak = holdNet
		}

		position := model.desk.Position(symbol)

		if position != nil && position.IsClosed() {
			position = nil
		}

		regime := FlatRegime()

		if position != nil {
			regime = OpenRegime()
		}

		action, resistance, disagreed := model.tree.Load().Predict(*cut, regime)

		bullish := enterNet >= system.Cfg.Learning.MinEnterZ &&
			holdNet >= system.Cfg.Learning.MinEnterZ &&
			exitNet <= holdNet &&
			risk <= enterNet

		if disagreed {
			return nil
		}

		if action != ActionNone {
			switch action {
			case ActionEnter:
				virtual := resistance > 0.2

				if bullish && virtual {
					model.Enter(symbol, path, virtual, !virtual)
				}
			case ActionExit:
				if !bullish && position != nil {
					model.Exit(position, path)
				}

				model.resetPath(symbol)
			case ActionHold:
				if position != nil {
					model.Hold(position, path)
				}

				model.resetPath(symbol)
			}

			return envelope
		}

		minPath := system.Cfg.Learning.MinPathLength

		if position != nil && path != "" && action != ActionExit {
			if minPath <= 0 || cut.Len() >= minPath {
				model.Hold(position, path)
			}
		}

		if !bullish || position != nil || cut.Len() == 0 {
			if cut.Len() > 0 && position == nil && !bullish {
				return nil
			}
		}

		if minPath > 0 && cut.Len() < minPath {
			return nil
		}
	}

	return envelope
}

func (model *Model) Enter(
	symbol string,
	path string,
	virtual bool,
	pathFound bool,
) error {
	return model.desk.Enter(symbol, path, virtual, pathFound)
}

func (model *Model) Hold(position *broker.Position, path string) {
	if position == nil || path == "" || path == position.EntryPath {
		return
	}

	n := len(position.HoldPaths)

	if n > 0 && position.HoldPaths[n-1] == path {
		return
	}

	position.HoldPaths = append(position.HoldPaths, path)
}

func (model *Model) Exit(
	position *broker.Position, path string,
) {
	if position == nil {
		return
	}

	holdPaths := position.HoldPaths

	if path == "" {
		if len(holdPaths) > 0 {
			path, holdPaths = holdPaths[len(holdPaths)-1], holdPaths[:len(holdPaths)-1]
		}
	}

	if path != "" {
		position.ExitPath = path
	}

	tree := model.tree.Load()

	if tree == nil {
		return
	}

	if position.Virtual {
		tree.Insert(
			position.EntryPath,
			data.NewValue("model", "action", "enter"),
		)

		for _, hold := range holdPaths {
			tree.Insert(hold, data.NewValue("model", "action", "hold"))
		}

		tree.Insert(
			position.ExitPath,
			data.NewValue("model", "action", "exit"),
		)
	}

	resistance := 0.05

	if position.PnL != nil && position.PnL.Sign() > 0 {
		resistance = -0.05
	}

	tree.Update(position.EntryPath, resistance)

	for _, hold := range position.HoldPaths {
		tree.Update(hold, resistance)
	}

	tree.Update(position.ExitPath, resistance)
}
