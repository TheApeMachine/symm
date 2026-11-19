package strategy

import (
	"context"
	"math"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/runtime"
	"github.com/theapemachine/nomagique/store"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/logic/resonance"
	"github.com/theapemachine/symm/strategy/deskgate"
	"github.com/theapemachine/symm/system"
)

/*
Model turns each tick's path cut into desk enter/hold/exit decisions and only
mutates the resistance tree on settled positions — fills and gates stay ahead of
learning so bad sends do not train the path book.
*/
type Model struct {
	*runtime.System
	desk      *broker.Desk
	price     *broker.Price
	store     *store.Client
	tree      *atomic.Pointer[Tree]
	board     *Board
	deskGate  *deskgate.Gate
	resetPath func(string)
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
	deskGate *deskgate.Gate,
) *Model {
	board := NewBoard(ctx)

	model := &Model{
		System:    runtime.NewSystem(ctx, "model"),
		desk:      desk,
		price:     price,
		store:     client,
		tree:      &atomic.Pointer[Tree]{},
		board:     board,
		deskGate:  deskGate,
		resetPath: resetPath,
	}

	tree := NewTree(ctx, "model/paths/", client)
	model.tree.Store(tree)

	if err := errnie.Require(map[string]any{
		"tree": tree,
	}); err != nil {
		errnie.Error(err)
	}

	return model
}

/*
Step drains stoploss pending exits first, then decides per symbol in parallel:
oracle/resonance/story gates, tree Predict, tier routing — decisions land on
the envelope for cmd and the hub without calling the broker on blocked paths.
*/
func (model *Model) Step(envelope *runtime.Envelope) *runtime.Envelope {
	if envelope == nil {
		errnie.Error(errnie.Err(
			errnie.NotFound,
			"[strategy.model.Step] envelope is nil",
			nil,
		))

		return nil
	}

	// First we need to handle all pending exits, which are positions
	// that have exited via the Stoploss mechanism.
	for _, tier := range []broker.TierType{
		broker.TierTypeVirtual, broker.TierTypePaper, broker.TierTypeReal,
	} {
		for position := range model.desk.Pending(tier) {
			// Settled rows train once ClosedAt exists — never on the send tick.
			if position.ClosedAt != nil {
				model.creditExit(position)
				model.desk.ClearPending(tier, position.Symbol)
				continue
			}

			if !model.desk.Has(position.Symbol) {
				model.desk.VetoPendingExit(position)
				continue
			}

			if !model.desk.ProfitableExit(position) {
				model.desk.VetoPendingExit(position)
				continue
			}

			errnie.Info("[strategy.model] pending stoploss exit",
				"tier", tier.String(),
				"symbol", position.Symbol,
				"exit_path", position.ExitPath,
			)

			// Do not SetExitPath before Exit — that overwrote Stoploss.TriggerReason
			// with "policy exit match". Attribution path stamps after arming.
			if err := model.desk.Exit(position, false); err != nil {
				if errnie.IsForbidden(err) {
					continue
				}

				errnie.Error(err)
				continue
			}

			if cut := envelope.Paths[position.Symbol]; cut != nil {
				position.SetExitPath(cut.String())
			}

			// Stoploss exits bypass Model.Exit — still clear open-phase tokens
			// so the next flat cycle does not inherit R06/R07 hold/exit crumbs.
			if model.resetPath != nil {
				model.resetPath(position.Symbol)
			}
			// Tier.Exit clears the pre-fill pending park before Send so a fast
			// fill's ClosedAt re-queue cannot be deleted by a late ClearPending.
		}
	}

	var wg sync.WaitGroup
	var candMu sync.Mutex
	candidates := make([]EnterCandidate, 0)

	for symbol, cut := range envelope.Paths {
		if cut == nil || cut.Len() < system.Cfg.Learning.MinPathLength {
			continue
		}

		wg.Go(func() {
			path := cut.String()
			open := model.desk.Has(symbol)
			resistance := 1.0

			// Oracle and resonance gates are entry filters. An open book still
			// has to hold or exit when the reading is cold or surprising.
			if !open && !model.oracleAllows(envelope.Regions[symbol]) {
				envelope.Decide(&runtime.Decision{
					Symbol: symbol,
					Action: "skip",
					Reason: model.oracleReason(envelope.Regions[symbol]),
					Gate:   "oracle",
					Path:   path,
					Tier:   model.decisionTier(symbol, resistance),
				})

				return
			}

			rez := envelope.Resonance[symbol]

			if rez == nil {
				envelope.Decide(&runtime.Decision{
					Symbol: symbol,
					Action: "skip",
					Reason: "no resonance reading",
					Gate:   "resonance",
					Path:   path,
					Tier:   model.decisionTier(symbol, resistance),
				})

				return
			}

			surprise := rez["surprise"]
			if regimeZ, ok := rez["surprise_regime_z"]; ok {
				surprise = regimeZ
			}
			supported := int(rez["supported_horizon"])
			precision, hasPrecision := rez["task_relative_precision"]

			learn := system.Cfg.Learning
			surpriseFloor := learn.MinSurprise
			if learn.MinEnterZ > 0 {
				surpriseFloor = learn.MinEnterZ
			}

			if rez["calibrated"] < 1 || supported < learn.MinSupportedHorizon || !hasPrecision || precision < learn.MinTaskRelativePrecision || surprise < surpriseFloor {
				envelope.Decide(&runtime.Decision{
					Symbol: symbol,
					Action: "skip",
					Reason: model.gateReason(rez, supported, hasPrecision, precision, surprise, surpriseFloor),
					Gate:   "resonance",
					Path:   path,
					Tier:   model.decisionTier(symbol, resistance),
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

			enterNet, holdNet, exitNet, ok := resonance.ForwardStoryNets(curve)

			if !ok {
				envelope.Decide(&runtime.Decision{
					Symbol: symbol,
					Action: "skip",
					Reason: "resonance curve incomplete",
					Gate:   "resonance",
					Path:   path,
					Tier:   model.decisionTier(symbol, resistance),
				})

				return
			}

			var bullish int

			switch {
			case enterNet >= holdNet && enterNet >= exitNet:
				bullish = 1
			case holdNet >= enterNet && holdNet >= exitNet:
				bullish = 0
			default:
				bullish = -1
			}

			if !open && bullish < 0 {
				envelope.Decide(&runtime.Decision{
					Symbol: symbol,
					Action: "skip",
					Reason: "forward story not bullish",
					Gate:   "story",
					Path:   path,
					Tier:   model.decisionTier(symbol, resistance),
				})

				return
			} else if open && bullish >= 0 {
				envelope.Decide(&runtime.Decision{
					Symbol: symbol,
					Action: "skip",
					Reason: "forward story bullish",
					Gate:   "story",
					Path:   path,
					Tier:   model.decisionTier(symbol, resistance),
				})
			}

			tree := model.tree.Load()
			action, resistance, matched := tree.Predict(*cut)

			// Open rows live on the tier they entered — never look up by the
			// current path's routeTier(resistance) (0 / paper-eligible misses virtual).
			openPos := model.openPosition(symbol)

			// Matched + ActionNone: leaf existed but brightness lost to resistance
			// (or the leaf was pruned). That is a veto, not an unexplored cold path.
			if matched {
				if openPos != nil {
					openPos.AppendHoldPath(path)
				}

				acted := false
				consumePath := false

				switch action {
				case ActionNone:
					if open && path != "" {
						envelope.Decide(&runtime.Decision{
							Symbol: symbol,
							Action: "hold",
							Reason: "path resistance veto",
							Gate:   "tree",
							Path:   path,
							Tier:   model.decisionTier(symbol, resistance),
						})
						acted = true
					}
				case ActionEnter:
					if !open {
						candMu.Lock()
						candidates = append(candidates, EnterCandidate{
							Symbol:     symbol,
							Path:       path,
							Resistance: resistance,
							PathFound:  true,
							Tier:       model.decisionTier(symbol, resistance),
							Decide: func(d *runtime.Decision) {
								envelope.Decide(d)
							},
						})
						candMu.Unlock()
						acted = true
						// Do not resetPath here — board.Place may skip the
						// candidate; reset only after a successful Enter.
					}
				case ActionHold:
					if open {
						envelope.Decide(&runtime.Decision{
							Symbol: symbol,
							Action: "hold",
							Reason: "path matched",
							Gate:   "tree",
							Path:   path,
							Tier:   model.decisionTier(symbol, resistance),
						})
						// Keep the sequencer phrase — resetting on every hold
						// forces 2+ regime flips before exit can even evaluate.
						acted = true
					}
				case ActionExit:
					if open && openPos != nil &&
						model.desk.ProfitableExit(openPos) &&
						model.Exit(openPos, path) {
						envelope.Decide(&runtime.Decision{
							Symbol: symbol,
							Action: "exit",
							Reason: "path matched",
							Gate:   "tree",
							Path:   path,
							Tier:   model.decisionTier(symbol, resistance),
						})
						acted = true
						consumePath = true
					}
				}

				if acted && consumePath && model.resetPath != nil {
					model.resetPath(symbol)
				}

				return
			}

			if !open {
				candMu.Lock()
				candidates = append(candidates, EnterCandidate{
					Symbol:     symbol,
					Path:       path,
					Resistance: resistance,
					PathFound:  false,
					Tier:       model.decisionTier(symbol, resistance),
					Decide: func(d *runtime.Decision) {
						envelope.Decide(d)
					},
				})
				candMu.Unlock()
				return
			}

			// Open book, unmatched path: seed hold/exit leaves from the story.
			acted := false
			consumePath := false

			switch {
			case bullish < 0:
				tree.Ensure(path, ActionExit)
				if openPos != nil &&
					model.desk.ProfitableExit(openPos) &&
					model.Exit(openPos, path) {
					envelope.Decide(&runtime.Decision{
						Symbol: symbol,
						Action: "exit",
						Reason: "story exit explore",
						Gate:   "story",
						Path:   path,
						Tier:   model.decisionTier(symbol, resistance),
					})
					acted = true
					consumePath = true
				}
			case bullish == 0:
				tree.Ensure(path, ActionHold)
				envelope.Decide(&runtime.Decision{
					Symbol: symbol,
					Action: "hold",
					Reason: "story hold explore",
					Gate:   "story",
					Path:   path,
					Tier:   model.decisionTier(symbol, resistance),
				})
				acted = true
			}

			if acted && consumePath && model.resetPath != nil {
				model.resetPath(symbol)
			}
		})
	}

	wg.Wait()

	live := system.Cfg.Market.Model == "real"
	model.board.Place(candidates, envelope, model.price, model.desk, live,
		func(cand EnterCandidate) {
			tier := model.routeTier(cand.Resistance)

			if !cand.PathFound {
				model.tree.Load().Ensure(cand.Path, ActionEnter)
			}

			hint := broker.SizeHint{
				Score:      cand.Score,
				Resistance: cand.Resistance,
				PathFound:  cand.PathFound,
				ShortHold:  cand.ShortHold,
			}

			if model.desk.Enter(cand.Symbol, cand.Path, tier, cand.PathFound, hint) != nil {
				return
			}

			if model.resetPath != nil {
				model.resetPath(cand.Symbol)
			}

			if position := model.desk.Position(tier, cand.Symbol); position != nil &&
				model.deskGate != nil {
				position.SetEntryDesk(model.deskGate.Features(
					model.price, cand.Symbol, envelope.Pumpdump[cand.Symbol],
				))
			}

			reason := map[bool]string{true: "path matched", false: "explore"}[cand.PathFound]
			if cand.ShortHold {
				reason += " (reserve)"
			}

			cand.Decide(&runtime.Decision{
				Symbol: cand.Symbol,
				Action: "enter",
				Reason: reason,
				Gate:   "tree",
				Path:   cand.Path,
				Tier:   tier.String(),
			})
		},
	)

	return envelope
}

func (model *Model) openTierKey(symbol string) string {
	if position := model.openPosition(symbol); position != nil {
		return position.TierKey
	}

	return ""
}

/*
openPosition finds the live open row for symbol on any desk tier. Exit/hold
must use this — routeTier(resistance) is only for new enters.
*/
func (model *Model) openPosition(symbol string) *broker.Position {
	if model == nil || model.desk == nil || symbol == "" {
		return nil
	}

	for _, tier := range []broker.TierType{
		broker.TierTypeReal, broker.TierTypePaper, broker.TierTypeVirtual,
	} {
		if position := model.desk.Position(tier, symbol); position != nil {
			return position
		}
	}

	return nil
}

func (model *Model) routeTier(resistance float64) broker.TierType {
	tier := broker.TierTypeVirtual

	if system.Cfg.Market.Model == "real" {
		tier = broker.TierTypeReal
	} else if resistance <= system.Cfg.Learning.PaperEligibleResistance {
		tier = broker.TierTypePaper
	}

	return tier
}

func (model *Model) decisionTier(symbol string, resistance float64) string {
	if key := model.openTierKey(symbol); key != "" {
		return key
	}

	return model.routeTier(resistance).String()
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
oracleReason names why oracleAllows failed — lit veto vs cold (no approve).
*/
func (model *Model) oracleReason(regions []*runtime.Region) string {
	for _, region := range regions {
		if region == nil || region.Brightness <= 0 {
			continue
		}

		row := store.Catalog.ByKey(region.Key)

		if row.Side == "veto" {
			if row.Phase != "" {
				return "oracle veto lit — " + row.Phase + " not tradable"
			}

			return "oracle veto lit — not tradable"
		}
	}

	return "oracle cold — no approve lit"
}

/*
gateReason builds human skip text for resonance vetoes — UI and replay need which
threshold failed without re-deriving curve math from raw envelope fields.
*/
func (model *Model) gateReason(
	rez map[string]float64,
	supported int,
	hasPrecision bool,
	precision, surprise, surpriseFloor float64,
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

	if surprise < surpriseFloor {
		return "surprise below threshold"
	}

	return "resonance gate"
}

/*
Exit arms broker sells and stamps ExitPath. Resistance / deskgate Learn run only
after ClosedAt (creditExit from pending) so rejects and tranche scale-outs cannot
train on ghost or partial exits. True when the exit was armed (caller may Decide).
*/
func (model *Model) Exit(position *broker.Position, path string) bool {
	if position == nil {
		return false
	}

	if position.Status() == runtime.BUSY || position.ClosedAt != nil {
		return false
	}

	if model.desk != nil {
		if !model.desk.ProfitableExit(position) {
			return false
		}

		errnie.Info("[strategy.model] policy exit",
			"symbol", position.Symbol,
			"tier", position.TierKey,
			"path", path,
			"pos_stage", position.Status().String(),
		)

		if err := model.desk.Exit(position, false); err != nil {
			if errnie.IsForbidden(err) {
				return false
			}

			errnie.Error(err)
			return false
		}
	}

	position.SetExitPath(path)

	if model.resetPath != nil {
		model.resetPath(position.Symbol)
	}

	return true
}

/*
creditExit trains path resistance and deskgate from a fully settled exit
(ClosedAt set). Shared by pending settle after flat fills — never on send.
EntryPath must Update here: Ensure alone leaves ActionEnter leaves at
ThresholdStart forever, so virtual→paper promotion and prune never move.
*/
func (model *Model) creditExit(position *broker.Position) {
	if position == nil || position.ClosedAt == nil {
		return
	}

	view, ok := position.TakeCreditView()
	if !ok {
		return
	}

	delta := model.resistanceDeltaView(view)
	tree := model.tree.Load()

	if tree != nil {
		if view.EntryPath != "" {
			tree.Ensure(view.EntryPath, ActionEnter)
			tree.Update(view.EntryPath, delta)
		}

		seen := make(map[string]struct{}, len(view.HoldPaths))
		for _, path := range view.HoldPaths {
			if path == "" || path == view.EntryPath || path == view.ExitPath {
				continue
			}
			if _, dup := seen[path]; dup {
				continue
			}
			seen[path] = struct{}{}
			tree.Ensure(path, ActionHold)
			tree.Update(path, delta)
		}

		if view.ExitPath != "" {
			tree.Ensure(view.ExitPath, ActionExit)
			tree.Update(view.ExitPath, delta)
		}
	}

	if model.deskGate != nil && len(view.EntryDesk) > 0 {
		developed := view.HasStop && view.PeakPct >= view.ArmPct
		if !developed && view.PnLPct != nil {
			developed = view.PnLPct.InexactFloat64() >=
				system.Cfg.Learning.PolicyExitMinProfitPct
		}
		model.deskGate.Learn(view.EntryDesk, developed)
	}
}

/*
resistanceDelta is the leaf step for a settled (or settling) position.

Losses raise resistance by the full ThresholdStep. Wins lower it by
step × timeWeight × pnlWeight, where short holds and larger fee-net PnL%
earn more credit, never more than one step per proof. Hold duration is
measured in seconds against a multi-hour half-life (positions live minutes–hours).
*/
func (model *Model) resistanceDelta(position *broker.Position) float64 {
	return model.resistanceDeltaView(position.SnapshotCreditView())
}

func (model *Model) resistanceDeltaView(view broker.CreditView) float64 {
	step := system.Cfg.Learning.ThresholdStep

	if view.PnL == nil || view.PnL.Sign() <= 0 {
		return step
	}

	halfLife := system.Cfg.Learning.ResistanceHoldHalfLifeSec
	pnlRef := system.Cfg.Learning.ResistancePnLRefPct

	hold := model.holdDurationView(view, halfLife).Seconds()
	timeWeight := 1.0 / (1.0 + hold/halfLife)

	pnlPct := 0.0
	if view.PnLPct != nil {
		pnlPct = view.PnLPct.InexactFloat64()
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
	return model.holdDurationView(position.SnapshotCreditView(), halfLifeSec)
}

func (model *Model) holdDurationView(view broker.CreditView, halfLifeSec float64) time.Duration {
	if view.OpenedAt.IsZero() {
		return time.Duration(halfLifeSec * float64(time.Second))
	}

	end := time.Now().UTC()
	if view.ClosedAt != nil {
		end = view.ClosedAt.UTC()
	}

	hold := end.Sub(view.OpenedAt.UTC())
	if hold < 0 {
		return time.Duration(halfLifeSec * float64(time.Second))
	}

	return hold
}
