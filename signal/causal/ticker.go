package causal

import (
	"io"
	"time"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/structure"
	"github.com/theapemachine/datura/transport"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/market"
)

type Ticker struct {
	clock *structure.ClockRing[*datura.Artifact]
	algo  io.ReadWriteCloser
}

func NewTicker(algo io.ReadWriteCloser) *Ticker {
	return &Ticker{
		clock: structure.NewClockRing[*datura.Artifact](1, 1, 1),
		algo:  algo,
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

	if err := transport.NewFlipFlop(
		datura.NewRWCStream(frame), ticker.algo,
	); err != nil {
		return frame.WithError(errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			err.Error(),
			err,
		)))
	}

	errnie.Error(frame.SetOrigin(string(logic.SourceCausal)))

	if datura.Peek[string](frame, "root") == "output" {
		frame.MergeOutput("counterfactualReady", true)
		frame.Merge("counterfactual_ready", true)
	}

	return frame
}
