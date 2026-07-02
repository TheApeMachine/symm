package pumpdump

import (
	"io"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/structure"
	"github.com/theapemachine/datura/transport"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique"
	"github.com/theapemachine/nomagique/algorithm"
	"github.com/theapemachine/nomagique/equation"
	"github.com/theapemachine/nomagique/probability"
	"github.com/theapemachine/symm/market"
)

type Trade struct {
	clock *structure.ClockRing[*datura.Artifact]
	algo  io.ReadWriteCloser
}

func NewTrade() *Trade {
	trade := &Trade{
		clock: structure.NewClockRing[*datura.Artifact](1, 1, 1),
	}

	trade.algo = nomagique.Number(
		algorithm.NewTradeFlowSample(datura.Acquire("pumpdump", datura.APPJSON)),
		equation.NewFlow(datura.Acquire(
			"pumpdump", datura.APPJSON,
		).WithAttributes(datura.Map[any]{
			"inputs": equation.FlowInputKeys,
		})),
		probability.NewClassifier(datura.Acquire(
			"pumpdump", datura.APPJSON,
		).WithAttributes(datura.Map[any]{
			"inputs": []string{
				"absorption",
				"drive",
				"balance",
				"starvation",
			},
		})),
	)

	return trade
}

func (trade *Trade) Measure(
	frame *datura.Artifact,
	crossSection *market.CrossSection,
) *datura.Artifact {
	if err := transport.NewFlipFlop(
		datura.NewRWCStream(frame), trade.algo,
	); err != nil {
		return frame.WithError(errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			err.Error(),
			err,
		)))
	}

	return frame
}
