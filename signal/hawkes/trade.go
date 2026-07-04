package hawkes

import (
	"io"
	"strconv"
	"time"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/structure"
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

	if err := nomagique.RoundTripArtifact(frame, trade.algo); err != nil {
		return frame.WithError(errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			err.Error(),
			err,
		)))
	}

	if datura.Peek[string](frame, "root") == "output" {
		if err := trade.classifier.Apply(frame); err != nil {
			return frame.WithError(errnie.Error(errnie.Err(
				errnie.UnprocessableContent,
				err.Error(),
				err,
			)))
		}
	}

	return completeMeasurement(frame)
}

func completeMeasurement(frame *datura.Artifact) *datura.Artifact {
	if datura.Peek[float64](frame, "output", "value") > 0 &&
		datura.Peek[float64](frame, "output", "confidence") > 0 &&
		datura.Peek[float64](frame, "output", "entry_baseline") > 0 &&
		datura.Peek[float64](frame, "output", "exit_baseline") > 0 {
		return frame
	}

	frenzy := logic.CategoryIndex(logic.CategoryFrenzy)
	saturation := logic.CategoryIndex(logic.CategorySaturation)
	organic := logic.CategoryIndex(logic.CategoryOrganic)
	exhaustion := logic.CategoryIndex(logic.CategoryExhaustion)
	baseline := 0.25

	frame.MergeOutputs(map[string]any{
		"frenzy":              datura.Peek[float64](frame, "output", "frenzy"),
		"saturation":          datura.Peek[float64](frame, "output", "saturation"),
		"organic":             datura.Peek[float64](frame, "output", "organic"),
		"exhaustion":          datura.Peek[float64](frame, "output", "exhaustion"),
		"probabilities":       []float64{baseline, baseline, baseline, baseline},
		"category":            float64(organic),
		"confidence":          baseline,
		"confidence_baseline": baseline,
		"distribution": map[string]float64{
			strconv.Itoa(frenzy):     baseline,
			strconv.Itoa(saturation): baseline,
			strconv.Itoa(organic):    baseline,
			strconv.Itoa(exhaustion): baseline,
		},
		"entry_baseline": baseline,
		"exit_baseline":  baseline,
		"strength":       datura.Peek[float64](frame, "output", "strength"),
		"value":          float64(organic),
	})
	frame.Poke("output", "root")
	frame.Poke([]string{
		"frenzy",
		"saturation",
		"organic",
		"exhaustion",
		"probabilities",
		"category",
		"confidence",
		"confidence_baseline",
		"distribution",
		"entry_baseline",
		"exit_baseline",
		"strength",
		"value",
	}, "inputs")

	return frame
}
