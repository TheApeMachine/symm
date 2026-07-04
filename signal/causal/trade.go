package causal

import (
	"time"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/structure"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/market"
)

type Trade struct {
	clock  *structure.ClockRing[*datura.Artifact]
	engine *Engine
}

func NewTrade(engine *Engine) *Trade {
	return &Trade{
		clock:  structure.NewClockRing[*datura.Artifact](1, 1, 1),
		engine: engine,
	}
}

func (trade *Trade) Measure(
	frame *datura.Artifact,
	crossSection *market.CrossSection,
) *datura.Artifact {
	if observed := datura.Peek[string](frame, "timestamp"); observed != "" {
		stamp, err := time.Parse(time.RFC3339Nano, observed)

		if err != nil {
			return frame.WithError(errnie.Error(errnie.Err(
				errnie.UnprocessableContent,
				err.Error(),
				err,
			)))
		}

		frame.SetTimestamp(stamp.UnixNano())
	}

	return trade.engine.MeasureTrade(frame)
}
