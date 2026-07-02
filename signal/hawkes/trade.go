package hawkes

import (
	"io"
	"time"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/structure"
	"github.com/theapemachine/datura/transport"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique"
	"github.com/theapemachine/nomagique/algorithm"
	"github.com/theapemachine/nomagique/probability"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/market"
)

type Trade struct {
	clock      *structure.ClockRing[*datura.Artifact]
	algo       io.ReadWriteCloser
	classifier *probability.Classifier
}

func NewTrade() *Trade {
	trade := &Trade{
		clock: structure.NewClockRing[*datura.Artifact](1, 1, 1),
	}

	trade.algo = nomagique.Number(
		algorithm.NewTradeExcitationSample(datura.Acquire("hawkes", datura.APPJSON)),
		algorithm.NewExcitation(datura.Acquire("hawkes", datura.APPJSON)),
	)
	trade.classifier = probability.NewClassifier(datura.Acquire(
		"hawkes", datura.APPJSON,
	).WithAttributes(datura.Map[any]{
		"inputs": []string{
			"frenzy",
			"saturation",
			"organic",
			"exhaustion",
		},
		"categoryIndexes": []float64{
			float64(logic.CategoryIndex(logic.CategoryFrenzy)),
			float64(logic.CategoryIndex(logic.CategorySaturation)),
			float64(logic.CategoryIndex(logic.CategoryOrganic)),
			float64(logic.CategoryIndex(logic.CategoryExhaustion)),
		},
	}))

	return trade
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

	if err := trade.classifier.Apply(frame); err != nil {
		return frame.WithError(errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			err.Error(),
			err,
		)))
	}

	return frame
}
