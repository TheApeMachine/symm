package tokenizer

import (
	"math"
	"sync"

	"github.com/theapemachine/symm/system"
)

const (
	EventLowVarianceFlux = iota // 5-Cycle
	EventDensitySpike           // 3-Cycle
	EventDensityTrough          // Inverse 3-Cycle
	EventPhaseInversion         // Double Transposition
)

type Calibrator struct {
	mu               sync.RWMutex
	sensitivityPop   float64
	sensitivityPhase float64
	window           *FastWindow
	phaseWindow      *FastWindow
}

type CalibratorOption func(*Calibrator)

func WithWindowSize(size int) CalibratorOption {
	return func(calibrator *Calibrator) {
		calibrator.window = NewFastWindow(size)
		calibrator.phaseWindow = NewFastWindow(size)
	}
}

func NewCalibrator(opts ...CalibratorOption) *Calibrator {
	calibrator := &Calibrator{
		sensitivityPop:   1.0,
		sensitivityPhase: 1.0,
		window:           NewFastWindow(system.Cfg.Learning.DefaultWindowSize),
		phaseWindow:      NewFastWindow(system.Cfg.Learning.DefaultWindowSize),
	}

	for _, opt := range opts {
		opt(calibrator)
	}

	return calibrator
}

func (calibrator *Calibrator) SensitivityPop() float64 {
	calibrator.mu.RLock()
	defer calibrator.mu.RUnlock()
	return calibrator.sensitivityPop
}

func (calibrator *Calibrator) SensitivityPhase() float64 {
	calibrator.mu.RLock()
	defer calibrator.mu.RUnlock()
	return calibrator.sensitivityPhase
}

func (calibrator *Calibrator) SetSensitivityPop(v float64) {
	calibrator.mu.Lock()
	defer calibrator.mu.Unlock()
	calibrator.sensitivityPop = v
}

func (calibrator *Calibrator) SetSensitivityPhase(v float64) {
	calibrator.mu.Lock()
	defer calibrator.mu.Unlock()
	calibrator.sensitivityPhase = v
}

/*
WindowStats returns (mean, stddev) for the density window.
Zero if window is nil or not warmed.
*/
func (calibrator *Calibrator) WindowStats() (mean, stddev float64) {
	calibrator.mu.RLock()
	defer calibrator.mu.RUnlock()

	if calibrator.window == nil || !calibrator.window.Warmed() {
		return 0, 0
	}

	return calibrator.window.Stats()
}

func (calibrator *Calibrator) FeedbackChunk(
	density float64,
	primeCoherence float64,
	phaseCoherence float64,
) {
	calibrator.mu.Lock()
	defer calibrator.mu.Unlock()

	if calibrator.window == nil {
		return
	}

	calibrator.window.Push(density)

	if !calibrator.window.Warmed() {
		return
	}

	meanDensity, stddev := calibrator.window.Stats()
	if meanDensity == 0 {
		return
	}

	targetDensity := system.Cfg.Learning.ShannonCapacity

	// Base error signal from density
	densityError := (targetDensity - meanDensity) / targetDensity

	// Invert coherence measurements (1.0 = highly coherent, 0.0 = totally incoherent)
	// We want to force splits when incoherence is high, and allow spans when coherence is high.
	incoherencePenalty := ((1.0 - primeCoherence) + (1.0 - phaseCoherence)) / 2.0

	// Shift the overall error signal towards earlier boundaries (more splits) when chunks are incoherent
	errorSignal := densityError - (incoherencePenalty * 0.5)

	lr := math.Max(stddev, 0.05)

	calibrator.sensitivityPop *= math.Exp(errorSignal * lr)
	calibrator.sensitivityPop = math.Max(
		0.01, math.Min(100.0, calibrator.sensitivityPop),
	)

	calibrator.sensitivityPhase *= math.Exp(errorSignal * lr)
	calibrator.sensitivityPhase = math.Max(
		0.01, math.Min(100.0, calibrator.sensitivityPhase),
	)
}

func (calibrator *Calibrator) ObservePhase(
	delta float64,
) {
	calibrator.mu.Lock()
	defer calibrator.mu.Unlock()

	if calibrator.phaseWindow == nil {
		return
	}

	calibrator.phaseWindow.Push(delta)
}

func (calibrator *Calibrator) DensityCeiling(
	fallback float64,
) float64 {
	calibrator.mu.RLock()
	defer calibrator.mu.RUnlock()

	return calibrator.dynamicLimit(
		calibrator.window, calibrator.sensitivityPop, fallback,
	)
}

func (calibrator *Calibrator) PhaseLimit(
	fallback float64,
) float64 {
	calibrator.mu.RLock()
	defer calibrator.mu.RUnlock()

	return calibrator.dynamicLimit(
		calibrator.phaseWindow,
		calibrator.sensitivityPhase,
		fallback,
	)
}

func (calibrator *Calibrator) dynamicLimit(
	window *FastWindow, sensitivity, fallback float64,
) float64 {
	if window == nil || !window.Warmed() {
		return fallback
	}

	mean, stddev := window.Stats()

	if mean <= 0 {
		return fallback
	}

	limit := mean + sensitivity*math.Max(stddev, 1e-9)

	if limit <= 0 {
		return fallback
	}

	return math.Min(fallback, limit)
}
