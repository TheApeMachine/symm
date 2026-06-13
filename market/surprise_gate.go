package market

import (
	"math"
	"sync"
	"sync/atomic"

	"github.com/theapemachine/nomagique/core"
	"github.com/theapemachine/nomagique/probability"
	"github.com/theapemachine/symm/logic"
)

const (
	surpriseBaselineAlpha   = 0.04
	surpriseBaselineMinObs  = 32
	surpriseBaselineFloor   = 0.05
	surpriseWarmupSigma     = 1.0
	surpriseTemperatureSpan = 2.0
)

/*
SurpriseGate tracks live surprise distribution for one signal source.
Thresholds are derived from the stream mean plus temperature-scaled sigma —
not from static YAML constants.
*/
type SurpriseGate struct {
	stream     Baseline
	changeSum  probability.ChangeSum
	lastSample float64
}

func newSurpriseGate() *SurpriseGate {
	return &SurpriseGate{
		stream: *NewBaseline(surpriseBaselineFloor, surpriseBaselineMinObs),
	}
}

func (gate *SurpriseGate) Observe(surprise float64, temperature float64) {
	if gate == nil || !finiteSurprise(surprise) {
		return
	}

	gate.lastSample = surprise
	alpha := AlphaFromSurprise(surprise, 0.01, 0.18)

	_ = gate.stream.Observe(surprise, alpha)
	_ = gate.changeSum.Observe(core.Float64(surprise))
}

func (gate *SurpriseGate) Threshold(temperature float64) float64 {
	if gate == nil {
		return warmupSurpriseThreshold(temperature)
	}

	sigma := surpriseWarmupSigma + surpriseTemperatureSpan*clampUnit(temperature)

	if gate.stream.Ready() {
		if bar, ok := gate.stream.Threshold(sigma); ok {
			cusumEvidence := float64(gate.changeSum.Observe(core.Float64(gate.lastSample)))

			return math.Max(bar, cusumEvidence)
		}
	}

	return warmupSurpriseThreshold(temperature)
}

func finiteSurprise(value float64) bool {
	return value > 0 && !math.IsNaN(value) && !math.IsInf(value, 0)
}

func warmupSurpriseThreshold(temperature float64) float64 {
	return 1.0 + surpriseTemperatureSpan*clampUnit(temperature)
}

/*
WarmupSurpriseThreshold is the macro surprise bar before per-source streams warm up.
*/
func WarmupSurpriseThreshold(temperature float64) float64 {
	return warmupSurpriseThreshold(temperature)
}

func loadSurpriseTemperature(slot *atomic.Uint64) float64 {
	return math.Float64frombits(slot.Load())
}

func clampUnit(value float64) float64 {
	if value < 0 {
		return 0
	}

	if value > 1 {
		return 1
	}

	return value
}

/*
SurpriseRegistry holds per-source adaptive surprise gates updated by Story.
*/
type SurpriseRegistry struct {
	gates       sync.Map
	temperature atomic.Uint64
}

var globalSurpriseRegistry = NewSurpriseRegistry()

func GlobalSurpriseRegistry() *SurpriseRegistry {
	return globalSurpriseRegistry
}

func NewSurpriseRegistry() *SurpriseRegistry {
	return &SurpriseRegistry{}
}

func (registry *SurpriseRegistry) SetTemperature(temperature float64) {
	if registry == nil {
		return
	}

	registry.temperature.Store(math.Float64bits(clampUnit(temperature)))
}

func (registry *SurpriseRegistry) Observe(source logic.SourceType, surprise float64) {
	if registry == nil || source == logic.SourceNone {
		return
	}

	registry.gate(source).Observe(surprise, loadSurpriseTemperature(&registry.temperature))
}

func (registry *SurpriseRegistry) Threshold(source logic.SourceType) float64 {
	temperature := 0.0

	if registry != nil {
		temperature = loadSurpriseTemperature(&registry.temperature)
	}

	if registry == nil || source == logic.SourceNone {
		return warmupSurpriseThreshold(temperature)
	}

	return registry.gate(source).Threshold(temperature)
}

func (registry *SurpriseRegistry) gate(source logic.SourceType) *SurpriseGate {
	raw, ok := registry.gates.Load(source)

	if ok {
		gate, gateOK := raw.(*SurpriseGate)

		if gateOK {
			return gate
		}
	}

	gate := newSurpriseGate()
	actual, _ := registry.gates.LoadOrStore(source, gate)

	gateStored, gateOK := actual.(*SurpriseGate)

	if gateOK {
		return gateStored
	}

	registry.gates.Store(source, gate)

	return gate
}
