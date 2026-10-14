package audit

import (
	"context"
	"fmt"
	"math"
	"strings"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
TruthfulnessStage validates that derived analytical feeds (such as CVD volumes,
trade rates, and order book states) faithfully match underlying tape identities
without hallucinated volume, dropped trades, or crossed markets (bid > ask).
*/
type TruthfulnessStage struct {
	*runtime.System
	ch                    chan *data.Measurement
	violations            []*Violation
	totalChecked          int
	crossedCount          int
	volumeDivergenceCount int
}

func NewTruthfulnessStage(ctx context.Context) *TruthfulnessStage {
	return &TruthfulnessStage{
		System:     runtime.NewSystem(ctx, "audit:truthfulness"),
		ch:         make(chan *data.Measurement, 64),
		violations: make([]*Violation, 0),
	}
}

func (stage *TruthfulnessStage) Channel() chan *data.Measurement {
	return stage.ch
}

func (stage *TruthfulnessStage) Run() error {
	errnie.Info(fmt.Sprintf("%s: Running audit...", stage.Name()))

	for measurement := range stage.ch {
		stage.totalChecked++

		if measurement == nil {
			stage.violations = append(stage.violations, NewViolation(
				"unknown",
				"measurement",
				"Nil measurement encountered in tape",
				nil,
				nil,
			))
			continue
		}

		if measurement.Error() != nil {
			stage.violations = append(stage.violations, NewViolation(
				measurement.Label,
				"measurement.error",
				measurement.Error().Error(),
				measurement,
				nil,
			))
		}

		stage.validateTapeIdentities(measurement)
	}

	return nil
}

func (stage *TruthfulnessStage) Violations() []*Violation {
	return stage.violations
}

/*
validateTapeIdentities verifies physical and accounting identities on the measurement:
1. Bid price <= Ask price (no crossed market quotes).
2. Buy volume + Sell volume == Total volume (volume conservation identity).
*/
func (stage *TruthfulnessStage) validateTapeIdentities(measurement *data.Measurement) {
	metrics := make(map[string]float64)

	for entry := range measurement.Read() {
		if entry != nil && entry.Metric != nil {
			metrics[entry.Metric.Label()] = entry.Metric.Raw()
		}
	}

	// 1. Crossed Market Identity Check: Best bid must never exceed best ask.
	bidPrice, hasBid := metrics["best_bid_price"]
	askPrice, hasAsk := metrics["best_ask_price"]

	if hasBid && hasAsk && bidPrice > askPrice {
		stage.crossedCount++
		stage.violations = append(stage.violations, NewViolation(
			measurement.Label,
			"truthfulness.crossed_market",
			fmt.Sprintf("Crossed market detected on %s: best bid %.4f > best ask %.4f (negative spread %.4f)",
				measurement.Label, bidPrice, askPrice, askPrice-bidPrice),
			measurement,
			nil,
		))
	}

	// 2. Volume Conservation Identity Check: Gross volume == Buy volume + Sell volume.
	grossVol, hasGross := metrics["gross_volume"]
	buyVol, hasBuy := metrics["buy_volume"]
	sellVol, hasSell := metrics["sell_volume"]

	if hasGross && hasBuy && hasSell {
		expectedGross := buyVol + sellVol
		diff := math.Abs(grossVol - expectedGross)

		if diff > core.Resolution*math.Max(grossVol, expectedGross) {
			stage.volumeDivergenceCount++
			stage.violations = append(stage.violations, NewViolation(
				measurement.Label,
				"truthfulness.volume_divergence",
				fmt.Sprintf("Volume conservation breached: gross %.4f != buy %.4f + sell %.4f (diff %.4f)",
					grossVol, buyVol, sellVol, diff),
				measurement,
				nil,
			))
		}
	}
}

func (stage *TruthfulnessStage) Report() string {
	var build strings.Builder

	for _, violation := range stage.violations {
		build.WriteString(violation.String())
	}

	return build.String()
}

func (stage *TruthfulnessStage) Data() any {
	return map[string]any{
		"passed":                  len(stage.violations) == 0,
		"total_checked":           stage.totalChecked,
		"crossed_market_count":    stage.crossedCount,
		"volume_divergence_count": stage.volumeDivergenceCount,
		"breach_count":            len(stage.violations),
	}
}

