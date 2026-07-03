package pumpdump

import (
	"io"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/structure"
	"github.com/theapemachine/datura/transport"
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
			"threshold": datura.Map[any]{"source": "spread"},
			"rvol": datura.Map[any]{
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
				"decline":     datura.Map[any]{"output": "rvolDecline"},
			},
			"precursor": datura.Map[any]{
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
			"spread": datura.Map[any]{
				"inputs": []string{"bid", "ask"},
			},
			"ignition": datura.Map[any]{
				"terms":     []string{"rvol", "precursor"},
				"source":    "ignition",
				"combine":   "ratio",
				"scale":     0.0,
				"leftKey":   "rvol",
				"rightKey":  "precursor",
				"scaleMode": "median",
			},
			"trend": datura.Map[any]{
				"terms":   []string{"precursor", "compression", "rvol"},
				"inverts": []string{"compression"},
			},
			"exhaustion": datura.Map[any]{
				"terms":   []string{"rvol", "precursor"},
				"inverts": []string{"rvol", "precursor"},
				"gate":    "rvolDecline",
			},
			"compression": datura.Map[any]{
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
			"decline": datura.Map[any]{
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
	if err := transport.NewFlipFlop(
		datura.NewRWCStream(frame), ticker.algo,
	); err != nil {
		return frame.WithError(errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			err.Error(),
			err,
		)))
	}

	return frame
}
