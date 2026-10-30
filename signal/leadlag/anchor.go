package leadlag

import (
	"time"

	nmcorrelation "github.com/theapemachine/nomagique/correlation"
	"github.com/theapemachine/nomagique/data"
	"github.com/theapemachine/nomagique/runtime"
)

const anchorBreakdownWindow = 10 * time.Second
const anchorBreakdownThreshold = 0.005

var anchorSymbols = []string{
	"BTC/USD", "XBT/USD", "XXBT/USD",
	"ETH/USD", "XETH/USD",
}

func (signal *Signal) appendAnchorBreakdown(value *data.Value) {
	if signal.Status() != runtime.READY || value == nil {
		return
	}

	now := time.Unix(0, int64(value.Timestamp))
	breakdown := 0.0

	for _, anchor := range anchorSymbols {
		untyped, ok := signal.paths.Load(anchor)
		if !ok {
			continue
		}

		path := untyped.([]nmcorrelation.Sample)
		ret, ok := returnOver(path, now, anchorBreakdownWindow)
		if !ok || ret >= -anchorBreakdownThreshold {
			continue
		}

		stress := (-ret - anchorBreakdownThreshold) / anchorBreakdownThreshold
		if stress > breakdown {
			breakdown = stress
		}
	}

	if breakdown <= 0 {
		return
	}

	value.Append([]float64{breakdown}, append([]string{
		signal.Name(),
		"aggregate",
	}, "anchor_breakdown")...)
}

func returnOver(path []nmcorrelation.Sample, now time.Time, window time.Duration) (float64, bool) {
	if len(path) == 0 {
		return 0, false
	}

	from := now.Add(-window)
	var atPrice, fromPrice float64
	hasAt, hasFrom := false, false

	for index := len(path) - 1; index >= 0; index-- {
		sample := path[index]
		if !hasAt && !sample.At.After(now) {
			atPrice = sample.Value
			hasAt = true
		}
		if !hasFrom && !sample.At.After(from) {
			fromPrice = sample.Value
			hasFrom = true
		}
		if hasAt && hasFrom {
			break
		}
	}

	if !hasAt || !hasFrom || fromPrice <= 0 || atPrice <= 0 {
		return 0, false
	}

	return atPrice/fromPrice - 1, true
}
