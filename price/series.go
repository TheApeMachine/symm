package price

import (
	"fmt"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/engine"
	"github.com/theapemachine/symm/numeric"
	"github.com/theapemachine/symm/numeric/adaptive"
)

func (prediction *Prediction) returnSeries(key predictionSeriesKey) *numeric.Derived {
	returns := prediction.returns[key]

	if returns == nil {
		returns = numeric.NewDerived(numeric.WithDynamics(adaptive.NewEMA(0)))
		prediction.returns[key] = returns
	}

	return returns
}

func (prediction *Prediction) marketMove(symbol string) *numeric.Derived {
	move := prediction.marketMoves[symbol]

	if move == nil {
		move = numeric.NewDerived(numeric.WithDynamics(adaptive.NewEMA(0)))
		prediction.marketMoves[symbol] = move
	}

	return move
}

func measurementDirection(measurement engine.Measurement) int {
	if measurement.Type == engine.Dump {
		return -1
	}

	return 1
}

func measurementRunway(measurement engine.Measurement) time.Duration {
	if measurement.Timeframe.End > measurement.Timeframe.Start {
		return time.Duration(
			measurement.Timeframe.End-measurement.Timeframe.Start,
		) * time.Second
	}

	if measurement.Timeframe.End < measurement.Timeframe.Start {
		errnie.Info(fmt.Sprintf(
			"warning: invalid measurement timeframe source=%s reason=%s start=%d end=%d",
			measurement.Source,
			measurement.Reason,
			measurement.Timeframe.Start,
			measurement.Timeframe.End,
		))
	}

	switch measurement.Type {
	case engine.Flow, engine.DepthFlow:
		return config.System.FlowHoldBeforeExit
	case engine.Causal:
		return config.System.MinHoldBeforeRotate
	default:
		return config.System.ScalpHoldBeforeExit
	}
}
