package strategy

import (
	"sync"

	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/data"
)

/*
Momentum watches open lots in profit and exits on the first tick where book
return, forward curve, or supported horizon stops improving.
*/
type Momentum struct {
	tracks *sync.Map
}

func NewMomentum() *Momentum {
	return &Momentum{
		tracks: &sync.Map{},
	}
}

type momentumSample struct {
	seeded        bool
	lastForward   float64
	lastPnLPct    float64
	lastSupported int
	peakSupported int
	curveAligned  bool
}

func (momentum *Momentum) track(position *broker.Position) *momentumSample {
	if position == nil || position.ID == "" {
		return &momentumSample{}
	}

	stored, _ := momentum.tracks.LoadOrStore(position.ID, &momentumSample{})
	return stored.(*momentumSample)
}

func (momentum *Momentum) forget(position *broker.Position) {
	if position == nil || position.ID == "" {
		return
	}

	momentum.tracks.Delete(position.ID)
}

func (momentum *Momentum) pnlPct(position *broker.Position) (float64, bool) {
	if position == nil || position.PnLPct == nil {
		return 0, false
	}

	return position.PnLPct.Float64(), true
}

func (momentum *Momentum) trigger(
	resonance *Resonance,
	tick *data.Value,
	symbol string,
	position *broker.Position,
) (string, bool) {
	if tick == nil || position == nil || !position.Winning() {
		return "", false
	}

	sample := momentum.track(position)
	
	if !sample.seeded {
		return "", false
	}

	pnlPct, hasPct := momentum.pnlPct(position)

	if !hasPct || pnlPct <= 0 {
		return "", false
	}

	if pnlPct < sample.lastPnLPct {
		return "momentum reversal", true
	}

	if resonance == nil {
		return "", false
	}

	reading := resonance.Read(tick, symbol)

	if reading.Calibrated < 1 {
		return "", false
	}

	meanForward := resonance.MeanForward(reading)

	if meanForward < sample.lastForward {
		return "momentum fade", true
	}

	aligned := resonance.ForwardAligned(reading)

	if sample.curveAligned && !aligned {
		return "momentum exhaust", true
	}

	supported := int(reading.SupportedHorizon)

	if supported < sample.lastSupported {
		return "momentum stall", true
	}

	return "", false
}

func (momentum *Momentum) advance(
	resonance *Resonance,
	tick *data.Value,
	symbol string,
	position *broker.Position,
) {
	if tick == nil || position == nil {
		return
	}

	sample := momentum.track(position)
	pnlPct, hasPct := momentum.pnlPct(position)

	var (
		reading     Reading
		meanForward float64
		supported   int
		aligned     bool
	)

	if resonance != nil {
		reading = resonance.Read(tick, symbol)
		meanForward = resonance.MeanForward(reading)
		supported = int(reading.SupportedHorizon)
		aligned = resonance.ForwardAligned(reading)
	}

	if !hasPct || pnlPct <= 0 {
		return
	}

	if !sample.seeded {
		sample.lastPnLPct = pnlPct

		if reading.Calibrated >= 1 {
			sample.lastForward = meanForward
			sample.lastSupported = supported
			sample.peakSupported = supported
			sample.curveAligned = aligned
		}

		sample.seeded = true
		return
	}

	if supported > sample.peakSupported {
		sample.peakSupported = supported
	}

	sample.lastPnLPct = pnlPct

	if reading.Calibrated >= 1 {
		sample.lastForward = meanForward
		sample.lastSupported = supported
		sample.curveAligned = aligned
	}
}
