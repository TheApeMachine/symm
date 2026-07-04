package pumpdump

import (
	"io"
	"strconv"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/structure"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique"
	"github.com/theapemachine/nomagique/equation"
	"github.com/theapemachine/nomagique/probability"
	"github.com/theapemachine/nomagique/vector"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/market"
)

type Ticker struct {
	clock *structure.ClockRing[*datura.Artifact]
	algo  io.ReadWriteCloser
}

func NewTicker() *Ticker {
	ticker := &Ticker{
		clock: structure.NewClockRing[*datura.Artifact](1, 1, 1),
	}

	ticker.algo = nomagique.Number(
		vector.NewFeatureExtractor(datura.Acquire(
			"pumpdump", datura.APPJSON,
		).WithAttributes(datura.Map[any]{
			"root": ".",
			"inputs": []string{
				"volume",
				"last",
				"bid",
				"ask",
			},
		})),
		equation.NewIgnition(datura.Acquire(
			"pumpdump", datura.APPJSON,
		).WithAttributes(datura.Map[any]{
			"order":     []string{"rvol", "precursor", "compression"},
			"outputs":   []string{"ignition", "compression", "trend", "exhaustion"},
			"threshold": map[string]any{"source": "spread"},
			"rvol": map[string]any{
				"input":       "volume",
				"transform":   "deltaPositive",
				"shortWindow": 0.0,
				"longWindow":  0.0,
				"outputKey":   "rvol",
				"scale":       0.0,
				"scaleMode":   "median",
				"centerMode":  "median",
				"leftKey":     "rvol",
				"rightKey":    "precursor",
				"decline":     map[string]any{"output": "rvolDecline"},
			},
			"precursor": map[string]any{
				"input":        "last",
				"returnLag":    0.0,
				"longWindow":   0.0,
				"positiveOnly": 1.0,
				"outputKey":    "precursor",
				"stageIndex":   1.0,
				"scale":        0.0,
				"scaleMode":    "median",
				"leftKey":      "rvol",
				"rightKey":     "precursor",
			},
			"spread": map[string]any{
				"inputs": []string{"bid", "ask"},
			},
			"ignition": map[string]any{
				"terms":     []string{"rvol", "precursor"},
				"source":    "ignition",
				"combine":   "ratio",
				"scale":     0.0,
				"leftKey":   "rvol",
				"rightKey":  "precursor",
				"scaleMode": "median",
			},
			"trend": map[string]any{
				"terms":   []string{"precursor", "compression", "rvol"},
				"inverts": []string{"compression"},
			},
			"exhaustion": map[string]any{
				"terms":   []string{"rvol", "precursor"},
				"inverts": []string{"rvol", "precursor"},
				"gate":    "rvolDecline",
			},
			"compression": map[string]any{
				"input":      "spread",
				"outputKey":  "compression",
				"scale":      0.0,
				"scaleMode":  "median",
				"terms":      []string{"compression", "precursor", "rvol"},
				"inverts":    []string{"precursor", "rvol"},
				"gate":       "precursor",
				"gateInvert": 1.0,
				"leftKey":    "rvol",
				"rightKey":   "precursor",
			},
			"decline": map[string]any{
				"source":    "rvolDecline",
				"output":    "exhaustion",
				"squash":    0.0,
				"attenuate": []string{"compression"},
			},
		})),
		probability.NewClassifier(datura.Acquire(
			"pumpdump", datura.APPJSON,
		).WithAttributes(datura.Map[any]{
			"inputs": []string{
				"ignition",
				"compression",
				"trend",
				"exhaustion",
			},
			"categoryIndexes": []float64{
				float64(logic.CategoryIndex(logic.CategoryVerticalIgnition)),
				float64(logic.CategoryIndex(logic.CategoryCoiledCompression)),
				float64(logic.CategoryIndex(logic.CategoryOrganicTrend)),
				float64(logic.CategoryIndex(logic.CategoryFadedExhaustion)),
			},
		})),
	)

	return ticker
}

func (ticker *Ticker) Measure(
	frame *datura.Artifact, crossSection *market.CrossSection,
) *datura.Artifact {
	if err := nomagique.RoundTripArtifact(frame, ticker.algo); err != nil {
		errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			err.Error(),
			err,
		))
	}

	if datura.Peek[float64](frame, "output", "value") > 0 &&
		datura.Peek[float64](frame, "output", "confidence") > 0 &&
		datura.Peek[float64](frame, "output", "entry_baseline") > 0 &&
		datura.Peek[float64](frame, "output", "exit_baseline") > 0 {
		return frame
	}

	ignition := logic.CategoryIndex(logic.CategoryVerticalIgnition)
	compression := logic.CategoryIndex(logic.CategoryCoiledCompression)
	trend := logic.CategoryIndex(logic.CategoryOrganicTrend)
	exhaustion := logic.CategoryIndex(logic.CategoryFadedExhaustion)
	baseline := 0.25

	frame.MergeOutputs(map[string]any{
		"ignition":            datura.Peek[float64](frame, "output", "ignition"),
		"compression":         datura.Peek[float64](frame, "output", "compression"),
		"trend":               datura.Peek[float64](frame, "output", "trend"),
		"exhaustion":          datura.Peek[float64](frame, "output", "exhaustion"),
		"probabilities":       []float64{baseline, baseline, baseline, baseline},
		"category":            float64(trend),
		"confidence":          baseline,
		"confidence_baseline": baseline,
		"distribution": map[string]float64{
			strconv.Itoa(ignition):    baseline,
			strconv.Itoa(compression): baseline,
			strconv.Itoa(trend):       baseline,
			strconv.Itoa(exhaustion):  baseline,
		},
		"entry_baseline": baseline,
		"exit_baseline":  baseline,
		"strength":       datura.Peek[float64](frame, "output", "strength"),
		"value":          float64(trend),
	})
	frame.Poke("output", "root")
	frame.Poke([]string{"volume", "last", "bid", "ask"}, "sourceInputs")
	frame.Poke([]string{
		"ignition",
		"compression",
		"trend",
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
