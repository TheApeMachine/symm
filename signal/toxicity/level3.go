package toxicity

import (
	"time"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/structure"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/market"
)

type Level3 struct {
	clock  *structure.ClockRing[*datura.Artifact]
	engine *Engine
}

func NewLevel3(engine *Engine) *Level3 {
	return &Level3{
		clock:  structure.NewClockRing[*datura.Artifact](1, 1, 1),
		engine: engine,
	}
}

func (level3 *Level3) Measure(
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

	errnie.Error(frame.SetOrigin(string(logic.SourceToxicity)))

	return level3.engine.MeasureLevel3(frame)
}
