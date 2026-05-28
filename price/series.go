package price

import (
	"time"

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

	switch measurement.Type {
	case engine.Flow, engine.DepthFlow:
		return config.System.FlowHoldBeforeExit
	case engine.Causal:
		return config.System.MinHoldBeforeRotate
	default:
		return config.System.ScalpHoldBeforeExit
	}
}
