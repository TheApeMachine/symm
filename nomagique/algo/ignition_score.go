package algo

import (
	"fmt"
	"math"

	"github.com/theapemachine/symm/nomagique"
)

func scoreIgnition(
	state *nomagique.Frame,
	barRate float64,
	priceMove float64,
	spread float64,
) error {
	rateBaseline, rateReady, err := ignitionHistoryMedian(state, historyRates)

	if err != nil {
		return err
	}

	spreadBaseline, spreadReady, err := ignitionHistoryMedian(state, historySpreads)

	if err != nil {
		return err
	}

	precursorBaseline, precursorReady, err := ignitionHistoryMedian(
		state,
		historyPrecursors,
	)

	if err != nil {
		return err
	}

	moveBaseline, moveReady, err := ignitionHistoryMedian(state, historyReturns)

	if err != nil {
		return err
	}

	rvol := ignitionRatio(barRate, rateBaseline, rateReady)
	buyMove := math.Max(priceMove, 0)
	sellMove := math.Max(-priceMove, 0)
	buyPrecursor := ignitionRatio(buyMove, precursorBaseline, precursorReady)
	sellPrecursor := ignitionRatio(sellMove, precursorBaseline, precursorReady)
	compression := ignitionCompression(spread, spreadBaseline, spreadReady)
	buyRejection := ignitionRatio(sellMove, moveBaseline, moveReady)
	sellRejection := ignitionRatio(buyMove, moveBaseline, moveReady)
	rvolScale := ignitionRatio(rateBaseline, rateBaseline, rateReady)
	precursorScale := ignitionRatio(
		precursorBaseline,
		precursorBaseline,
		precursorReady,
	)
	moveScale := ignitionRatio(moveBaseline, moveBaseline, moveReady)
	compressionScale, err := ignitionCompressionScale(state, spreadBaseline, spreadReady)

	if err != nil {
		return err
	}

	buyIgnition, buyTrend, buyCompression := ignitionFamily(
		rvol,
		buyPrecursor,
		compression,
		rvolScale,
		precursorScale,
		compressionScale,
	)
	sellIgnition, sellTrend, sellCompression := ignitionFamily(
		rvol,
		sellPrecursor,
		compression,
		rvolScale,
		precursorScale,
		compressionScale,
	)
	priorRVOL := number(*state, SymbolIgnitionLastRVOL)
	buyExhaustion := ignitionExhaustion(priorRVOL, rvol, buyRejection, moveScale)
	sellExhaustion := ignitionExhaustion(priorRVOL, rvol, sellRejection, moveScale)
	buyStrength := maximum(
		buyIgnition,
		buyCompression,
		buyTrend,
		buyExhaustion,
	)
	sellStrength := maximum(
		sellIgnition,
		sellCompression,
		sellTrend,
		sellExhaustion,
	)

	if !finite(
		rvol,
		buyPrecursor,
		sellPrecursor,
		buyStrength,
		sellStrength,
	) {
		return fmt.Errorf("ignition: calculated strength must be finite")
	}

	putIgnitionSide(
		state,
		true,
		rvol,
		buyPrecursor,
		buyCompression,
		buyIgnition,
		buyTrend,
		buyExhaustion,
		buyStrength,
	)
	putIgnitionSide(
		state,
		false,
		rvol,
		sellPrecursor,
		sellCompression,
		sellIgnition,
		sellTrend,
		sellExhaustion,
		sellStrength,
	)

	if sellStrength > buyStrength {
		state.Put(SymbolValue, sellStrength)
		state.Put(SymbolPrecursor, sellPrecursor)
		state.Put(SymbolCompression, sellCompression)
		state.Put(SymbolIgnition, sellIgnition)
		state.Put(SymbolTrend, sellTrend)
		state.Put(SymbolExhaustion, sellExhaustion)
		state.Put(SymbolStrength, sellStrength)
		state.Put(SymbolCategory, 0)
	} else {
		state.Put(SymbolValue, buyStrength)
		state.Put(SymbolPrecursor, buyPrecursor)
		state.Put(SymbolCompression, buyCompression)
		state.Put(SymbolIgnition, buyIgnition)
		state.Put(SymbolTrend, buyTrend)
		state.Put(SymbolExhaustion, buyExhaustion)
		state.Put(SymbolStrength, buyStrength)
		state.Put(SymbolCategory, 0)
	}

	state.Put(SymbolRVOL, rvol)
	state.Put(SymbolIgnitionLastRVOL, rvol)
	state.Put(SymbolIgnitionClassified, boolNumber(rateReady && spreadReady))

	return nil
}

func ignitionFamily(
	rvol float64,
	precursor float64,
	compression float64,
	rvolScale float64,
	precursorScale float64,
	compressionScale float64,
) (float64, float64, float64) {
	scaledRVOL := ignitionSquash(rvol, rvolScale)
	quietCompression := ignitionInverse(compression, compressionScale)
	quietPrecursor := ignitionInverse(precursor, precursorScale)
	ignitionScore := ignitionEvidence(rvol > 0 && precursor > 0, rvol, precursor)
	trendScore := ignitionEvidence(
		precursor > 0 && scaledRVOL > 0 && quietCompression > 0,
		precursor,
		scaledRVOL,
		quietCompression,
	)
	compressionScore := ignitionEvidence(
		compression > 0 && scaledRVOL > 0 && quietPrecursor > 0,
		compression,
		scaledRVOL,
		quietPrecursor,
	)

	return ignitionScore, trendScore, compressionScore
}

func ignitionExhaustion(
	priorRVOL float64,
	rvol float64,
	rejection float64,
	moveScale float64,
) float64 {
	positiveDecrease := math.Max(priorRVOL-rvol, 0)
	relativeDecrease := ignitionRatio(positiveDecrease, priorRVOL, priorRVOL > 0)
	scaledRejection := ignitionSquash(rejection, moveScale)

	return relativeDecrease * scaledRejection
}

func ignitionCompression(value float64, baseline float64, ready bool) float64 {
	normalized := ignitionRatio(value, baseline, ready)

	return math.Max(1-normalized, 0)
}

func ignitionCompressionScale(
	state *nomagique.Frame,
	baseline float64,
	ready bool,
) (float64, error) {
	if !ready || baseline <= 0 {
		return 0, nil
	}

	count := int(number(*state, ignitionHistoryCounts[historySpreads]))
	values := [MaxIgnitionHistory]float64{}
	retained := 0

	for index := 0; index < count; index++ {
		spread, found := state.Get(ignitionHistorySamples[historySpreads][index])

		if !found || !finite(spread) {
			return 0, fmt.Errorf("ignition: spread history sample %d is invalid", index)
		}

		compression := ignitionCompression(spread, baseline, true)

		if compression <= 0 {
			continue
		}

		values[retained] = compression
		retained++
	}

	if retained == 0 {
		return 0, nil
	}

	sortIgnitionValues(&values, retained)
	middle := retained / 2
	median := values[middle]

	if retained%2 == 0 {
		median = (values[middle-1] + values[middle]) / 2
	}

	if median <= 0 || !finite(median) {
		return 0, nil
	}

	return median, nil
}

func ignitionEvidence(ready bool, values ...float64) float64 {
	if !ready || len(values) == 0 {
		return 0
	}

	logSum := 0.0

	for _, value := range values {
		if value <= 0 || !finite(value) {
			return 0
		}

		logSum += math.Log(value)
	}

	return math.Exp(logSum / float64(len(values)))
}

func ignitionRatio(value float64, baseline float64, ready bool) float64 {
	if !ready || value <= 0 || baseline <= 0 || !finite(value, baseline) {
		return 0
	}

	return value / baseline
}

func ignitionSquash(value float64, scale float64) float64 {
	if value <= 0 || scale <= 0 || !finite(value, scale) {
		return 0
	}

	return value / (scale + value)
}

func ignitionInverse(value float64, scale float64) float64 {
	switch {
	case !finite(value, scale):
		return 0
	case value < 0:
		return 0
	case value == 0:
		return 1
	case scale <= 0:
		return 0
	default:
		return scale / (scale + value)
	}
}

func maximum(values ...float64) float64 {
	result := 0.0

	for _, value := range values {
		result = math.Max(result, value)
	}

	return result
}

func putIgnitionSide(
	state *nomagique.Frame,
	buy bool,
	rvol float64,
	precursor float64,
	compression float64,
	ignitionScore float64,
	trend float64,
	exhaustion float64,
	strength float64,
) {
	if buy {
		state.Put(SymbolBuyValue, strength)
		state.Put(SymbolBuyRVOL, rvol)
		state.Put(SymbolBuyPrecursor, precursor)
		state.Put(SymbolBuyCompression, compression)
		state.Put(SymbolBuyIgnition, ignitionScore)
		state.Put(SymbolBuyTrend, trend)
		state.Put(SymbolBuyExhaustion, exhaustion)
		state.Put(SymbolBuyStrength, strength)
		state.Put(SymbolBuyCategory, 0)

		return
	}

	state.Put(SymbolSellValue, strength)
	state.Put(SymbolSellRVOL, rvol)
	state.Put(SymbolSellPrecursor, precursor)
	state.Put(SymbolSellCompression, compression)
	state.Put(SymbolSellIgnition, ignitionScore)
	state.Put(SymbolSellTrend, trend)
	state.Put(SymbolSellExhaustion, exhaustion)
	state.Put(SymbolSellStrength, strength)
	state.Put(SymbolSellCategory, 0)
}
