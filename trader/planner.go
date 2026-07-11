package trader

import (
	"math"
	"sort"
	"sync"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/structure"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/algorithm"
	pmanifold "github.com/theapemachine/nomagique/physics/manifold"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/logic"
	field "github.com/theapemachine/symm/logic/manifold"
	"github.com/theapemachine/symm/strategy"
	"github.com/theapemachine/symm/types"
)

type stopConfig struct {
	TrailingOffsetBps        int
	MinOffsetBps             int
	MaxOffsetBps             int
	MomentumDecayFraction    float64
	StagnationMaxTouches     int
	StagnationZoneFraction   float64
	TakeProfitArmPct         float64
	TakeProfitTightOffsetBps int
	TakeProfitCapPct         float64
	BreakoutThresholdPct     float64
	BreakoutHoldProbability  float64
}

func loadStopConfig() stopConfig {
	v := viper.GetViper()
	return stopConfig{
		TrailingOffsetBps:        v.GetInt("trading.stop.trailing_offset_bps"),
		MinOffsetBps:             v.GetInt("trading.stop.min_offset_bps"),
		MaxOffsetBps:             v.GetInt("trading.stop.max_offset_bps"),
		MomentumDecayFraction:    v.GetFloat64("trading.stop.momentum_decay_fraction"),
		StagnationMaxTouches:     v.GetInt("trading.stop.stagnation_max_touches"),
		StagnationZoneFraction:   v.GetFloat64("trading.stop.stagnation_zone_fraction"),
		TakeProfitArmPct:         v.GetFloat64("trading.stop.take_profit_arm_pct"),
		TakeProfitTightOffsetBps: v.GetInt("trading.stop.take_profit_tight_offset_bps"),
		TakeProfitCapPct:         v.GetFloat64("trading.stop.take_profit_cap_pct"),
		BreakoutThresholdPct:     v.GetFloat64("trading.stop.breakout_threshold_pct"),
		BreakoutHoldProbability:  v.GetFloat64("trading.stop.breakout_hold_probability"),
	}
}

type PositionTracker struct {
	Symbol           string
	PeakReturn       float64
	PeakMomentum     float64
	PeakTouchCount   int
	InStagnationZone bool
}

func expSafe(x float64) float64 {
	if x > 100.0 {
		return math.MaxFloat64
	}
	if x < -100.0 {
		return 0.0
	}
	return math.Exp(x)
}

// GetMomentum calculates field momentum (manifold drive + resonance energy)
func (tracker *PositionTracker) GetMomentum(thesis *strategy.Thesis) float64 {
	if thesis == nil {
		return 0
	}

	snapshot, _ := thesis.Evidence("manifold")
	manifoldReading, hasManifold := field.ReadingFromEvidence(snapshot)
	resonanceOutcome, hasResonance := Evidence[logic.ResonanceOutcome](thesis, "resonance")

	if hasManifold && hasResonance {
		return manifoldReading.GuidanceSpeed + resonanceOutcome.Energy
	}
	return 0
}

func (tracker *PositionTracker) Update(
	pos broker.PositionData,
	thesis *strategy.Thesis,
	stopConfig stopConfig,
) (bool, string) {
	returnPct := pos.ReturnPct

	// Update Peak Return
	if returnPct > tracker.PeakReturn {
		tracker.PeakReturn = returnPct
		tracker.PeakTouchCount = 0
		tracker.InStagnationZone = false
	}

	// Dynamic Trailing Stop calculation
	offsetBps := stopConfig.TrailingOffsetBps
	if stopConfig.TakeProfitArmPct > 0 && tracker.PeakReturn >= stopConfig.TakeProfitArmPct {
		offsetBps = stopConfig.TakeProfitTightOffsetBps
	}

	// Expand trailing stop when bid-ask spread widens, bounded by max_offset
	spreadBps := pos.Spread.Float64() * 10000.0
	if spreadBps > 0 {
		offsetBps = int(math.Min(float64(stopConfig.MaxOffsetBps), math.Max(float64(stopConfig.MinOffsetBps), float64(offsetBps)+spreadBps)))
	}

	stopReturn := tracker.PeakReturn - (float64(offsetBps) / 10000.0)

	// 1. Trailing Stop trigger
	if returnPct < stopReturn {
		return true, "trailing_stop"
	}

	// 2. Take Profit Cap
	if stopConfig.TakeProfitCapPct > 0 && returnPct >= stopConfig.TakeProfitCapPct {
		return true, "take_profit_cap"
	}

	// 3. Stagnation touch-counting
	if stopConfig.StagnationMaxTouches > 0 && tracker.PeakReturn > 0 {
		zoneBoundary := tracker.PeakReturn * (1.0 - stopConfig.StagnationZoneFraction)
		if returnPct >= zoneBoundary && returnPct < tracker.PeakReturn {
			if !tracker.InStagnationZone {
				tracker.InStagnationZone = true
				tracker.PeakTouchCount++
			}
		} else if returnPct < zoneBoundary {
			tracker.InStagnationZone = false
		}

		if tracker.PeakTouchCount >= stopConfig.StagnationMaxTouches {
			return true, "stagnation"
		}
	}

	// 4. Momentum Decay (manifold drive + resonance energy)
	if stopConfig.MomentumDecayFraction > 0 && thesis != nil {
		momentum := tracker.GetMomentum(thesis)
		if momentum > tracker.PeakMomentum {
			tracker.PeakMomentum = momentum
		}
		if tracker.PeakMomentum > 0 && returnPct > 0 {
			if momentum < tracker.PeakMomentum*stopConfig.MomentumDecayFraction {
				return true, "momentum_decay"
			}
		}
	}

	// 5. Breakout predictive next-tick gate
	if stopConfig.BreakoutThresholdPct > 0 && returnPct >= stopConfig.BreakoutThresholdPct && thesis != nil {
		snapshot, hasSnapshot := thesis.Evidence("manifold")
		manifoldReading, hasManifold := field.ReadingFromEvidence(snapshot)
		resonanceOutcome, hasResonance := Evidence[logic.ResonanceOutcome](thesis, "resonance")
		causalOutput, hasCausal := Evidence[algorithm.PearlOutput](thesis, "causal")

		if hasSnapshot && hasManifold && hasResonance {
			pForecast := 1.0 / (1.0 + expSafe(-resonanceOutcome.ReturnForecast*100.0))
			pMomentum := 0.5
			if manifoldReading.GuidanceSpeed > 0 {
				pMomentum = 1.0 / (1.0 + expSafe(-manifoldReading.GuidanceSpeed))
			}
			pCausal := 0.5
			if hasCausal {
				pCausal = 1.0 / (1.0 + expSafe(-causalOutput.Association))
			}

			pUp := (pForecast + pMomentum + pCausal) / 3.0
			if pUp < stopConfig.BreakoutHoldProbability {
				return true, "breakout_edge_faded"
			}
		}
	}

	return false, ""
}

/*
Planner consumes the Thesis produced by the logic.Analyzer and builds a
strategy.Graph whose nodes encode a category's conviction (score) and whose
edges propagate that conviction through the decision hierarchy defined in
DECISION.md. Walk() converges on the ForecastEdge node whose signed magnitude
selects the action with the highest expected utility.

The decision tree has five layers:

 1. Systemic  — Sentiment + Correlation  (is the world helping?)
 2. Origin    — Causal (Pearl)            (who is driving this?)
 3. Quality   — Toxicity + DepthFlow + CVD (is the liquidity sincere?)
 4. Timing    — Fluid + Hawkes            (is the engine running hot?)
 5. Forecast  — ForecastEdge              (the signed utility score)

Each lower-layer category propagates upward via edges whose weight is derived
from the evidence confidence/strength, so a high-conviction "Turbulent" reading
dampens the final ForecastEdge while a high-conviction "Frenzy" amplifies it.
*/
type Planner struct {
	desk     *broker.Desk
	price    *broker.Price
	analyzer *logic.Analyzer
	theses   *sync.Map
	trackers *sync.Map
	uiHub    chan []byte
}

/*
NewPlanner instantiates a Planner with a desk for capacity queries, a price
stream for live mid-price snapshots, and the analyzer whose Level3-derived
theses it consumes on every Update. The thesis ring buffer is initialised
lazily on the first Update call.
*/
func NewPlanner(
	desk *broker.Desk, price *broker.Price, analyzer *logic.Analyzer, uiHub chan []byte,
) *Planner {
	return &Planner{
		desk:     desk,
		price:    price,
		analyzer: analyzer,
		theses:   &sync.Map{},
		trackers: &sync.Map{},
		uiHub:    uiHub,
	}
}

/*
Update drains the analyzer's theses produced since the last call, stores
each in a per-symbol ring of depth 8 (keeping the most recent 8
snapshots), then evaluates the latest thesis for every symbol and
returns the set of Intents whose signed ForecastEdge utility clears the
entry threshold.

No symbol having a pending thesis is not an error; stop and exit
evaluation for currently held positions still runs unconditionally.
*/
func (planner *Planner) Update() ([]strategy.Intent, error) {
	theses := planner.analyzer.PendingTheses()

	// 1. Store each new thesis in its per-symbol ring.
	if len(theses) > 0 {
		for symbol, thesis := range theses {
			found, _ := planner.theses.LoadOrStore(
				symbol,
				structure.NewListRing[*strategy.Thesis](8),
			)

			ring := found.(*structure.ListRing[*strategy.Thesis])
			ring.Push(thesis.Clone()) // Push a cloned snapshot, not an alias
		}
	}

	// 2. Evaluate stateful stops and exits on currently held positions
	intents := make([]strategy.Intent, 0, len(theses))
	positions := planner.desk.Positions()
	holdings := make(map[string]*broker.Position, len(positions))

	for _, position := range positions {
		if position == nil || position.Data == nil {
			continue
		}

		holdings[position.Data.Symbol] = position
	}

	stopConfig := loadStopConfig()

	for symbol, position := range holdings {
		rawTracker, _ := planner.trackers.LoadOrStore(symbol, &PositionTracker{
			Symbol: position.Data.Symbol,
		})
		tracker := rawTracker.(*PositionTracker)

		var thesis *strategy.Thesis

		if ringVal, ok := planner.theses.Load(symbol); ok {
			ring := ringVal.(*structure.ListRing[*strategy.Thesis])
			ring.Do(func(t *strategy.Thesis) {
				if t != nil {
					thesis = t
				}
			})
		}

		if thesis != nil {
			stepAt, hasStep := thesis.Evidence("step_at")

			if hasStep {
				at := stepAt.(time.Time)

				if time.Since(at) > 500*time.Millisecond {
					thesis = nil // Thesis is too stale to drive momentum-based exits
				}
			}
		}

		shouldExit, reason := tracker.Update(*position.Data, thesis, stopConfig)

		if shouldExit {
			err := planner.desk.Sell(position.Data.Symbol)

			if err == nil {
				intents = append(intents, strategy.Intent{
					Symbol:     position.Data.Symbol,
					Action:     strategy.ActionSell,
					Size:       *decimal.NewFromFloat64(1.0),
					Edge:       position.Data.PnL,
					Confidence: 1.0,
					Thesis:     thesis,
				})
				errnie.Info("planner: triggered sell exit for " + position.Data.Symbol + " due to " + reason)
			} else {
				errnie.Error(err)
			}
		}
	}

	// 3. Reap trackers for positions we no longer hold
	planner.trackers.Range(func(key, value any) bool {
		symbol := key.(string)
		if _, active := holdings[symbol]; !active {
			planner.trackers.Delete(symbol)
		}
		return true
	})

	// 4. Evaluate entries for symbols in the active theses
	edgeMinBps := viper.GetViper().GetFloat64("trading.edge_min_bps")
	edgeThreshold := edgeMinBps / 10000.0

	var candidates []strategy.Intent

	// Only evaluate entries for symbols that just received a new thesis update
	for symbol := range theses {
		// Skip buy check if we already hold this position
		if _, active := holdings[symbol]; active {
			continue
		}

		if ringVal, ok := planner.theses.Load(symbol); ok {
			ring := ringVal.(*structure.ListRing[*strategy.Thesis])

			intent, err := planner.evaluate(symbol, ring, edgeThreshold)

			if err != nil {
				errnie.Error(err)
				continue
			}

			if intent != nil {
				candidates = append(candidates, *intent)
			}
		}
	}

	// 5. Rank and execute entries
	if len(candidates) > 0 {
		// Sort candidates by edge (descending)
		sort.Slice(candidates, func(i, j int) bool {
			return candidates[i].Edge.Rat().Cmp(candidates[j].Edge.Rat()) > 0
		})

		maxPositions := viper.GetViper().GetInt("trading.slots.normal")
		for _, intent := range candidates {
			openPositions := planner.desk.OpenPositions()
			deskStatus := planner.desk.Status()

			if deskStatus == types.BUSY {
				break // Cannot take more positions
			}

			opportunity := openPositions >= maxPositions
			edgeFloat := intent.Edge.Float64()
			if opportunity && edgeFloat < 2*edgeThreshold {
				continue // Must clear 2x threshold if in reserved slots
			}

			sizeFloat := intent.Size.Float64()

			entryPrice, err := planner.price.Get(intent.Symbol)

			if err != nil {
				errnie.Error(errnie.Err(
					errnie.Internal,
					"failed to get entry price: "+err.Error(),
					err,
				))

				continue
			}

			if entryPrice.Last == nil || entryPrice.Last.Sign() <= 0 {
				continue
			}

			err = planner.desk.Buy(intent.Symbol, sizeFloat, *entryPrice.Last, opportunity)
			if err == nil {
				intents = append(intents, intent)
			} else {
				errnie.Error(err)
			}
		}
	}

	planner.Publish(intents)

	return intents, nil
}

/*
evaluate extracts the latest thesis from the ring, builds a CategoryGraph from
its stored evidence, walks the graph to derive a ForecastEdge utility score,
and returns an Intent if the score clears the entry threshold.

A thesis is skipped (returns nil) if:
  - the ring is empty
  - the ForecastEdge score is NaN, Inf, or zero
  - the symbol has no price evidence
*/
func (planner *Planner) evaluate(
	symbol string,
	ring *structure.ListRing[*strategy.Thesis],
	edgeThreshold float64,
) (*strategy.Intent, error) {
	if ring.Len() == 0 {
		return nil, errnie.Error(errnie.Err(
			errnie.Internal,
			"ring is empty",
			nil,
		))
	}

	var latest *strategy.Thesis

	ring.Do(func(thesis *strategy.Thesis) {
		if thesis != nil {
			latest = thesis
		}
	})

	if latest == nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Internal,
			"latest thesis is nil",
			nil,
		))
	}

	graph := planner.buildGraph(latest)

	if graph == nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Internal,
			"graph is nil",
			nil,
		))
	}

	utility := graph.Walk()

	if utility <= 0 || math.IsNaN(utility) || math.IsInf(utility, 0) {
		return nil, errnie.Error(errnie.Err(
			errnie.Internal,
			"utility is zero, NaN, or Inf",
			nil,
		))
	}

	var resonance logic.ResonanceOutcome

	if res, ok := Evidence[logic.ResonanceOutcome](latest, "resonance"); ok {
		resonance = res
	}

	maturityVal := 1.0

	if matEv, ok := latest.Evidence("maturity"); ok {
		if m, ok := matEv.(float64); ok && m > 0 {
			maturityVal = m
		}
	}

	// Calculate net expected yield using utility as a conviction scaler on the forecast,
	// discounted by signal maturity to enforce confidence threshold.
	expectedYield := resonance.ReturnForecast * utility * maturityVal

	if expectedYield <= 0 || math.IsNaN(expectedYield) || math.IsInf(expectedYield, 0) {
		return nil, errnie.Error(errnie.Err(
			errnie.Internal,
			"expected yield is zero, NaN, or Inf",
			nil,
		))
	}

	netYield, err := planner.price.WithFriction(symbol, expectedYield)

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Internal,
			"failed to get friction: "+err.Error(),
			err,
		))
	}

	minimumEdge := decimal.NewFromFloat64(edgeThreshold)

	if netYield.Cmp(minimumEdge) < 0 {
		return nil, errnie.Error(errnie.Err(
			errnie.Internal,
			"net yield is less than minimum edge",
			nil,
		))
	}

	// Fixed Fractional Risk Sizing:
	//   position_fraction = risk_pct% / stop_distance%
	//
	// Example: 1.5% risk / 1% stop = 0.15 (15% of wallet)
	//          But capped by base_fraction (budget ceiling).
	stopConfig := loadStopConfig()

	trailingStopRatio := decimal.NewFromInt64(
		int64(stopConfig.TrailingOffsetBps),
	).Div(decimal.NewFromInt64(10000))

	if trailingStopRatio.Sign() <= 0 {
		return nil, errnie.Error(errnie.Err(
			errnie.Internal,
			"trailing stop ratio is zero or negative",
			nil,
		))
	}

	riskPct, err := decimal.NewFromString(
		viper.GetViper().GetString("trading.sizing.risk_pct"),
	)

	if err != nil || riskPct.Sign() <= 0 {
		return nil, errnie.Error(errnie.Err(
			errnie.Internal,
			"risk percentage is zero or negative",
			nil,
		))
	}

	riskRatio := riskPct.Div(decimal.NewFromInt64(100))
	calculatedFraction := riskRatio.Div(trailingStopRatio)
	baseFraction, err := decimal.NewFromString(
		viper.GetViper().GetString("trading.sizing.base_fraction"),
	)

	if err != nil || baseFraction.Sign() <= 0 {
		return nil, errnie.Error(errnie.Err(
			errnie.Internal,
			"base fraction is zero or negative",
			nil,
		))
	}

	positionFraction := calculatedFraction

	if baseFraction.Cmp(calculatedFraction) < 0 {
		positionFraction = baseFraction
	}

	intent := &strategy.Intent{
		Symbol:     symbol,
		Action:     strategy.ActionBuy,
		Size:       *positionFraction,
		Edge:       *netYield,
		Confidence: utility,
		Thesis:     latest,
	}
	intent.Velocity = resonance.ReturnForecast

	return intent, nil
}

/*
buildGraph extracts evidence from the thesis and constructs a strategy.Graph
whose CategoryNode scores encode the conviction of each detected category and
whose CategoryEdge weights propagate conviction through the decision hierarchy.

The graph structure mirrors the layered decision tree defined in DECISION.md:

	Layer 1 (Systemic)      → Sentiment, Correlation
	Layer 2 (Origin)        → Causal (Pearl)
	Layer 3 (Quality)       → Toxicity, DepthFlow, CVD
	Layer 4 (Timing)        → Fluid, Hawkes
	Layer 5 (Forecast)      → ForecastEdge (single node, the signed utility)

Edges flow upward: Layer 1 → Layer 2 → Layer 3 → Layer 4 → Layer 5.

A nil thesis or one with no meaningful evidence returns nil.
*/
func (planner *Planner) buildGraph(thesis *strategy.Thesis) *strategy.Graph {
	if thesis == nil {
		return nil
	}

	nodes := make([]strategy.CategoryNode, 0, len(types.CategoryOrder))
	edges := make([]strategy.CategoryEdge, 0, len(types.CategoryOrder)*2)

	// Appending is strictly bottom-up to guarantee topological order alignment in single-pass Walks:
	// Layer 1 (Systemic) -> Layer 2 (Origin Target Linkages)
	if err := planner.addCausalNodes(thesis, &nodes, &edges); err != nil {
		errnie.Error(err)
	}

	// Outgoing linkages connecting Causal Origin Targets to Forecast layer
	edges = append(edges, strategy.CategoryEdge{
		From:   types.EndogenousAlpha,
		To:     types.ForecastEdge,
		Weight: 0.5,
	}, strategy.CategoryEdge{
		From:   types.SystemicBeta,
		To:     types.ForecastEdge,
		Weight: 0.3,
	}, strategy.CategoryEdge{
		From:   types.CausalNoise,
		To:     types.ForecastEdge,
		Weight: -0.4,
	}, strategy.CategoryEdge{
		From:   types.LiquidityShock,
		To:     types.ForecastEdge,
		Weight: -0.6,
	})

	// Layer 3-4 (Quality, Timing) -> Layer 5
	planner.addTimingNodes(thesis, &nodes, &edges)

	if len(nodes) == 0 {
		return nil
	}

	// Layer 5 Sink Node
	nodes = append(nodes, strategy.CategoryNode{
		Category: types.ForecastEdge,
		Score:    0,
	})

	return strategy.NewGraph(nodes, edges)
}

func (planner *Planner) addCausalNodes(
	thesis *strategy.Thesis,
	nodes *[]strategy.CategoryNode,
	edges *[]strategy.CategoryEdge,
) error {
	output, ok := Evidence[algorithm.PearlOutput](thesis, "causal")
	if !ok {
		return nil
	}

	catIndex := int(output.Category)
	if catIndex <= 0 || catIndex > len(types.CategoryOrder) {
		return nil
	}

	originCategory := types.CategoryByIndex(catIndex)
	originScore := output.Strength * output.Confidence

	if math.IsNaN(originScore) || math.IsInf(originScore, 0) {
		return nil
	}

	*nodes = append(*nodes, strategy.CategoryNode{
		Category: originCategory,
		Score:    originScore,
	})

	// Propagate Causal target up to Inertial/Timing layer
	if output.Confidence > 0 {
		*edges = append(*edges, strategy.CategoryEdge{
			From:   originCategory,
			To:     types.Inertial,
			Weight: 0.6,
		})
	}

	if output.Distribution != nil {
		planner.addOriginFromDistribution(output, nodes, edges)
	}

	return nil
}

func (planner *Planner) addOriginFromDistribution(
	output algorithm.PearlOutput,
	nodes *[]strategy.CategoryNode,
	edges *[]strategy.CategoryEdge,
) {
	// PearlOutput.Distribution from the causal ladder contains the keys:
	// "association", "intervention", "counterfactual", "residual"
	// We map these to the Origin (Layer 2) categories as defined in DECISION.md
	causalMap := map[string]types.CategoryType{
		"counterfactual": types.EndogenousAlpha,
		"association":    types.SystemicBeta,
		"intervention":   types.LiquidityShock,
		"residual":       types.CausalNoise,
	}

	for key, targetCategory := range causalMap {
		prob, ok := output.Distribution[key]
		if !ok || math.IsNaN(prob) || math.IsInf(prob, 0) || prob <= 0 {
			continue
		}

		*nodes = append(*nodes, strategy.CategoryNode{
			Category: targetCategory,
			Score:    prob * output.Confidence,
		})

		// Note: The linkages from EndogenousAlpha, SystemicBeta, etc. to ForecastEdge
		// are appended separately in buildGraph.
	}
}

func (planner *Planner) addTimingNodes(
	thesis *strategy.Thesis,
	nodes *[]strategy.CategoryNode,
	edges *[]strategy.CategoryEdge,
) {
	snapshot, ok := thesis.Evidence("manifold")
	if !ok {
		return
	}

	reading, ok := field.ReadingFromEvidence(snapshot)
	if !ok {
		return
	}

	laminarScore := scoreLaminar(reading)
	turbulentScore := scoreTurbulent(reading)
	inertialScore := scoreInertial(reading)
	viscousScore := scoreViscous(reading)

	appendNodeIfPositive(nodes, types.Laminar, laminarScore)
	appendNodeIfPositive(nodes, types.Turbulent, turbulentScore)
	appendNodeIfPositive(nodes, types.Inertial, inertialScore)
	appendNodeIfPositive(nodes, types.Viscous, viscousScore)

	if laminarScore > 0 {
		*edges = append(*edges, strategy.CategoryEdge{
			From:   types.Laminar,
			To:     types.ForecastEdge,
			Weight: 0.6,
		})
	}

	if inertialScore > 0 {
		*edges = append(*edges, strategy.CategoryEdge{
			From:   types.Inertial,
			To:     types.ForecastEdge,
			Weight: 0.5,
		})
	}

	if turbulentScore > 0 {
		*edges = append(*edges, strategy.CategoryEdge{
			From:   types.Turbulent,
			To:     types.ForecastEdge,
			Weight: -0.8,
		})
	}

	if viscousScore > 0 {
		*edges = append(*edges, strategy.CategoryEdge{
			From:   types.Viscous,
			To:     types.ForecastEdge,
			Weight: -0.4,
		})
	}

	resonance, ok := Evidence[logic.ResonanceOutcome](thesis, "resonance")
	if !ok || !resonance.IsFinite() {
		return
	}

	frenzyScore := scoreFrenzy(resonance)
	saturationScore := scoreSaturation(resonance)
	organicScore := scoreOrganic(resonance)
	exhaustionScore := scoreExhaustion(resonance)

	appendNodeIfPositive(nodes, types.Frenzy, frenzyScore)
	appendNodeIfPositive(nodes, types.Saturation, saturationScore)
	appendNodeIfPositive(nodes, types.Organic, organicScore)
	appendNodeIfPositive(nodes, types.Exhaustion, exhaustionScore)

	if frenzyScore > 0 {
		*edges = append(*edges, strategy.CategoryEdge{
			From:   types.Frenzy,
			To:     types.ForecastEdge,
			Weight: 0.7,
		})
	}

	if saturationScore > 0 {
		*edges = append(*edges, strategy.CategoryEdge{
			From:   types.Saturation,
			To:     types.ForecastEdge,
			Weight: -0.9,
		})
	}

	if organicScore > 0 {
		*edges = append(*edges, strategy.CategoryEdge{
			From:   types.Organic,
			To:     types.ForecastEdge,
			Weight: 0.3,
		})
	}

	if exhaustionScore > 0 {
		*edges = append(*edges, strategy.CategoryEdge{
			From:   types.Exhaustion,
			To:     types.ForecastEdge,
			Weight: -0.5,
		})
	}

	// Add predictive return forecast as a direct sink input to ForecastEdge
	forecast := resonance.ReturnForecast
	if !math.IsNaN(forecast) && !math.IsInf(forecast, 0) && forecast != 0 {
		*nodes = append(*nodes, strategy.CategoryNode{
			Category: types.ForecastEdge,
			Score:    forecast * 2.0,
		})
	}
}

func (planner *Planner) selectAction(
	utility float64,
) strategy.Action {
	if utility > 0 {
		return strategy.ActionBuy
	}

	if utility < 0 {
		return strategy.ActionSell
	}

	return strategy.ActionHold
}

func (planner *Planner) Stops() map[string]any {
	stops := make(map[string]any)
	stopConfig := loadStopConfig()
	positions := planner.desk.Positions()
	holdings := make(map[string]*broker.Position, len(positions))

	for _, position := range positions {
		if position == nil || position.Data == nil {
			continue
		}

		holdings[position.Data.Symbol] = position
	}

	planner.trackers.Range(func(key, value any) bool {
		symbol := key.(string)
		tracker := value.(*PositionTracker)
		position, active := holdings[symbol]

		if !active {
			return true
		}

		entryVal := position.Data.EntryPrice.Float64()
		if entryVal <= 0 {
			return true
		}

		offsetBps := stopConfig.TrailingOffsetBps
		if stopConfig.TakeProfitArmPct > 0 && tracker.PeakReturn >= stopConfig.TakeProfitArmPct {
			offsetBps = stopConfig.TakeProfitTightOffsetBps
		}

		spreadBps := position.Data.Spread.Float64() * 10000.0
		if spreadBps > 0 {
			offsetBps = int(math.Min(float64(stopConfig.MaxOffsetBps), math.Max(float64(stopConfig.MinOffsetBps), float64(offsetBps)+spreadBps)))
		}

		stopReturn := tracker.PeakReturn - (float64(offsetBps) / 10000.0)
		stopPrice := entryVal * (1.0 + stopReturn)

		var thesis *strategy.Thesis
		if found, ok := planner.theses.Load(symbol); ok {
			ring := found.(*structure.ListRing[*strategy.Thesis])
			ring.Do(func(t *strategy.Thesis) {
				if t != nil {
					thesis = t
				}
			})
		}

		momentum := tracker.GetMomentum(thesis)
		momentumFloor := tracker.PeakMomentum * stopConfig.MomentumDecayFraction
		momentumHealth := 1.0
		if stopConfig.MomentumDecayFraction > 0 && tracker.PeakMomentum > 0 {
			denom := tracker.PeakMomentum - momentumFloor
			if denom > 0 {
				momentumHealth = math.Max(0, math.Min(1, (momentum-momentumFloor)/denom))
			} else {
				momentumHealth = 0
			}
		}

		stagnationHealth := 1.0
		if stopConfig.StagnationMaxTouches > 0 {
			stagnationHealth = math.Max(0, math.Min(1, float64(stopConfig.StagnationMaxTouches-tracker.PeakTouchCount)/float64(stopConfig.StagnationMaxTouches)))
		}

		stops[symbol] = map[string]any{
			"symbol":                 symbol,
			"stop_price":             stopPrice,
			"peak_return":            tracker.PeakReturn,
			"stop_return":            stopReturn,
			"momentum":               momentum,
			"peak_momentum":          tracker.PeakMomentum,
			"momentum_floor":         momentumFloor,
			"momentum_health":        momentumHealth,
			"momentum_active":        stopConfig.MomentumDecayFraction > 0,
			"peak_touch_count":       tracker.PeakTouchCount,
			"stagnation_max_touches": stopConfig.StagnationMaxTouches,
			"stagnation_health":      stagnationHealth,
			"stagnation_pending":     tracker.InStagnationZone,
			"stagnation_active":      stopConfig.StagnationMaxTouches > 0,
		}

		return true
	})

	return stops
}

func (planner *Planner) Publish(intents []strategy.Intent) {
	if planner.uiHub == nil {
		return
	}

	output := datura.Map[any]{}

	if len(intents) > 0 {
		output["intents"] = intents
	}

	stops := planner.Stops()

	if len(stops) > 0 {
		output["stops"] = stops
	}

	if len(output) == 0 {
		return
	}

	select {
	case planner.uiHub <- output.Marshal():
	default:
	}
}

// ── Fluid score helpers ────────────────────────────────────────────────────

func scoreLaminar(reading pmanifold.Reading) float64 {
	visc := reading.ViscosityProxy
	div := reading.Divergence
	turb := reading.CoherenceMag2

	viscTerm := clamp01(visc / (visc + 1))
	divTerm := 1 - clamp01(math.Abs(div))
	turbTerm := 1 - clamp01(turb)

	return viscTerm * divTerm * turbTerm
}

func scoreTurbulent(reading pmanifold.Reading) float64 {
	return clamp01(reading.CoherenceMag2) * clamp01(math.Abs(reading.Divergence))
}

func scoreInertial(reading pmanifold.Reading) float64 {
	press := clamp01(reading.PressureGradNorm / (reading.PressureGradNorm + 1))
	div := clamp01(math.Abs(reading.Divergence))

	return press * div
}

func scoreViscous(reading pmanifold.Reading) float64 {
	lowVisc := 1 - clamp01(reading.ViscosityProxy/(reading.ViscosityProxy+1))
	press := clamp01(reading.PressureGradNorm / (reading.PressureGradNorm + 1))

	return lowVisc * press
}

// ── Hawkes / Resonance score helpers ───────────────────────────────────────

func scoreFrenzy(outcome logic.ResonanceOutcome) float64 {
	energy := clamp01(outcome.Energy / (outcome.Energy + 1))
	surprise := clamp01(outcome.Surprise / (outcome.Surprise + 1))

	return energy * (1 - math.Abs(surprise-0.5))
}

func scoreSaturation(outcome logic.ResonanceOutcome) float64 {
	energy := clamp01(outcome.Energy / (outcome.Energy + 1))
	surprise := clamp01(outcome.Surprise / (outcome.Surprise + 1))

	return energy * surprise
}

func scoreOrganic(outcome logic.ResonanceOutcome) float64 {
	energy := 1 - clamp01(outcome.Energy/(outcome.Energy+1))
	surprise := 1 - clamp01(outcome.Surprise/(outcome.Surprise+1))

	return energy * surprise
}

func scoreExhaustion(outcome logic.ResonanceOutcome) float64 {
	energy := 1 - clamp01(outcome.Energy/(outcome.Energy+1))
	surprise := clamp01(outcome.Surprise / (outcome.Surprise + 1))

	return energy * surprise
}

// ── Helpers ────────────────────────────────────────────────────────────────

func clamp01(value float64) float64 {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return 0
	}

	if value < 0 {
		return 0
	}

	if value > 1 {
		return 1
	}

	return value
}

func appendNodeIfPositive(
	nodes *[]strategy.CategoryNode,
	category types.CategoryType,
	score float64,
) {
	if score <= 0 || math.IsNaN(score) || math.IsInf(score, 0) {
		return
	}

	*nodes = append(*nodes, strategy.CategoryNode{
		Category: category,
		Score:    score,
	})
}

func Evidence[T any](thesis *strategy.Thesis, key string) (T, bool) {
	var zero T

	snapshot, ok := thesis.Evidence(key)

	if !ok {
		return zero, false
	}

	value, ok := snapshot.(T)

	if !ok {
		return zero, false
	}

	return value, true
}
