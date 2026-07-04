package causal

import (
	"time"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/structure"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/market"
)

type Ticker struct {
	clock  *structure.ClockRing[*datura.Artifact]
	engine *Engine
}

func NewTicker(engine *Engine) *Ticker {
	return &Ticker{
		clock:  structure.NewClockRing[*datura.Artifact](1, 1, 1),
		engine: engine,
	}
}

func (ticker *Ticker) Measure(
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

	return ticker.engine.MeasureTicker(frame)
}
