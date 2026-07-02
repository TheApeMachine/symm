package exhaust

import (
	"io"
	"time"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/structure"
	"github.com/theapemachine/datura/transport"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/probability"
	"github.com/theapemachine/symm/market"
)

type Trade struct {
	clock      *structure.ClockRing[*datura.Artifact]
	algo       io.ReadWriteCloser
	classifier *probability.Classifier
}

func NewTrade(algo io.ReadWriteCloser, classifier *probability.Classifier) *Trade {
	return &Trade{
		clock:      structure.NewClockRing[*datura.Artifact](1, 1, 1),
		algo:       algo,
		classifier: classifier,
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

	if err := transport.NewFlipFlop(
		datura.NewRWCStream(frame), trade.algo,
	); err != nil {
		return frame.WithError(errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			err.Error(),
			err,
		)))
	}

	if datura.Peek[string](frame, "root") != "output" {
		return frame
	}

	if datura.Peek[float64](frame, "output", "category") <= 0 {
		return frame
	}

	if err := trade.classifier.Apply(frame); err != nil {
		return frame.WithError(errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			err.Error(),
			err,
		)))
	}

	return frame
}
