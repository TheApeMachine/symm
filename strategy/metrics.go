package strategy

import (
	"io"
	"sync/atomic"

	"github.com/theapemachine/symm/data"
)

/*
TickMetrics is one workspace tick's aggregate strategy measurements for the
Longtail dashboard. It rolls up per-symbol gate and path-drift readings.
*/
type TickMetrics struct {
	Symbols          int     `json:"symbols"`
	GatePass         int     `json:"gatePass"`
	GateTotal        int     `json:"gateTotal"`
	FrictionPass     int     `json:"frictionPass"`
	FrictionTotal    int     `json:"frictionTotal"`
	PathDrift        int     `json:"pathDrift"`
	PathCompared     int     `json:"pathCompared"`
	CanonicalTicks   int     `json:"canonicalTicks"`
	PolicyMatches    int     `json:"policyMatches"`
	PolicyChecks     int     `json:"policyChecks"`
	ExploredEnter    int     `json:"exploredEnter"`
	ConfirmedEnter   int     `json:"confirmedEnter"`
	VenueOpen        int     `json:"venueOpen"`
	VirtualExcursion int     `json:"virtualExcursion"`
	MeanMaturity     float64 `json:"meanMaturity"`
	MeanSurprise     float64 `json:"meanSurprise"`
}

type metricsAccumulator struct {
	snapshot atomic.Value
}

func newMetricsAccumulator() *metricsAccumulator {
	acc := &metricsAccumulator{}
	acc.snapshot.Store(TickMetrics{})
	return acc
}

func (acc *metricsAccumulator) reset() TickMetrics {
	if acc == nil {
		return TickMetrics{}
	}

	empty := TickMetrics{}
	acc.snapshot.Store(empty)
	return empty
}

func (acc *metricsAccumulator) add(delta TickMetrics) {
	if acc == nil {
		return
	}

	current, _ := acc.snapshot.Load().(TickMetrics)
	current.Symbols += delta.Symbols
	current.GatePass += delta.GatePass
	current.GateTotal += delta.GateTotal
	current.FrictionPass += delta.FrictionPass
	current.FrictionTotal += delta.FrictionTotal
	current.PathDrift += delta.PathDrift
	current.PathCompared += delta.PathCompared
	current.CanonicalTicks += delta.CanonicalTicks
	current.PolicyMatches += delta.PolicyMatches
	current.PolicyChecks += delta.PolicyChecks
	current.ExploredEnter += delta.ExploredEnter
	current.ConfirmedEnter += delta.ConfirmedEnter
	current.VenueOpen += delta.VenueOpen
	current.VirtualExcursion += delta.VirtualExcursion

	if delta.Symbols > 0 {
		n := float64(delta.Symbols)
		total := float64(current.Symbols)
		prev := total - n
		if total > 0 {
			current.MeanMaturity = (current.MeanMaturity*prev + delta.MeanMaturity*n) / total
			current.MeanSurprise = (current.MeanSurprise*prev + delta.MeanSurprise*n) / total
		}
	}

	acc.snapshot.Store(current)
}

func (acc *metricsAccumulator) publish(hub io.Writer) {
	if acc == nil || hub == nil {
		return
	}

	snap, _ := acc.snapshot.Load().(TickMetrics)
	value := data.NewValue("training", "longtail", "aggregate")
	value.Append(snap, "longtail")
	io.Copy(hub, value)
}
