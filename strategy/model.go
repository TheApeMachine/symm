package strategy

import (
	"context"
	"math"
	"strconv"
	"strings"
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
			if position.ClosedAt != nil {
				model.desk.ClearPending(tier, position.Symbol)
				continue
			}

			if !model.desk.Has(position.Symbol) {
				model.desk.VetoPendingExit(position)
				continue
			}

			if cut := envelope.Paths[position.Symbol]; cut != nil {
				position.ExitPath = cut.String()
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

			if err := model.desk.Exit(position, false); err != nil {
				if errnie.IsForbidden(err) {
					continue
				}

				errnie.Error(err)
				continue
			}

			model.desk.ClearPending(tier, position.Symbol)
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
			supported := int(rez["supported_horizon"])
			precision, hasPrecision := rez["task_relative_precision"]

			learn := system.Cfg.Learning

			if rez["calibrated"] < 1 || supported < learn.MinSupportedHorizon || !hasPrecision || precision < learn.MinTaskRelativePrecision || surprise < learn.MinSurprise {
				envelope.Decide(&runtime.Decision{
					Symbol: symbol,
					Action: "skip",
					Reason: model.gateReason(rez, supported, hasPrecision, precision, surprise),
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

			tier := model.routeTier(resistance)

			// Matched + ActionNone: leaf existed but brightness lost to resistance
			// (or the leaf was pruned). That is a veto, not an unexplored cold path.
			if matched {
				position := model.desk.Position(tier, symbol)

				if position != nil {
					position.HoldPaths = append(position.HoldPaths, path)
				}

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
					}
				case ActionEnter:
					if !open {
						candMu.Lock()
						candidates = append(candidates, EnterCandidate{
							Symbol:     symbol,
							Path:       path,
							Resistance: resistance,
							PathFound:  true,
							Decide: func(d *runtime.Decision) {
								envelope.Decide(d)
							},
						})
						candMu.Unlock()
					}
				case ActionExit:
					if open {
						position := model.desk.Position(tier, symbol)

						if position != nil && model.desk.ProfitableExit(position) {
							model.Exit(position, path)
						}
					}

					envelope.Decide(&runtime.Decision{
						Symbol: symbol,
						Action: "exit",
						Reason: "path matched",
						Gate:   "tree",
						Path:   path,
						Tier:   model.decisionTier(symbol, resistance),
					})
				}

				if model.resetPath != nil {
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
					Decide: func(d *runtime.Decision) {
						envelope.Decide(d)
					},
				})
				candMu.Unlock()
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

			if model.desk.Enter(cand.Symbol, cand.Path, tier, cand.PathFound) != nil {
				return
			}

			if model.resetPath != nil {
				model.resetPath(cand.Symbol)
			}

			if position := model.desk.Position(tier, cand.Symbol); position != nil &&
				model.deskGate != nil {
				position.EntryDesk = model.deskGate.Features(
					model.price, cand.Symbol, envelope.Pumpdump[cand.Symbol],
				)
			}

			cand.Decide(&runtime.Decision{
				Symbol: cand.Symbol,
				Action: "enter",
				Reason: map[bool]string{true: "path matched", false: "explore"}[cand.PathFound],
				Gate:   "tree",
				Path:   cand.Path,
				Tier:   tier.String(),
			})
		},
	)

	return envelope
}

func (model *Model) openTierKey(symbol string) string {
	for _, tier := range []broker.TierType{
		broker.TierTypeReal, broker.TierTypePaper, broker.TierTypeVirtual,
	} {
		if model.desk.Position(tier, symbol) != nil {
			return tier.String()
		}
	}

	return ""
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
Exit arms broker sells before tree Update on pending positions — resistance moves
only after ClosedAt so rejects and repeated marks cannot train on ghost exits.
*/
func (model *Model) Exit(position *broker.Position, path string) {
	if position == nil {
		return
	}

	if position.Status() == runtime.BUSY || position.ClosedAt != nil {
		return
	}

	if model.desk != nil {
		if !model.desk.ProfitableExit(position) {
			return
		}

		errnie.Info("[strategy.model] policy exit",
			"symbol", position.Symbol,
			"tier", position.TierKey,
			"path", path,
			"pos_stage", position.Status().String(),
		)

		if err := model.desk.Exit(position, false); err != nil {
			if errnie.IsForbidden(err) {
				return
			}

			errnie.Error(err)
			return
		}
	}

	position.ExitPath = path

	if position.ExitPath == "" && len(position.HoldPaths) > 1 {
		position.ExitPath = strings.Join(
			position.HoldPaths[len(position.HoldPaths)-2:], "/",
		)

		return
	}

	if position.ExitPath == "" {
		return
	}

	model.tree.Load().Update(
		position.ExitPath, model.resistanceDelta(position),
	)

	if model.deskGate != nil && len(position.EntryDesk) > 0 {
		model.deskGate.Learn(
			position.EntryDesk,
			model.deskGate.Developed(position),
		)
	}

	if model.resetPath != nil {
		model.resetPath(position.Symbol)
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
