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

	// Settled lots must hit the tree even when the symbol drops off this tick's
	// path map — otherwise Exit never Inserts and model/paths stays empty.
	model.flushPending(envelope)

	for symbol, cut := range envelope.Paths {
		if cut == nil {
			continue
		}

		path := cut.String()

		rez := envelope.Resonance[symbol]
		learn := system.Cfg.Learning
		bypass := learn != nil && learn.ResonanceBypass

		if rez == nil {
			if !bypass {
				envelope.Decide(&runtime.Decision{
					Symbol: symbol,
					Action: "skip",
					Reason: "no resonance reading",
					Gate:   "resonance",
					Path:   path,
				})

				continue
			}

			rez = model.bypassReading(learn)
		}

		surprise := rez["surprise"]
		supported := int(rez["supported_horizon"])
		confidence, hasPrecision := rez["task_relative_precision"]
		minHorizon := 2
		minPrecision := 0.5
		minSurprise := 0.5

		if learn != nil {
			minHorizon = learn.MinSupportedHorizon
			minPrecision = learn.MinTaskRelativePrecision
			minSurprise = learn.MinSurprise
		}

		if !bypass && (rez["calibrated"] < 1 || supported < minHorizon || !hasPrecision || confidence < minPrecision || surprise < minSurprise) {
			envelope.Decide(&runtime.Decision{
				Symbol:     symbol,
				Action:     "skip",
				Reason:     model.gateReason(rez, supported, hasPrecision, confidence, surprise, minHorizon, minPrecision, minSurprise),
				Gate:       "resonance",
				Path:       path,
				Confidence: confidence,
			})

			continue
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
			if !bypass {
				envelope.Decide(&runtime.Decision{
					Symbol:     symbol,
					Action:     "skip",
					Reason:     "forward curve incomplete",
					Gate:       "resonance",
					Path:       path,
					Confidence: confidence,
				})

				continue
			}

			enterNet, holdNet, exitNet, risk = 1, 1, 0, 0
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

		minEnter := 0.35

		if learn != nil {
			minEnter = learn.MinEnterZ
		}

		bullish := bypass || (enterNet >= minEnter &&
			holdNet >= minEnter &&
			exitNet <= holdNet &&
			risk < minEnter)

		if disagreed {
			envelope.Decide(&runtime.Decision{
				Symbol:     symbol,
				Action:     "skip",
				Reason:     "tree leaves disagree under cut",
				Gate:       "tree",
				Path:       path,
				Confidence: confidence,
			})

			continue
		}

		wantEnter := position == nil &&
			(action == ActionEnter || action == ActionNone && bullish && cut.Len() > 0)

		if wantEnter && !Board(envelope.Regions[symbol]).Tradable() {
			envelope.Decide(&runtime.Decision{
				Symbol:     symbol,
				Action:     "skip",
				Reason:     "oracle veto lit — not tradable",
				Gate:       "oracle",
				Path:       path,
				Confidence: confidence,
			})

			continue
		}

		if action != ActionNone {
			model.applyAction(
				envelope, symbol, path, action, resistance,
				bullish, bypass, position, confidence,
			)

			continue
		}

		minPath := 0

		if learn != nil {
			minPath = learn.MinPathLength
		}

		if position != nil && path != "" && action != ActionExit {
			if minPath <= 0 || cut.Len() >= minPath {
				model.Hold(position, path)
				envelope.Decide(&runtime.Decision{
					Symbol:     symbol,
					Action:     "hold",
					Reason:     "open lot tracking live path",
					Path:       path,
					Confidence: confidence,
				})

				continue
			}
		}

		if cut.Len() > 0 && position == nil && !bullish {
			envelope.Decide(&runtime.Decision{
				Symbol:     symbol,
				Action:     "skip",
				Reason:     "forward story not bullish",
				Gate:       "story",
				Path:       path,
				Confidence: confidence,
			})

			continue
		}

		if minPath > 0 && cut.Len() < minPath {
			envelope.Decide(&runtime.Decision{
				Symbol:     symbol,
				Action:     "skip",
				Reason:     "path shorter than min length",
				Gate:       "path",
				Path:       path,
				Confidence: confidence,
			})

			continue
		}

		if wantEnter {
			model.exploreEnter(envelope, symbol, path, confidence)
		}
	}

	return envelope
}

func (model *Model) applyAction(
	envelope *runtime.Envelope,
	symbol, path string,
	action Action,
	resistance float64,
	bullish, bypass bool,
	position *broker.Position,
	confidence float64,
) {
	switch action {
	case ActionEnter:
		virtual := bypass || resistance > 0.2

		if !bullish {
			envelope.Decide(&runtime.Decision{
				Symbol:     symbol,
				Action:     "skip",
				Reason:     "enter leaf but forward story not bullish",
				Gate:       "story",
				Path:       path,
				Confidence: confidence,
			})

			return
		}

		if !virtual {
			envelope.Decide(&runtime.Decision{
				Symbol:     symbol,
				Action:     "skip",
				Reason:     "enter leaf resistance too low for virtual sample",
				Gate:       "resistance",
				Path:       path,
				Confidence: confidence,
			})

			return
		}

		if err := model.Enter(symbol, path, true, false); err != nil {
			gate, reason := "desk", err.Error()

			if denial, ok := err.(*broker.EntryDenial); ok && denial != nil {
				gate, reason = denial.Gate, denial.Reason
			}

			envelope.Decide(&runtime.Decision{
				Symbol:     symbol,
				Action:     "blocked",
				Reason:     reason,
				Gate:       gate,
				Path:       path,
				Virtual:    true,
				Confidence: confidence,
			})

			return
		}

		envelope.Decide(&runtime.Decision{
			Symbol:     symbol,
			Action:     "enter",
			Reason:     "tree enter · virtual sample",
			Path:       path,
			Virtual:    true,
			Confidence: confidence,
		})

	case ActionExit:
		if !bullish && position != nil {
			model.Exit(position, path)
			envelope.Decide(&runtime.Decision{
				Symbol:     symbol,
				Action:     "exit",
				Reason:     "tree exit · story no longer bullish",
				Path:       path,
				Confidence: confidence,
			})
		} else {
			envelope.Decide(&runtime.Decision{
				Symbol:     symbol,
				Action:     "skip",
				Reason:     "exit leaf without open lot or still bullish",
				Gate:       "story",
				Path:       path,
				Confidence: confidence,
			})
		}

		model.resetPath(symbol)

	case ActionHold:
		if position != nil {
			model.Hold(position, path)
			envelope.Decide(&runtime.Decision{
				Symbol:     symbol,
				Action:     "hold",
				Reason:     "tree hold",
				Path:       path,
				Confidence: confidence,
			})
		} else {
			envelope.Decide(&runtime.Decision{
				Symbol:     symbol,
				Action:     "skip",
				Reason:     "hold leaf with no open lot",
				Gate:       "position",
				Path:       path,
				Confidence: confidence,
			})
		}

		model.resetPath(symbol)
	}
}

func (model *Model) gateReason(
	rez map[string]float64,
	supported int,
	hasPrecision bool,
	confidence, surprise float64,
	minHorizon int,
	minPrecision, minSurprise float64,
) string {
	if rez["calibrated"] < 1 {
		return "resonance not calibrated"
	}

	if supported < minHorizon {
		return "supported horizon below minimum"
	}

	if !hasPrecision {
		return "task-relative precision not ready"
	}

	if confidence < minPrecision {
		return "task-relative precision below threshold"
	}

	if surprise < minSurprise {
		return "surprise below threshold"
	}

	return "resonance gate"
}

/*
bypassReading invents a calibrated one-step curve so lab bypass can exercise
story/tree/desk without a live manifold.
*/
func (model *Model) bypassReading(learn *system.Learning) map[string]float64 {
	horizon := len(resonance.Features)

	if learn != nil && learn.MinSupportedHorizon > horizon {
		horizon = learn.MinSupportedHorizon
	}

	rez := map[string]float64{
		"calibrated":              1,
		"surprise":                1,
		"task_relative_precision": 1,
		"supported_horizon":       float64(horizon),
	}

	// Enter/hold det+sup and every oracle approve dim; vetoes stay flat.
	for i := range horizon {
		val := 0.0

		switch {
		case i == 0 || i == 1 || i == 6 || i == 7:
			val = 1
		case i >= 9 && i%2 == 0:
			val = 1
		}

		rez["forward_curve_"+strconv.Itoa(i)] = val
	}

	return rez
}

/*
exploreEnter opens a cold-tree virtual sample when story (or bypass) clears
and no enter leaf exists yet — otherwise the tree never seeds.
*/
func (model *Model) exploreEnter(
	envelope *runtime.Envelope,
	symbol, path string,
	confidence float64,
) {
	if err := model.Enter(symbol, path, true, false); err != nil {
		gate, reason := "desk", err.Error()

		if denial, ok := err.(*broker.EntryDenial); ok && denial != nil {
			gate, reason = denial.Gate, denial.Reason
		}

		envelope.Decide(&runtime.Decision{
			Symbol:     symbol,
			Action:     "blocked",
			Reason:     reason,
			Gate:       gate,
			Path:       path,
			Virtual:    true,
			Confidence: confidence,
		})

		return
	}

	envelope.Decide(&runtime.Decision{
		Symbol:     symbol,
		Action:     "enter",
		Reason:     "cold-tree explore · virtual sample",
		Path:       path,
		Virtual:    true,
		Confidence: confidence,
	})
}

func (model *Model) flushPending(envelope *runtime.Envelope) {
	if model == nil || model.desk == nil {
		return
	}

	for _, symbol := range model.desk.Pending() {
		pending, ok := model.desk.TakePending(symbol)

		if !ok || pending == nil {
			continue
		}

		path := ""

		if cut := envelope.Paths[symbol]; cut != nil {
			path = cut.String()
		}

		if path == "" {
			path = pending.ExitPath
		}

		if path == "" {
			if n := len(pending.HoldPaths); n > 0 {
				path = pending.HoldPaths[n-1]
			}
		}

		// Never fall back to EntryPath — exit leaves must stay distinct.
		model.Exit(pending, path)
	}
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

	// One live cut only — a diary of every board flicker is not a policy.
	if len(position.HoldPaths) == 1 && position.HoldPaths[0] == path {
		return
	}

	position.HoldPaths = []string{path}
}

func (model *Model) Exit(
	position *broker.Position, path string,
) {
	if position == nil {
		return
	}

	if path == "" {
		if n := len(position.HoldPaths); n > 0 {
			path = position.HoldPaths[n-1]
		}
	}

	if path != "" {
		position.ExitPath = path
	}

	tree := model.tree.Load()

	if tree == nil {
		return
	}

	// Virtual episode = enter + exit. Hold paths are live tracking, not leaves.
	if position.Virtual {
		tree.Insert(
			position.EntryPath,
			data.NewValue("model", "action", "enter"),
		)
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
	tree.Update(position.ExitPath, resistance)
}
