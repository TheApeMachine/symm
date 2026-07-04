package causal

import (
	"time"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/structure"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/market"
)

type Book struct {
	clock  *structure.ClockRing[*datura.Artifact]
	engine *Engine
}

func NewBook(engine *Engine) *Book {
	return &Book{
		clock:  structure.NewClockRing[*datura.Artifact](1, 1, 1),
		engine: engine,
	}
}

func (book *Book) Measure(
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

	return book.engine.MeasureBook(frame)
}
