package algo

import (
	"fmt"

	"github.com/theapemachine/symm/nomagique/types"
)

func (ignition *Ignition) score(
	mapping ignitionMap,
	barRate float64,
	priceMove float64,
	spread float64,
) error {
	rateBaseline, rateReady, err := ignition.historyMedian(mapping, "rates")
	if err != nil {
		return err
	}
	spreadBaseline, spreadReady, err := ignition.historyMedian(mapping, "spreads")
	if err != nil {
		return err
	}
	precursorBaseline, precursorReady, err := ignition.historyMedian(mapping, "precursors")
	if err != nil {
		return err
	}
	moveBaseline, moveReady, err := ignition.historyMedian(mapping, "returns")
	if err != nil {
		return err
	}

	rvol, err := ignition.ratio(barRate, rateBaseline, rateReady)
	if err != nil {
		return err
	}
	buyMove, err := ignition.positive(priceMove)
	if err != nil {
		return err
	}
	sellDirection, err := ignition.difference(0, priceMove)
	if err != nil {
		return err
	}
	sellMove, err := ignition.positive(sellDirection)
	if err != nil {
		return err
	}
	buyPrecursor, err := ignition.ratio(buyMove, precursorBaseline, precursorReady)
	if err != nil {
		return err
	}
	sellPrecursor, err := ignition.ratio(sellMove, precursorBaseline, precursorReady)
	if err != nil {
		return err
	}
	compression, err := ignition.compression(spread, spreadBaseline, spreadReady)
	if err != nil {
		return err
	}
	buyRejection, err := ignition.ratio(sellMove, moveBaseline, moveReady)
	if err != nil {
		return err
	}
	sellRejection, err := ignition.ratio(buyMove, moveBaseline, moveReady)
	if err != nil {
		return err
	}

	rvolScale, err := ignition.ratio(rateBaseline, rateBaseline, rateReady)
	if err != nil {
		return err
	}
	precursorScale, err := ignition.ratio(precursorBaseline, precursorBaseline, precursorReady)
	if err != nil {
		return err
	}
	moveScale, err := ignition.ratio(moveBaseline, moveBaseline, moveReady)
	if err != nil {
		return err
	}
	compressionScale, err := ignition.compressionScale(mapping, spreadBaseline, spreadReady)
	if err != nil {
		return err
	}

	buy, err := ignition.family(
		rvol,
		buyPrecursor,
		compression,
		rvolScale,
		precursorScale,
		compressionScale,
	)
	if err != nil {
		return err
	}
	sell, err := ignition.family(
		rvol,
		sellPrecursor,
		compression,
		rvolScale,
		precursorScale,
		compressionScale,
	)
	if err != nil {
		return err
	}

	buyExhaustion, err := ignition.exhaustion(
		ignitionNumber(mapping, ignitionLastRVOL),
		rvol,
		buyRejection,
		moveScale,
	)
	if err != nil {
		return err
	}
	sellExhaustion, err := ignition.exhaustion(
		ignitionNumber(mapping, ignitionLastRVOL),
		rvol,
		sellRejection,
		moveScale,
	)
	if err != nil {
		return err
	}
	ignitionPut(buy, "exhaustion", buyExhaustion)
	ignitionPut(sell, "exhaustion", sellExhaustion)

	buyStrength, err := ignition.maximum(
		ignitionNumber(buy, "ignition"),
		ignitionNumber(buy, "compression"),
		ignitionNumber(buy, "trend"),
		buyExhaustion,
	)
	if err != nil {
		return err
	}
	sellStrength, err := ignition.maximum(
		ignitionNumber(sell, "ignition"),
		ignitionNumber(sell, "compression"),
		ignitionNumber(sell, "trend"),
		sellExhaustion,
	)
	if err != nil {
		return err
	}
	ignitionPut(buy, "strength", buyStrength)
	ignitionPut(buy, "value", buyStrength)
	ignitionPut(sell, "strength", sellStrength)
	ignitionPut(sell, "value", sellStrength)

	if !ignitionFinite(buyStrength, sellStrength) {
		return ignitionError("calculated strength must be finite")
	}

	ignition.copySide(mapping, "buy", buy)
	ignition.copySide(mapping, "sell", sell)
	legacy := buy
	if sellStrength > buyStrength {
		legacy = sell
	}
	for _, key := range []string{
		"value", "precursor", "compression", "ignition", "trend",
		"exhaustion", "strength", "category",
	} {
		ignitionPut(mapping, key, ignitionNumber(legacy, key))
	}
	ignitionPut(mapping, "rvol", rvol)
	ignitionPut(mapping, ignitionLastRVOL, rvol)
	ignitionPut(mapping, ignitionClassified, ignitionBool(rateReady && spreadReady))
	return nil
}

func (ignition *Ignition) family(
	rvol float64,
	precursor float64,
	compression float64,
	rvolScale float64,
	precursorScale float64,
	compressionScale float64,
) (ignitionMap, error) {
	scaledRVOL, err := ignition.squash(rvol, rvolScale)
	if err != nil {
		return ignitionMap{}, err
	}
	quietCompression, err := ignition.inverse(compression, compressionScale)
	if err != nil {
		return ignitionMap{}, err
	}
	quietPrecursor, err := ignition.inverse(precursor, precursorScale)
	if err != nil {
		return ignitionMap{}, err
	}
	ignitionScore, err := ignition.evidence(rvol > 0 && precursor > 0, rvol, precursor)
	if err != nil {
		return ignitionMap{}, err
	}
	trendScore, err := ignition.evidence(
		precursor > 0 && scaledRVOL > 0 && quietCompression > 0,
		precursor,
		scaledRVOL,
		quietCompression,
	)
	if err != nil {
		return ignitionMap{}, err
	}
	compressionScore, err := ignition.evidence(
		compression > 0 && scaledRVOL > 0 && quietPrecursor > 0,
		compression,
		scaledRVOL,
		quietPrecursor,
	)
	if err != nil {
		return ignitionMap{}, err
	}

	output := types.NewMap[string, types.Value[float64]]()
	ignitionPut(output, "value", 0)
	ignitionPut(output, "rvol", rvol)
	ignitionPut(output, "precursor", precursor)
	ignitionPut(output, "compression", compressionScore)
	ignitionPut(output, "ignition", ignitionScore)
	ignitionPut(output, "trend", trendScore)
	ignitionPut(output, "exhaustion", 0)
	ignitionPut(output, "strength", 0)
	ignitionPut(output, "category", 0)
	return output, nil
}

func (ignition *Ignition) exhaustion(
	priorRVOL float64,
	rvol float64,
	rejection float64,
	moveScale float64,
) (float64, error) {
	decrease, err := ignition.difference(priorRVOL, rvol)
	if err != nil {
		return 0, err
	}
	positiveDecrease, err := ignition.positive(decrease)
	if err != nil {
		return 0, err
	}
	relativeDecrease, err := ignition.ratio(positiveDecrease, priorRVOL, priorRVOL > 0)
	if err != nil {
		return 0, err
	}
	scaledRejection, err := ignition.squash(rejection, moveScale)
	if err != nil {
		return 0, err
	}
	return ignition.product(relativeDecrease, scaledRejection)
}

func (ignition *Ignition) compression(
	value float64,
	baseline float64,
	ready bool,
) (float64, error) {
	normalized, err := ignition.ratio(value, baseline, ready)
	if err != nil {
		return 0, err
	}
	remaining, err := ignition.difference(1, normalized)
	if err != nil {
		return 0, err
	}
	return ignition.positive(remaining)
}

func (ignition *Ignition) compressionScale(
	mapping ignitionMap,
	baseline float64,
	ready bool,
) (float64, error) {
	if !ready || baseline <= 0 {
		return 0, nil
	}

	samples := types.NewMap[string, types.Value[float64]]()
	index := 0
	for _, spread := range ignition.historyValues(mapping, "spreads") {
		compression, err := ignition.compression(spread, baseline, true)
		if err != nil {
			return 0, err
		}
		if compression <= 0 {
			continue
		}
		samples.Put(fmt.Sprintf("sample/%d", index), types.NewValue(compression))
		index++
	}

	median, medianReady, err := ignition.median(samples)
	if err != nil {
		return 0, err
	}
	if !medianReady || median <= 0 || !ignitionFinite(median) {
		return 0, nil
	}
	return median, nil
}
