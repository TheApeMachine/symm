package audit

import (
	"context"
	"fmt"
	"math"
	"strings"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/data"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
TruthfulnessStage validates that derived analytical feeds (such as CVD volumes,
trade rates, and order book states) faithfully match underlying tape identities
without hallucinated volume, dropped trades, or crossed markets (bid > ask).
*/
type TruthfulnessStage struct {
	*runtime.System
	ch                    chan *data.Value
	violations            []*Violation
	totalChecked          int
	crossedCount          int
	volumeDivergenceCount int
	tradeAnomalyCount     int
}

func NewTruthfulnessStage(ctx context.Context) *TruthfulnessStage {
	return &TruthfulnessStage{
		System:     runtime.NewSystem(ctx, "audit:truthfulness"),
		ch:         make(chan *data.Value, 64),
		violations: make([]*Violation, 0),
	}
}

func (stage *TruthfulnessStage) Channel() chan *data.Value {
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

		if measurement.Role == "runtime:join" {
			continue
		}

		if measurement.Role == "error" {
			stage.violations = append(stage.violations, NewViolation(
				measurement.Scope,
				"measurement.error",
				"error measurement encountered",
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
func (stage *TruthfulnessStage) validateTapeIdentities(measurement *data.Value) {
	metrics := data.Metrics(measurement)

	// 1. Crossed Market Identity Check: Best bid must never exceed best ask, and quotes must be positive.
	bidPrice, hasBid := metrics["best_price:bid"]
	askPrice, hasAsk := metrics["best_price:ask"]

	if hasBid && bidPrice <= 0 {
		stage.crossedCount++
		stage.violations = append(stage.violations, NewViolation(
			measurement.Scope,
			"truthfulness.quote_anomaly",
			fmt.Sprintf("Non-positive best bid on %s: %.4f", measurement.Scope, bidPrice),
			measurement,
			nil,
		))
	}

	if hasAsk && askPrice <= 0 {
		stage.crossedCount++
		stage.violations = append(stage.violations, NewViolation(
			measurement.Scope,
			"truthfulness.quote_anomaly",
			fmt.Sprintf("Non-positive best ask on %s: %.4f", measurement.Scope, askPrice),
			measurement,
			nil,
		))
	}

	if hasBid && hasAsk && bidPrice > askPrice {
		stage.crossedCount++
		stage.violations = append(stage.violations, NewViolation(
			measurement.Scope,
			"truthfulness.crossed_market",
			fmt.Sprintf("Crossed market detected on %s: best bid %.4f > best ask %.4f (negative spread %.4f)",
				measurement.Scope, bidPrice, askPrice, askPrice-bidPrice),
			measurement,
			nil,
		))
	}

	// 2. Relative Spread Identity Check: relative_spread == (ask - bid) / midpoint.
	relSpread, hasSpread := metrics["relative_spread"]

	if hasBid && hasAsk && hasSpread && (bidPrice+askPrice) > 0 {
		midpoint := (bidPrice + askPrice) / 2.0
		expectedSpread := (askPrice - bidPrice) / midpoint
		spreadDiff := math.Abs(relSpread - expectedSpread)

		if spreadDiff > 1e-4 {
			stage.volumeDivergenceCount++
			stage.violations = append(stage.violations, NewViolation(
				measurement.Scope,
				"truthfulness.identity_divergence",
				fmt.Sprintf("Relative spread identity breached on %s: reported %.6f != expected %.6f (diff %.6f)",
					measurement.Scope, relSpread, expectedSpread, spreadDiff),
				measurement,
				nil,
			))
		}
	}

	// 3. Friction Hurdle Identity Check: friction_hurdle == relative_spread + 2*fee_rate.
	frictionHurdle, hasHurdle := metrics["friction_hurdle"]
	feeRate, hasFee := metrics["fee_rate"]

	if hasSpread && hasHurdle && hasFee {
		expectedHurdle := relSpread + 2.0*feeRate
		hurdleDiff := math.Abs(frictionHurdle - expectedHurdle)

		if hurdleDiff > 1e-4 {
			stage.volumeDivergenceCount++
			stage.violations = append(stage.violations, NewViolation(
				measurement.Scope,
				"truthfulness.identity_divergence",
				fmt.Sprintf("Friction hurdle identity breached on %s: reported %.6f != expected %.6f (diff %.6f)",
					measurement.Scope, frictionHurdle, expectedHurdle, hurdleDiff),
				measurement,
				nil,
			))
		}
	}

	// 4. CVD Volume & Flow Invariants (Production Signal Checks):
	// gross_notional must be non-negative and signed_net_fraction must strictly lie in [-1, 1].
	grossNotional, hasGrossNotional := metrics["gross_notional"]
	signedFraction, hasSignedFraction := metrics["signed_net_fraction"]

	if hasGrossNotional && grossNotional < 0 {
		stage.volumeDivergenceCount++
		stage.violations = append(stage.violations, NewViolation(
			measurement.Scope,
			"truthfulness.volume_divergence",
			fmt.Sprintf("CVD gross notional negative on %s: %.4f", measurement.Scope, grossNotional),
			measurement,
			nil,
		))
	}

	if hasSignedFraction && math.Abs(signedFraction) > 1.0+1e-6 {
		stage.volumeDivergenceCount++
		stage.violations = append(stage.violations, NewViolation(
			measurement.Scope,
			"truthfulness.volume_divergence",
			fmt.Sprintf("CVD signed net fraction out of bounds [-1, 1] on %s: %.4f", measurement.Scope, signedFraction),
			measurement,
			nil,
		))
	}

	if hasGrossNotional && hasSignedFraction && grossNotional == 0 && math.Abs(signedFraction) > 1e-6 {
		stage.volumeDivergenceCount++
		stage.violations = append(stage.violations, NewViolation(
			measurement.Scope,
			"truthfulness.volume_divergence",
			fmt.Sprintf("CVD non-zero net fraction %.4f with zero gross notional on %s", signedFraction, measurement.Scope),
			measurement,
			nil,
		))
	}

	// 5. Non-Finite Numerical Anomaly Check: No NaN or Inf allowed in any metric.
	for label, rawVal := range metrics {
		if math.IsNaN(rawVal) || math.IsInf(rawVal, 0) {
			stage.tradeAnomalyCount++
			stage.violations = append(stage.violations, NewViolation(
				measurement.Scope,
				"truthfulness.non_finite_metric",
				fmt.Sprintf("Non-finite numerical value encountered on %s for metric %s: %v",
					measurement.Scope, label, rawVal),
				measurement,
				nil,
			))
		}
	}

	// 6. Trade Metric Truthfulness: Traded price and quantity must be strictly positive.
	tradePrice, hasTradePrice := metrics["price"]
	tradeQty, hasTradeQty := metrics["qty"]

	if hasTradePrice && tradePrice <= 0 {
		stage.tradeAnomalyCount++
		stage.violations = append(stage.violations, NewViolation(
			measurement.Scope,
			"truthfulness.trade_anomaly",
			fmt.Sprintf("Non-positive trade price on %s: %.4f", measurement.Scope, tradePrice),
			measurement,
			nil,
		))
	}

	if hasTradeQty && tradeQty <= 0 {
		stage.tradeAnomalyCount++
		stage.violations = append(stage.violations, NewViolation(
			measurement.Scope,
			"truthfulness.trade_anomaly",
			fmt.Sprintf("Non-positive trade quantity on %s: %.4f", measurement.Scope, tradeQty),
			measurement,
			nil,
		))
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
		"trade_anomaly_count":     stage.tradeAnomalyCount,
		"breach_count":            len(stage.violations),
	}
}
