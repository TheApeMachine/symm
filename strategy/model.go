package strategy

import (
	"context"
	"math"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/theapemachine/nomagique/runtime"
	"github.com/theapemachine/nomagique/store"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/logic/resonance"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/ui"
)

/*
Model turns each tick's path cut into desk enter/hold/exit decisions and only
mutates the resistance tree on settled lots — fills and gates stay ahead of
learning so bad sends do not train the path book.
*/
type Model struct {
	*runtime.System
	desk      *broker.Desk
	price     *broker.Price
	store     *store.Client
	tree      *atomic.Pointer[Tree]
	resetPath func(string)
	live      bool
}

/*
NewModel swaps the path tree via atomic.Pointer so S3 reload and Step never
race on the same radix; resetPath clears grammar when tree actions consume a cut.
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
		live:      system.Cfg.Market.Model == "real",
	}

	tree := NewTree(ctx, "model/paths/", client)
	model.tree.Store(tree)

	return model
}

/*
Step drains stoploss pending exits first, then decides per symbol in parallel:
oracle/resonance/story gates, tree Predict, tier routing — decisions land on
the envelope for cmd and the hub without calling the broker on blocked paths.
*/
func (model *Model) Step(envelope *runtime.Envelope) *runtime.Envelope {
	if envelope == nil {
		return nil
	}

	// First we need to handle all pending exits, which are positions
	// that have exited via the Stoploss mechanism.
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
		// Live desk: stamp every outcome real. Untiered skips otherwise
		// land on the virtual tab and the REAL rail stays empty.
		if decision != nil && decision.Tier == "" && model.live {
			decision.Tier = broker.TierTypeReal.String()
		}

		mu.Lock()
		envelope.Decide(decision)
		mu.Unlock()
	}

	for symbol, cut := range envelope.Paths {
		if cut == nil {
			continue
		}

		wg.Go(func() {
			path := cut.String()
			var open []*broker.Position

			// Check if we currently have any open positions for this symbol.
			for _, tt := range []broker.TierType{
				broker.TierTypeVirtual, broker.TierTypePaper, broker.TierTypeReal,
			} {
				for position := range model.desk.Positions(tt, symbol) {
					open = append(open, position)
				}
			}

			learn := system.Cfg.Learning
			bypass := learn.ResonanceBypass

			// Live orders keep the entry gates; lab bypass is paper/virtual only.
			if model.live {
				bypass = false
			}

			// Oracle gate after pending exits: block flat symbols that are not
			// tradable. Open lots still run hold/exit.
			if len(open) == 0 && !bypass && !model.oracleAllows(envelope.Regions[symbol]) {
				decide(&runtime.Decision{
					Symbol: symbol,
					Action: "skip",
					Reason: "oracle veto lit — not tradable",
					Gate:   "oracle",
					Path:   path,
				})
				return
			}

			rez := envelope.Resonance[symbol]

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

			bullish := bypass || (enterNet >= minEnter && holdNet >= minEnter && exitNet <= holdNet && risk < minEnter)

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
		})
	}

	wg.Wait()

	if ui.UI != nil && ui.UI.Active() && len(envelope.Decisions) > 0 {
		if payload := envelope.Publish("decisions"); len(payload) > 0 {
			ui.UI.Write(payload)
		}
	}

	return envelope
}

/*
oracleAllows is true when at least one approve is lit and no lit veto is
selected. Grid.Observe emits exactly one side of each oracle pair; desk bool
vetoes are hard brightness 1. Idle metric ties (board / contagion / capacity
at brightness 0 while kernels are cold) stay out of the way — only a lit
veto fails closed.
*/
func (model *Model) oracleAllows(regions []*runtime.Region) bool {
	approved := false

	for _, region := range regions {
		if region == nil {
			continue
		}

		switch store.Catalog.ByKey(region.Key).Side {
		case "veto":
			if region.Brightness > 0 {
				return false
			}
		case "approve":
			if region.Brightness > 0 {
				approved = true
			}
		}
	}

	return approved
}

/*
applyAction handles explicit tree leaves (enter/hold/exit) — story bullishness
and min profit still gate so an exit leaf cannot flatten underwater lots or
enter against a bearish forward curve.
*/
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

  - real model: one live book; same gate path virtual used for samples
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
	if model.desk != nil && model.desk.AwaitingFill(symbol) {
		decide(&runtime.Decision{
			Symbol: symbol,
			Action: "skip",
			Reason: "awaiting entry fill",
			Gate:   "tier",
			Path:   path,
		})
		return
	}

	if model.live {
		model.tryEnter(decide, symbol, path, broker.TierTypeReal, pathFound, "real enter")
		return
	}

	floor := system.Cfg.Learning.PaperEligibleResistance

	if bypass || !pathFound || resistance > floor {
		model.tryEnter(decide, symbol, path, broker.TierTypeVirtual, false, "virtual sample")
		return
	}

	model.tryEnter(decide, symbol, path, broker.TierTypePaper, true, "paper enter")
	model.tryEnter(decide, symbol, path, broker.TierTypeReal, true, "real enter")
}

/*
tryEnter is the single desk.Enter choke point so blocked tiers emit structured
Decision rows (gate tier) instead of silent no-ops in the disruptor log.
*/
func (model *Model) tryEnter(
	decide func(*runtime.Decision),
	symbol, path string,
	tt broker.TierType,
	pathFound bool,
	reason string,
) {
	if err := model.desk.Enter(symbol, path, tt, pathFound); err != nil {
		decide(&runtime.Decision{
			Symbol: symbol,
			Action: "blocked",
			Reason: err.Error(),
			Gate:   "tier",
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

/*
gateReason builds human skip text for resonance vetoes — UI and replay need which
threshold failed without re-deriving curve math from raw envelope fields.
*/
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

/*
bypassReading fabricates a bullish calibrated resonance map for lab tiers only —
live model forces bypass off so paper shortcuts never reach REAL rails.
*/
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

/*
exitCut picks the path attributed to a stoploss settle: live cut if still lit,
else ExitPath/HoldPaths, and refuses EntryPath so pending exit does not train
on the opening grammar sentence.
*/
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

	if pending != nil && path == "" {
		return ""
	}

	if pending != nil && pending.ClosedAt == nil && path == pending.EntryPath {
		return ""
	}

	return path
}

/*
Hold records the latest non-entry path for outcome attribution — one hold path
per lot so tree training sees the path in force when stoploss later fires.
*/
func (model *Model) Hold(position *broker.Position, path string) {
	if position == nil || path == "" || path == position.EntryPath {
		return
	}

	if len(position.HoldPaths) == 1 && position.HoldPaths[0] == path {
		return
	}

	position.HoldPaths = []string{path}
}

/*
Exit arms broker sells before tree Update on pending lots — resistance moves
only after ClosedAt so rejects and repeated marks cannot train on ghost exits.
*/
func (model *Model) Exit(position *broker.Position, path string) {
	if position == nil {
		return
	}

	settled := position.ClosedAt != nil

	if path == "" || (!settled && path == position.EntryPath) {
		if settled {
			model.dropPending(position)
		}

		return
	}

	// Train only on realized fills. Policy exits arm the broker first; marking
	// the tree before the order fills poisoned resistance on rejects and
	// re-trained every tick while Closing.
	if !settled {
		if position.ExitPath != "" {
			return
		}

		position.ExitPath = path

		if model.desk == nil {
			return
		}

		model.desk.Exit(model.tierOf(position), position)
		return
	}

	if position.TreeApplied {
		model.dropPending(position)
		return
	}

	if position.ExitPath == "" {
		position.ExitPath = path
	}

	tree := model.tree.Load()

	tree.Ensure(position.EntryPath, ActionEnter)
	tree.Ensure(position.ExitPath, ActionExit)

	delta := model.resistanceDelta(position)

	tree.Update(position.EntryPath, delta)
	tree.Update(position.ExitPath, delta)
	position.TreeApplied = true
	model.dropPending(position)
}

/*
tierOf maps cache tier_key back to desk routes for Exit/ClearPending — wrong
tier would send paper sells down the real socket or drop pending too early.
*/
func (model *Model) tierOf(position *broker.Position) broker.TierType {
	switch position.TierKey {
	case broker.TierTypePaper.String():
		return broker.TierTypePaper
	case broker.TierTypeReal.String():
		return broker.TierTypeReal
	default:
		return broker.TierTypeVirtual
	}
}

/*
resistanceDelta is the leaf step for a settled (or settling) lot.

Losses raise resistance by the full ThresholdStep. Wins lower it by
step × timeWeight × pnlWeight, where short holds and larger fee-net PnL%
earn more credit, never more than one step per proof. Hold duration is
measured in seconds against a multi-hour half-life (lots live minutes–hours).
*/
func (model *Model) resistanceDelta(position *broker.Position) float64 {
	step := system.Cfg.Learning.ThresholdStep

	if position == nil || position.PnL == nil || position.PnL.Sign() <= 0 {
		return step
	}

	halfLife := system.Cfg.Learning.ResistanceHoldHalfLifeSec
	pnlRef := system.Cfg.Learning.ResistancePnLRefPct

	hold := model.holdDuration(position, halfLife).Seconds()
	timeWeight := 1.0 / (1.0 + hold/halfLife)

	pnlPct := 0.0

	if position.PnLPct != nil {
		pnlPct = position.PnLPct.InexactFloat64()
	}

	pnlWeight := math.Tanh(pnlPct / pnlRef)

	if pnlWeight < 0 {
		pnlWeight = 0
	}

	return -step * timeWeight * pnlWeight
}

/*
holdDuration clamps negative or missing clocks so resistance credit for quick
wins does not explode when OpenedAt/ClosedAt are partial on exchange imports.
*/
func (model *Model) holdDuration(position *broker.Position, halfLifeSec float64) time.Duration {
	if position.OpenedAt.IsZero() {
		return time.Duration(halfLifeSec * float64(time.Second))
	}

	end := time.Now().UTC()

	if position.ClosedAt != nil {
		end = position.ClosedAt.UTC()
	}

	hold := end.Sub(position.OpenedAt.UTC())

	if hold < 0 {
		return time.Duration(halfLifeSec * float64(time.Second))
	}

	return hold
}

/*
dropPending releases the desk pending slot after tree training consumed the lot —
until then strategy must still see the position for exit_path attribution.
*/
func (model *Model) dropPending(position *broker.Position) {
	if model.desk == nil || position == nil {
		return
	}

	model.desk.ClearPending(model.tierOf(position), position.Symbol)
}
