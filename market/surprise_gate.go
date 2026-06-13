package market

import (
	"math"
	"sync"

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
	temperature atomicTemperature
}

type atomicTemperature struct {
	value float64
	mu    sync.RWMutex
}

func (holder *atomicTemperature) Store(temperature float64) {
	holder.mu.Lock()
	holder.value = clampUnit(temperature)
	holder.mu.Unlock()
}

func (holder *atomicTemperature) Load() float64 {
	holder.mu.RLock()
	defer holder.mu.RUnlock()

	return holder.value
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

	registry.temperature.Store(temperature)
}

func (registry *SurpriseRegistry) Observe(source logic.SourceType, surprise float64) {
	if registry == nil || source == logic.SourceNone {
		return
	}

	registry.gate(source).Observe(surprise, registry.temperature.Load())
}

func (registry *SurpriseRegistry) Threshold(source logic.SourceType) float64 {
	temperature := 0.0

	if registry != nil {
		temperature = registry.temperature.Load()
	}

	if registry == nil || source == logic.SourceNone {
		return warmupSurpriseThreshold(temperature)
	}

	return registry.gate(source).Threshold(temperature)
}

func (registry *SurpriseRegistry) gate(source logic.SourceType) *SurpriseGate {
	raw, ok := registry.gates.Load(source)

	if ok {
		return raw.(*SurpriseGate)
	}

	gate := newSurpriseGate()
	actual, _ := registry.gates.LoadOrStore(source, gate)

	return actual.(*SurpriseGate)
}
