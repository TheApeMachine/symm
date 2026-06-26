package manifold

import (
	"encoding/binary"
	"encoding/json"
	"math"
	"time"

	"github.com/theapemachine/datura"
	mkernel "github.com/theapemachine/nomagique/physics/manifold"
)

func (field *Field) featureVector(symbol string) (pressure, coherence, guidance, viscosity float64, ok bool) {
	if field == nil {
		return 0, 0, 0, 0, false
	}

	reading, _, _, found := field.Reading(symbol)

	if found && readingHasSignal(reading) {
		return reading.PressureGradNorm, reading.CoherenceMag2, reading.GuidanceSpeed, reading.ViscosityProxy, true
	}

	return 0, 0, 0, 0, false
}

func readingHasSignal(reading mkernel.Reading) bool {
	if !reading.IsFinite() {
		return false
	}

	return reading.PressureGradNorm != 0 ||
		reading.CoherenceMag2 != 0 ||
		reading.GuidanceSpeed != 0 ||
		reading.ViscosityProxy != 0
}

func encodeFeaturePayload(pressure, coherence, guidance, viscosity float64) []byte {
	samples := []float64{pressure, coherence, guidance, viscosity}
	payload := make([]byte, 8*len(samples))

	for index, sample := range samples {
		offset := index * 8
		binary.BigEndian.PutUint64(payload[offset:offset+8], math.Float64bits(sample))
	}

	return payload
}

func decodeFeaturePayload(raw []byte) (pressure, coherence, guidance, viscosity float64, ok bool) {
	if len(raw) == 0 {
		return 0, 0, 0, 0, false
	}

	if raw[0] == '{' {
		var body map[string]any

		if json.Unmarshal(raw, &body) != nil {
			return 0, 0, 0, 0, false
		}

		features, featuresOk := body["features"].([]any)

		if !featuresOk || len(features) < 4 {
			return 0, 0, 0, 0, false
		}

		pressure, _ = features[0].(float64)
		coherence, _ = features[1].(float64)
		guidance, _ = features[2].(float64)
		viscosity, _ = features[3].(float64)

		return pressure, coherence, guidance, viscosity, true
	}

	samples := make([]float64, 0, len(raw)/8)

	for offset := 0; offset+8 <= len(raw); offset += 8 {
		bits := binary.BigEndian.Uint64(raw[offset : offset+8])
		sample := math.Float64frombits(bits)

		if math.IsNaN(sample) || math.IsInf(sample, 0) {
			continue
		}

		samples = append(samples, sample)
	}

	if len(samples) < 4 {
		return 0, 0, 0, 0, false
	}

	return samples[0], samples[1], samples[2], samples[3], true
}

func (signal *Signal) integrateField(eventAt time.Time) {
	if signal == nil || signal.field == nil {
		return
	}

	if eventAt.IsZero() {
		eventAt = time.Now()
	}

	if stepErr := signal.field.maybeStep(eventAt); stepErr != nil {
		signal.err = stepErr
	}
}

func (signal *Signal) publishFeatures(scope string, payload []byte) {
	if signal == nil || signal.tree == nil || scope == "" || len(payload) == 0 {
		return
	}

	artifact := datura.Acquire("manifold-features", datura.APPJSON)
	artifact.WithRole("features")
	artifact.WithScope(scope)
	artifact.WithPayload(payload)

	if updated, _ := signal.tree.InsertArtifact(artifact.Prefix("role", "scope"), artifact); updated != nil {
		signal.tree = updated
	}

	artifact.Release()
}

func (signal *Signal) resolveFeatures(
	scope string,
	eventAt time.Time,
) (pressure, coherence, guidance, viscosity float64, ok bool) {
	eventStamp := eventAt.UnixNano()

	if signal != nil &&
		signal.featureCache.ok &&
		signal.featureCache.scope == scope &&
		signal.featureCache.eventStamp == eventStamp {
		return signal.featureCache.pressure,
			signal.featureCache.coherence,
			signal.featureCache.guidance,
			signal.featureCache.viscosity,
			true
	}

	signal.integrateField(eventAt)

	if signal.field != nil {
		pressure, coherence, guidance, viscosity, ok = signal.field.featureVector(scope)

		if ok {
			payload := encodeFeaturePayload(pressure, coherence, guidance, viscosity)
			signal.publishFeatures(scope, payload)
			signal.rememberFeatures(scope, eventStamp, pressure, coherence, guidance, viscosity, true)

			return pressure, coherence, guidance, viscosity, true
		}
	}

	for inbound := range signal.tree.Seek([]byte("features/" + scope)) {
		if inbound == nil || !inbound.HasPayload() {
			continue
		}

		payload := inbound.DecryptPayload()
		pressure, coherence, guidance, viscosity, decoded := decodeFeaturePayload(payload)

		if decoded {
			signal.rememberFeatures(scope, eventStamp, pressure, coherence, guidance, viscosity, true)

			return pressure, coherence, guidance, viscosity, true
		}
	}

	signal.rememberFeatures(scope, eventStamp, 0, 0, 0, 0, false)

	return 0, 0, 0, 0, false
}

func (signal *Signal) rememberFeatures(
	scope string,
	eventStamp int64,
	pressure, coherence, guidance, viscosity float64,
	ok bool,
) {
	if signal == nil {
		return
	}

	signal.featureCache = featureCacheEntry{
		scope:      scope,
		eventStamp: eventStamp,
		pressure:   pressure,
		coherence:  coherence,
		guidance:   guidance,
		viscosity:  viscosity,
		ok:         ok,
	}
}
