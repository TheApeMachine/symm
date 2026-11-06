package strategy

import (
	"context"
	"strconv"
	"sync"
	"sync/atomic"

	"github.com/theapemachine/nomagique/runtime"
	"github.com/theapemachine/nomagique/store"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/logic/resonance"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/ui"
)

/*
Model is the main strategy engine, which is responsible for making
decisions based on the current state of the market.
*/
type Model struct {
	*runtime.System
	desk      *broker.Desk
	price     *broker.Price
	store     *store.Client
	tree      *atomic.Pointer[Tree]
	resetPath func(string)
}

/*
NewModel initializes a new model with the given context, desk, price,
and reset path function. It also initializes the tree and the store.
*/
func NewModel(
	ctx context.Context,
	desk *broker.Desk,
	price *broker.Price,
	resetPath func(string),
	client *store.Client,
) *Model {
	model := &Model{
		System:    runtime.NewSystem(ctx, "model"),
		desk:      desk,
		price:     price,
		store:     client,
		tree:      &atomic.Pointer[Tree]{},
		resetPath: resetPath,
	}

	tree := NewTree(ctx, "model/paths/", client)
	model.tree.Store(tree)

	return model
}

/*
Step is the main entry point for the model. It takes an envelope and returns 
a new envelope. This makes it compatible with the LMAX Disruptor pipeline.
*/
func (model *Model) Step(envelope *runtime.Envelope) *runtime.Envelope {
	if envelope == nil {
		return nil
	}

	for _, tier := range []broker.TierType{
		broker.TierTypeVirtual, broker.TierTypePaper, broker.TierTypeReal,
	} {
		for position := range model.desk.Pending(tier) {
			model.Exit(position, model.exitCut(envelope, position.Symbol, position))
		}
	}

	var mu sync.Mutex
	var wg sync.WaitGroup

	decide := func(decision *runtime.Decision) {
		mu.Lock()
		envelope.Decide(decision)
		mu.Unlock()

		if decision == nil || decision.Symbol == "" {
			return
		}

		ui.UI.WriteJSON(map[string]any{
			"decisions": map[string]any{decision.Symbol: decision},
		})
	}

	for symbol, cut := range envelope.Paths {
		if cut == nil {
			continue
		}

		symbol, cut := symbol, cut
		wg.Add(1)

		task := func() {
			defer wg.Done()

			path := cut.String()
			var open []*broker.Position

			for _, tt := range []broker.TierType{
				broker.TierTypeVirtual, broker.TierTypePaper, broker.TierTypeReal,
			} {
				for position := range model.desk.Positions(tt, symbol) {
					open = append(open, position)
				}
			}

			rez := envelope.Resonance[symbol]
			learn := system.Cfg.Learning
			bypass := learn.ResonanceBypass

			if rez == nil {
				if !bypass {
					decide(&runtime.Decision{
						Symbol: symbol,
						Action: "skip",
						Reason: "no resonance reading",
						Gate:   "resonance",
						Path:   path,
					})
					return
				}

				rez = model.bypassReading(learn)
			}

			surprise := rez["surprise"]
			supported := int(rez["supported_horizon"])
			precision, hasPrecision := rez["task_relative_precision"]

			if !bypass && (rez["calibrated"] < 1 ||
				supported < learn.MinSupportedHorizon ||
				!hasPrecision ||
				precision < learn.MinTaskRelativePrecision ||
				surprise < learn.MinSurprise) {
				decide(&runtime.Decision{
					Symbol: symbol,
					Action: "skip",
					Reason: model.gateReason(rez, supported, hasPrecision, precision, surprise),
					Gate:   "resonance",
					Path:   path,
				})
				return
			}

			curve := make([]float64, 0, len(resonance.Features))

			for i := range len(resonance.Features) {
				value, ok := rez["forward_curve_"+strconv.Itoa(i)]
				if !ok {
					break
				}

				curve = append(curve, value)
			}

			enterNet, holdNet, exitNet, risk, ok := resonance.ForwardStoryNets(curve)

			if !ok {
				if !bypass {
					decide(&runtime.Decision{
						Symbol: symbol,
						Action: "skip",
						Reason: "forward curve incomplete",
						Gate:   "resonance",
						Path:   path,
					})
					return
				}

				enterNet, holdNet, exitNet, risk = 1, 1, 0, 0
			}

			action, resistance, disagreed := model.tree.Load().Predict(*cut)
			minEnter := learn.MinEnterZ

			bullish := bypass || (enterNet >= minEnter &&
				holdNet >= minEnter &&
				exitNet <= holdNet &&
				risk < minEnter)

			if disagreed {
				decide(&runtime.Decision{
					Symbol: symbol,
					Action: "skip",
					Reason: "tree leaves disagree under cut",
					Gate:   "tree",
					Path:   path,
				})
				return
			}

			minPath := learn.MinPathLength

			// Cold explore (no ENTER leaf) needs a full-length cut — a 1-token
			// flicker is not a choice. Bypass does not shorten that bar.
			wantEnter := action == ActionEnter ||
				(action == ActionNone && bullish && cut.Len() >= minPath)

			// Resonance bypass is the lab "open the floodgates" switch: skip
			// the emit-board oracle too. Lab oracle lamps stay local-only.
			if wantEnter && !bypass && !Board(envelope.Regions[symbol]).Tradable() {
				decide(&runtime.Decision{
					Symbol: symbol,
					Action: "skip",
					Reason: "oracle veto lit — not tradable",
					Gate:   "oracle",
					Path:   path,
				})
				return
			}

			if action != ActionNone {
				model.applyAction(decide, symbol, path, action, resistance, bullish, bypass, open)
				return
			}

			if len(open) > 0 && path != "" && cut.Len() >= minPath {
				for _, lot := range open {
					model.Hold(lot, path)
				}

				decide(&runtime.Decision{
					Symbol: symbol,
					Action: "hold",
					Reason: "open lot tracking live path",
					Path:   path,
				})
				return
			}

			if cut.Len() > 0 && len(open) == 0 && !bullish {
				decide(&runtime.Decision{
					Symbol: symbol,
					Action: "skip",
					Reason: "forward story not bullish",
					Gate:   "story",
					Path:   path,
				})
				return
			}

			if cut.Len() < minPath {
				decide(&runtime.Decision{
					Symbol: symbol,
					Action: "skip",
					Reason: "path shorter than min length",
					Gate:   "path",
					Path:   path,
				})
				return
			}

			if wantEnter {
				model.enterTiers(decide, symbol, path, resistance, false, bypass)
			}
		}

		if system.Q.Submit(task) != nil {
			task()
		}
	}

	wg.Wait()
	return envelope
}

func (model *Model) applyAction(
	decide func(*runtime.Decision),
	symbol, path string,
	action Action,
	resistance float64,
	bullish, bypass bool,
	open []*broker.Position,
) {
	switch action {
	case ActionEnter:
		if !bullish {
			decide(&runtime.Decision{
				Symbol: symbol,
				Action: "skip",
				Reason: "enter leaf but forward story not bullish",
				Gate:   "story",
				Path:   path,
			})
			return
		}

		model.enterTiers(decide, symbol, path, resistance, true, bypass)

	case ActionExit:
		if bullish || len(open) == 0 {
			decide(&runtime.Decision{
				Symbol: symbol,
				Action: "skip",
				Reason: "exit leaf without open lot or still bullish",
				Gate:   "story",
				Path:   path,
			})
			model.resetPath(symbol)
			return
		}

		// Policy match takes profit only once fee-net PnL% clears the
		// floor. Thin greens and underwater lots stay for the hard stop;
		// seeding exit leaves from losses poisoned early trees.
		minProfit := system.Cfg.Learning.PolicyExitMinProfitPct
		exited := 0

		for _, lot := range open {
			if lot == nil || lot.PnLPct == nil ||
				lot.PnLPct.InexactFloat64() < minProfit {
				continue
			}

			model.Exit(lot, path)
			exited++
		}

		if exited == 0 {
			for _, lot := range open {
				model.Hold(lot, path)
			}

			decide(&runtime.Decision{
				Symbol: symbol,
				Action: "hold",
				Reason: "exit leaf ignored — lot below min profit",
				Gate:   "pnl",
				Path:   path,
			})
			return
		}

		decide(&runtime.Decision{
			Symbol: symbol,
			Action: "exit",
			Reason: "tree exit · story no longer bullish",
			Path:   path,
		})
		model.resetPath(symbol)

	case ActionHold:
		if len(open) > 0 {
			for _, lot := range open {
				model.Hold(lot, path)
			}

			decide(&runtime.Decision{
				Symbol: symbol,
				Action: "hold",
				Reason: "tree hold",
				Path:   path,
			})
		} else {
			decide(&runtime.Decision{
				Symbol: symbol,
				Action: "skip",
				Reason: "hold leaf with no open lot",
				Gate:   "position",
				Path:   path,
			})
		}

		model.resetPath(symbol)
	}
}

/*
enterTiers routes onto virtual / paper / real.

  - virtual: no enter path yet, or resistance above the paper floor, or bypass
  - paper:   enter path found and resistance ≤ paper_eligible_resistance
  - real:    paper rules plus Tier.Unlocked (paper win-rate / edge on real)
*/
func (model *Model) enterTiers(
	decide func(*runtime.Decision),
	symbol, path string,
	resistance float64,
	pathFound, bypass bool,
) {
	floor := system.Cfg.Learning.PaperEligibleResistance

	if bypass || !pathFound || resistance > floor {
		model.tryEnter(decide, symbol, path, broker.TierTypeVirtual, false, "virtual sample")
		return
	}

	model.tryEnter(decide, symbol, path, broker.TierTypePaper, true, "paper enter")
	model.tryEnter(decide, symbol, path, broker.TierTypeReal, true, "real enter")
}

func (model *Model) tryEnter(
	decide func(*runtime.Decision),
	symbol, path string,
	tt broker.TierType,
	pathFound bool,
	reason string,
) {
	if err := model.desk.Enter(symbol, path, tt, pathFound); err != nil {
		action, gate, reason := "blocked", "tier", err.Error()

		if denial, ok := err.(*broker.EntryDenial); ok {
			gate = denial.Gate
			reason = denial.Reason

			if denial.Gate == "size" {
				action = "skip"
			}
		}

		decide(&runtime.Decision{
			Symbol: symbol,
			Action: action,
			Reason: reason,
			Gate:   gate,
			Path:   path,
			Tier:   tt.String(),
		})
		return
	}

	decide(&runtime.Decision{
		Symbol: symbol,
		Action: "enter",
		Reason: reason,
		Path:   path,
		Tier:   tt.String(),
	})
}

func (model *Model) gateReason(
	rez map[string]float64,
	supported int,
	hasPrecision bool,
	precision, surprise float64,
) string {
	learn := system.Cfg.Learning

	if rez["calibrated"] < 1 {
		return "resonance not calibrated"
	}

	if supported < learn.MinSupportedHorizon {
		return "supported horizon below minimum"
	}

	if !hasPrecision {
		return "task-relative precision not ready"
	}

	if precision < learn.MinTaskRelativePrecision {
		return "task-relative precision below threshold"
	}

	if surprise < learn.MinSurprise {
		return "surprise below threshold"
	}

	return "resonance gate"
}

func (model *Model) bypassReading(learn *system.Learning) map[string]float64 {
	dims := len(resonance.Features)
	supported := max(learn.MinSupportedHorizon, 1)

	rez := map[string]float64{
		"calibrated":              1,
		"surprise":                1,
		"task_relative_precision": 1,
		"supported_horizon":       float64(supported),
	}

	for i := range dims {
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

func (model *Model) exitCut(
	envelope *runtime.Envelope,
	symbol string,
	pending *broker.Position,
) string {
	path := ""

	if envelope != nil {
		if cut := envelope.Paths[symbol]; cut != nil {
			path = cut.String()
		}
	}

	if path == "" && pending != nil {
		path = pending.ExitPath
	}

	if path == "" && pending != nil {
		if n := len(pending.HoldPaths); n > 0 {
			path = pending.HoldPaths[n-1]
		}
	}

	if pending != nil && (path == "" || path == pending.EntryPath) {
		return ""
	}

	return path
}

func (model *Model) Hold(position *broker.Position, path string) {
	if position == nil || path == "" || path == position.EntryPath {
		return
	}

	if len(position.HoldPaths) == 1 && position.HoldPaths[0] == path {
		return
	}

	position.HoldPaths = []string{path}
}

func (model *Model) Exit(position *broker.Position, path string) {
	if position == nil || path == "" || path == position.EntryPath {
		return
	}

	settled := position.ClosedAt != nil

	// Stoploss lots sit in pending until one distinct cut is attributed.
	// Re-training every tick was seeding a new EXIT leaf per live path and
	// walking resistance into the thousands.
	if settled && position.ExitPath != "" {
		model.dropPending(position)
		return
	}

	position.ExitPath = path
	tree := model.tree.Load()

	tree.Ensure(position.EntryPath, ActionEnter)
	tree.Ensure(position.ExitPath, ActionExit)

	delta := system.Cfg.Learning.ThresholdStep

	if position.PnL != nil && position.PnL.Sign() > 0 {
		delta = -delta
	}

	tree.Update(position.EntryPath, delta)
	tree.Update(position.ExitPath, delta)

	if settled {
		model.dropPending(position)
		return
	}

	if model.desk == nil {
		return
	}

	model.desk.Exit(model.bookTier(position), position)
}

func (model *Model) bookTier(position *broker.Position) broker.TierType {
	switch position.Book {
	case broker.TierTypePaper.String():
		return broker.TierTypePaper
	case broker.TierTypeReal.String():
		return broker.TierTypeReal
	default:
		return broker.TierTypeVirtual
	}
}

func (model *Model) dropPending(position *broker.Position) {
	if model.desk == nil || position == nil {
		return
	}

	model.desk.ClearPending(model.bookTier(position), position.Symbol)
}
