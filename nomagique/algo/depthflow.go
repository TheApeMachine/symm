package algo

import (
	"math"

	"github.com/theapemachine/symm/nomagique/statistic"
	"github.com/theapemachine/symm/nomagique/temporal"
	"github.com/theapemachine/symm/nomagique/types"
	nmtypes "github.com/theapemachine/symm/nomagique/types"
)

var (
	SymbolTouchBidQty    = nmtypes.MustIntern("depthflow/touch_bid_qty")
	SymbolTouchAskQty    = nmtypes.MustIntern("depthflow/touch_ask_qty")
	SymbolDeepBidQty     = nmtypes.MustIntern("depthflow/deep_bid_qty")
	SymbolDeepAskQty     = nmtypes.MustIntern("depthflow/deep_ask_qty")
	SymbolTouchImbalance = nmtypes.MustIntern("depthflow/touch_imbalance")
	SymbolDeepImbalance  = nmtypes.MustIntern("depthflow/deep_imbalance")
	SymbolSpoofScore     = nmtypes.MustIntern("depthflow/spoof_score")
	SymbolLoadedScore    = nmtypes.MustIntern("depthflow/loaded_score")
	SymbolThinScore      = nmtypes.MustIntern("depthflow/thin_score")
	SymbolNeutralScore   = nmtypes.MustIntern("depthflow/neutral_score")
	SymbolSeparation     = nmtypes.MustIntern("depthflow/separation")
)

/*
Depthflow evaluates multi-resolution order book imbalance (touch vs decayed deep),
adaptive total depth baseline, and hypothesis scores (loaded, spoofed, thinning, neutral).
*/
func Depthflow() nmtypes.Primitive {
	return nmtypes.Pipe(
		imbalanceCalculator,
		nmtypes.Configure(
			statistic.Baseline,
			nmtypes.Span,
			temporal.Window,
		),
		hypothesisClassifier,
	)
}

func imbalanceCalculator(
	state types.Frame,
	input types.Frame,
) (types.Frame, types.Frame, error) {
	touchBid, _ := input.Get(SymbolTouchBidQty)
	touchAsk, _ := input.Get(SymbolTouchAskQty)
	deepBid, _ := input.Get(SymbolDeepBidQty)
	deepAsk, _ := input.Get(SymbolDeepAskQty)

	touchTotal := touchBid + touchAsk
	touchImbalance := 0.0

	if touchTotal > 0 {
		touchImbalance = (touchBid - touchAsk) / touchTotal
	}

	deepTotal := deepBid + deepAsk
	deepImbalance := 0.0

	if deepTotal > 0 {
		deepImbalance = (deepBid - deepAsk) / deepTotal
	}

	totalDepth := touchTotal + deepTotal

	output := input
	output.Put(SymbolTouchImbalance, touchImbalance)
	output.Put(SymbolDeepImbalance, deepImbalance)
	output.Put(nmtypes.SampleValue, totalDepth)

	return state, output, nil
}

func hypothesisClassifier(
	state types.Frame,
	input types.Frame,
) (types.Frame, types.Frame, error) {
	touchImbalance, _ := input.Get(SymbolTouchImbalance)
	deepImbalance, _ := input.Get(SymbolDeepImbalance)
	totalDepth, _ := input.Get(nmtypes.SampleValue)
	baselineDepth, hasBaseline := input.Get(statistic.SymbolBaselineValue)

	if !hasBaseline {
		baselineDepth = totalDepth
	}

	// 1. Spoof score: contrast between touch and deep when signs disagree
	spoofScore := 0.0

	if touchImbalance*deepImbalance < 0 {
		spoofScore = math.Abs(touchImbalance-deepImbalance) / 2.0
	}

	// 2. Loaded score: alignment when both touch and deep agree strongly in direction
	loadedScore := 0.0

	if touchImbalance*deepImbalance > 0 {
		loadedScore = math.Abs(touchImbalance) * math.Abs(deepImbalance)
	}

	// 3. Thin score: total book depth collapsing below its adaptive baseline
	thinScore := 0.0

	if baselineDepth > 0 && totalDepth < baselineDepth {
		thinScore = (baselineDepth - totalDepth) / baselineDepth
	}

	if thinScore > 1.0 {
		thinScore = 1.0
	}

	// 4. Neutral score: residual balance
	maxEvidence := math.Max(spoofScore, math.Max(loadedScore, thinScore))
	neutralScore := math.Max(0.0, 1.0-maxEvidence)

	// 5. SNR separation among hypothesis candidates
	scores := []float64{loadedScore, spoofScore, thinScore, neutralScore}
	dominant, runnerUp := topTwo(scores)
	separation := 0.0

	if dominant > 0 {
		separation = (dominant - runnerUp) / dominant
	}

	output := input
	output.Put(SymbolSpoofScore, spoofScore)
	output.Put(SymbolLoadedScore, loadedScore)
	output.Put(SymbolThinScore, thinScore)
	output.Put(SymbolNeutralScore, neutralScore)
	output.Put(SymbolSeparation, separation)

	return state, output, nil
}

func topTwo(values []float64) (float64, float64) {
	first, second := 0.0, 0.0

	for _, value := range values {
		if value > first {
			second = first
			first = value
			continue
		}

		if value > second {
			second = value
		}
	}

	return first, second
}
