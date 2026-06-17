package resonance

import (
	"math"
	"time"

	"github.com/theapemachine/nomagique/learning"
)

const defaultPredictionHorizonSec = 60

/*
PredictionFrames builds dashboard prediction chart points from the latest settle batch.
*/
func (signal *Signal) PredictionFrames(horizonSec int) []map[string]any {
	if signal == nil || len(signal.lastSettled) == 0 {
		return nil
	}

	if horizonSec <= 0 {
		horizonSec = defaultPredictionHorizonSec
	}

	frames := make([]map[string]any, 0, len(signal.lastSettled)*3)

	for _, entry := range signal.lastSettled {
		frames = append(frames, predictionFramesFromEntry(entry, horizonSec)...)
	}

	return frames
}

func predictionFramesFromEntry(entry settledSymbolEntry, horizonSec int) []map[string]any {
	if entry.measurement.Symbol == "" || len(entry.layers) == 0 {
		return nil
	}

	eventSec := entry.measurement.ObservedAt.Unix()

	if entry.measurement.ObservedAt.IsZero() {
		eventSec = time.Now().Unix()
	}

	forecast := meanLayerVector(entry.layers, layerPredictionMean)
	actual := meanLayerVector(entry.layers, layerStateMean)
	errorValue := forecast - actual

	if !finiteScalar(forecast) || !finiteScalar(actual) || !finiteScalar(errorValue) {
		return nil
	}

	symbol := entry.measurement.Symbol

	return []map[string]any{
		predictionPoint(symbol, "prediction", eventSec, forecast, horizonSec),
		predictionPoint(symbol, "actual", eventSec, actual, horizonSec),
		predictionPoint(symbol, "error", eventSec, errorValue, horizonSec),
	}
}

func predictionPoint(
	symbol string,
	kind string,
	eventSec int64,
	value float64,
	horizonSec int,
) map[string]any {
	return map[string]any{
		"type":    "prediction",
		"symbol":  symbol,
		"kind":    kind,
		"x":       eventSec,
		"value":   value,
		"horizon": horizonSec,
	}
}

func layerPredictionMean(layer learning.ResonanceLayerWire) float64 {
	return meanVector(layer.Prediction)
}

func layerStateMean(layer learning.ResonanceLayerWire) float64 {
	return meanVector(layer.State)
}

func meanLayerVector(
	layers []learning.ResonanceLayerWire,
	project func(learning.ResonanceLayerWire) float64,
) float64 {
	if len(layers) == 0 {
		return 0
	}

	sum := 0.0
	count := 0

	for _, layer := range layers {
		value := project(layer)

		if !finiteScalar(value) {
			continue
		}

		sum += value
		count++
	}

	if count == 0 {
		return 0
	}

	return sum / float64(count)
}

func meanVector(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}

	sum := 0.0
	count := 0

	for _, value := range values {
		if !finiteScalar(value) {
			continue
		}

		sum += value
		count++
	}

	if count == 0 {
		return 0
	}

	return sum / float64(count)
}

func finiteScalar(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}
