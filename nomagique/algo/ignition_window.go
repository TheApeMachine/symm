package algo

import (
	"math"
	"strings"

	"github.com/theapemachine/symm/nomagique/statistic"
	"github.com/theapemachine/symm/nomagique/transport"
	"github.com/theapemachine/symm/nomagique/types"
)

func (ignition *Ignition) initialize(
	mapping ignitionMap,
	capacity float64,
	volume float64,
	last float64,
	spread float64,
	sec float64,
	nsec float64,
	hasTime bool,
) error {
	ignitionPut(mapping, ignitionInitialized, 1)
	ignitionPut(mapping, ignitionClassified, 0)
	ignitionPut(mapping, ignitionBars, 0)
	ignitionPut(mapping, ignitionPrevClose, last)
	ignitionPut(mapping, ignitionLastRVOL, 0)
	ignitionPut(mapping, ignitionBarVolume, volume)
	ignition.initializeOutput(mapping)

	if hasTime {
		ignitionPut(mapping, ignitionHaveTime, 1)
		ignitionPut(mapping, ignitionLastSec, sec)
		ignitionPut(mapping, ignitionLastNsec, nsec)
		ignitionPut(mapping, ignitionBarOpenSec, sec)
		ignitionPut(mapping, ignitionBarOpenNsec, nsec)
	}

	if err := ignition.appendHistory(mapping, "deltas", capacity, volume, positiveOnly); err != nil {
		return err
	}
	if err := ignition.appendHistory(mapping, "spreads", capacity, spread, positiveOnly); err != nil {
		return err
	}

	return nil
}

func (ignition *Ignition) initializeOutput(mapping ignitionMap) {
	for _, key := range []string{
		"value", "rvol", "precursor", "spread", "compression", "ignition",
		"trend", "exhaustion", "strength", "category", "ready", "maturity",
	} {
		ignitionPut(mapping, key, 0)
	}
	for _, side := range []string{"buy", "sell"} {
		for _, key := range []string{
			"value", "rvol", "precursor", "compression", "ignition",
			"trend", "exhaustion", "strength", "category",
		} {
			ignitionPut(mapping, side+"/"+key, 0)
		}
	}
}

func (ignition *Ignition) advance(
	mapping ignitionMap,
	capacity float64,
	volume float64,
	last float64,
	spread float64,
	sec float64,
	nsec float64,
	hasTime bool,
) error {
	windowHasTime := ignitionFlag(mapping, ignitionHaveTime)
	if windowHasTime && hasTime && ignitionBefore(
		sec,
		nsec,
		ignitionNumber(mapping, ignitionLastSec),
		ignitionNumber(mapping, ignitionLastNsec),
	) {
		return ignitionError("observation time cannot move backwards")
	}

	if hasTime {
		ignitionPut(mapping, ignitionLastSec, sec)
		ignitionPut(mapping, ignitionLastNsec, nsec)
		if !windowHasTime {
			windowHasTime = true
			ignitionPut(mapping, ignitionHaveTime, 1)
			ignitionPut(mapping, ignitionBarOpenSec, sec)
			ignitionPut(mapping, ignitionBarOpenNsec, nsec)
		}
	}

	barVolume, err := ignition.sum(
		ignitionNumber(mapping, ignitionBarVolume),
		volume,
	)
	if err != nil {
		return err
	}
	ignitionPut(mapping, ignitionBarVolume, barVolume)

	barTarget, targetReady, err := ignition.historyMedian(mapping, "deltas")
	if err != nil {
		return err
	}
	barOpenSec := ignitionNumber(mapping, ignitionBarOpenSec)
	barOpenNsec := ignitionNumber(mapping, ignitionBarOpenNsec)
	closes := targetReady && barTarget > 0 && barVolume >= barTarget &&
		windowHasTime && hasTime && ignitionAfter(sec, nsec, barOpenSec, barOpenNsec)

	if closes {
		if err := ignition.closeBar(mapping, capacity, last, spread, sec, nsec, barVolume); err != nil {
			return err
		}
	}

	if err := ignition.appendHistory(mapping, "deltas", capacity, volume, positiveOnly); err != nil {
		return err
	}
	if err := ignition.appendHistory(mapping, "spreads", capacity, spread, positiveOnly); err != nil {
		return err
	}

	return nil
}

func (ignition *Ignition) closeBar(
	mapping ignitionMap,
	capacity float64,
	last float64,
	spread float64,
	sec float64,
	nsec float64,
	barVolume float64,
) error {
	priceMove, err := ignition.logRatio(last, ignitionNumber(mapping, ignitionPrevClose))
	if err != nil {
		return err
	}
	duration, err := ignition.duration(
		sec,
		nsec,
		ignitionNumber(mapping, ignitionBarOpenSec),
		ignitionNumber(mapping, ignitionBarOpenNsec),
	)
	if err != nil {
		return err
	}
	if duration <= 0 {
		return ignitionError("volume bar requires positive elapsed event time")
	}

	barRate, err := ignition.rate(barVolume, duration)
	if err != nil {
		return err
	}
	if err := ignition.score(mapping, barRate, priceMove, spread); err != nil {
		return err
	}

	if err := ignition.appendHistory(mapping, "rates", capacity, barRate, positiveOnly); err != nil {
		return err
	}
	if err := ignition.appendHistory(mapping, "returns", capacity, math.Abs(priceMove), nonNegative); err != nil {
		return err
	}
	if err := ignition.appendHistory(mapping, "precursors", capacity, math.Abs(priceMove), positiveOnly); err != nil {
		return err
	}

	bars, err := ignition.sum(ignitionNumber(mapping, ignitionBars), 1)
	if err != nil {
		return err
	}
	ignitionPut(mapping, ignitionBars, bars)
	ignitionPut(mapping, ignitionPrevClose, last)
	ignitionPut(mapping, ignitionBarOpenSec, sec)
	ignitionPut(mapping, ignitionBarOpenNsec, nsec)
	ignitionPut(mapping, ignitionBarVolume, 0)
	return nil
}

func (ignition *Ignition) compose(mapping ignitionMap, spread float64) {
	ignitionPut(mapping, "spread", spread)
	ready := ignitionFlag(mapping, ignitionClassified)
	ignitionPut(mapping, "ready", ignitionBool(ready))
	bars := ignitionNumber(mapping, ignitionBars)
	maturity := 0.0
	if bars >= 0 {
		maturity = bars / (bars + 1)
	}
	ignitionPut(mapping, "maturity", maturity)
}

func (ignition *Ignition) copySide(mapping ignitionMap, side string, values ignitionMap) {
	for key, value := range values.All() {
		mapping.Put(side+"/"+key, value)
	}
}

func (ignition *Ignition) appendHistory(
	mapping ignitionMap,
	name string,
	capacity float64,
	value float64,
	accept func(float64) bool,
) error {
	if !accept(value) {
		return nil
	}

	windowState := ignition.history(mapping, name)
	ignitionPut(windowState, "capacity", capacity)
	ignitionPut(windowState, "sample", value)
	input := types.NewInput(types.NewValue(windowState))
	primitive := transport.NewWindow(input)
	output, err := ignition.execute("retain history", input, primitive)
	if err != nil {
		return err
	}
	ignition.saveHistory(mapping, name, output)
	return nil
}

func (ignition *Ignition) historyMedian(mapping ignitionMap, name string) (float64, bool, error) {
	return ignition.median(transport.Samples(ignition.history(mapping, name)))
}

func (ignition *Ignition) median(samples ignitionMap) (float64, bool, error) {
	input := types.NewInput(types.NewValue(samples))
	primitive := statistic.NewMedian(input)
	output, err := ignition.execute("median", input, primitive)
	if err != nil {
		return 0, false, err
	}
	return ignitionNumber(output, "result"), ignitionFlag(output, "ready"), nil
}

func (ignition *Ignition) history(mapping ignitionMap, name string) ignitionMap {
	prefix := "history/" + name + "/"
	output := types.NewMap[string, types.Value[float64]]()
	for key, value := range mapping.All() {
		if strings.HasPrefix(key, prefix) {
			output.Put(strings.TrimPrefix(key, prefix), value)
		}
	}
	return output
}

func (ignition *Ignition) historyValues(mapping ignitionMap, name string) []float64 {
	values := make([]float64, 0)
	for key, value := range ignition.history(mapping, name).All() {
		if strings.HasPrefix(key, "sample/") {
			values = append(values, value.Read())
		}
	}
	return values
}

func (ignition *Ignition) saveHistory(mapping ignitionMap, name string, history ignitionMap) {
	prefix := "history/" + name + "/"
	remove := make([]string, 0)
	for key := range mapping.All() {
		if strings.HasPrefix(key, prefix) {
			remove = append(remove, key)
		}
	}
	for _, key := range remove {
		mapping.Delete(key)
	}
	for key, value := range history.All() {
		if key == "sample" {
			continue
		}
		mapping.Put(prefix+key, value)
	}
}
