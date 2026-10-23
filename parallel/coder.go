package parallel

import (
	"fmt"
	"sync"

	"github.com/theapemachine/symm/data"
	"github.com/theapemachine/symm/logic/resonance"
	"github.com/theapemachine/symm/nomagique/learning"
	"github.com/theapemachine/symm/nomagique/transport"
)

const physicsFeatureCount = 8

type coderPair struct {
	control   *learning.PredictiveCoder
	treatment *learning.PredictiveCoder
	scorer    *resonanceFeatureScorer
	step      int64
}

type CoderBank struct {
	mu    sync.Mutex
	pace  float64
	pairs map[string]*coderPair
}

func NewCoderBank(pace float64) *CoderBank {
	return &CoderBank{
		pace:  pace,
		pairs: map[string]*coderPair{},
	}
}

type CoderSide struct {
	Surprise     float64
	Calibrated   bool
	ForecastSign float64
	CurveMean    float64
	Skill        float64
	ForwardCurve []float64
}

type CoderReading struct {
	Control   CoderSide
	Treatment CoderSide
}

func (read CoderReading) ForBook(book BookID) CoderSide {
	if book == BookTreatment {
		return read.Treatment
	}

	return read.Control
}

func (bank *CoderBank) Step(
	symbol string,
	tick *data.Value,
	midpoint float64,
	physics []float64,
) (CoderReading, error) {
	if bank == nil || tick == nil {
		return CoderReading{}, fmt.Errorf("parallel: nil coder bank or tick")
	}

	bank.mu.Lock()
	pair := bank.pairs[symbol]
	if pair == nil {
		pair = newCoderPair(bank.pace, len(resonance.Features))
		bank.pairs[symbol] = pair
	}
	bank.mu.Unlock()

	features, err := pair.scorer.standardize(tick, symbol)
	if err != nil {
		return CoderReading{}, err
	}

	bank.mu.Lock()
	pair.step++
	step := pair.step
	control := pair.control
	treatment := pair.treatment
	bank.mu.Unlock()

	hasReference := 0.0
	if midpoint > 0 {
		hasReference = 1
	}

	controlOut := bank.runCoder(control, features, midpoint, hasReference, step, tick.Timestamp)
	treatFeatures := append(append([]float64{}, features...), physics...)
	treatOut := bank.runCoder(treatment, treatFeatures, midpoint, hasReference, step, tick.Timestamp)

	return CoderReading{
		Control:   sideReading(controlOut),
		Treatment: sideReading(treatOut),
	}, nil
}

func (bank *CoderBank) runCoder(
	coder *learning.PredictiveCoder,
	features []float64,
	midpoint, hasReference float64,
	step int64,
	timestamp uint64,
) [12][]float64 {
	if coder == nil {
		return [12][]float64{}
	}

	return transport.Read[[12][]float64](coder.Next(transport.NewValue([2][]float64{
		features,
		{midpoint, hasReference, float64(step), float64(timestamp) / 1e9},
	}).Next(nil)))
}

func sideReading(out [12][]float64) CoderSide {
	read := CoderSide{}

	if len(out[0]) > 5 {
		read.Surprise = out[0][5]
	}

	if len(out[10]) > 0 {
		read.Calibrated = out[10][1] > 0.5
	}

	if len(out[10]) > 2 && len(out[10]) > 3 {
		resolved := out[10][2]
		pending := out[10][3]
		if resolved+pending > 0 {
			read.Skill = resolved / (resolved + pending)
		}
	}

	curve := out[11]
	read.ForwardCurve = append([]float64{}, curve...)

	if len(curve) > 0 {
		sum := 0.0
		for _, v := range curve {
			sum += v
		}

		read.CurveMean = sum / float64(len(curve))

		pos, neg := 0, 0
		for _, v := range curve {
			if v > 0 {
				pos++
			} else if v < 0 {
				neg++
			}
		}

		if pos == len(curve) {
			read.ForecastSign = 1
		} else if neg == len(curve) {
			read.ForecastSign = -1
		}
	}

	return read
}

func newCoderPair(pace float64, featureDim int) *coderPair {
	controlPace := learning.NewPace(pace, pace*0.1, 0.15, 0.1, 0.2, 256)
	target := learning.NewDirectionalTarget(0)

	control := learning.NewPredictiveCoder(
		coderArch(featureDim), 8, target, controlPace, 0.05, true, learning.ReadoutAll,
	).(*learning.PredictiveCoder)

	treatDim := featureDim + physicsFeatureCount
	treat := learning.NewPredictiveCoder(
		coderArch(treatDim), 8, target, controlPace, 0.05, true, learning.ReadoutAll,
	).(*learning.PredictiveCoder)

	return &coderPair{
		control:   control,
		treatment: treat,
		scorer:    newResonanceFeatureScorer(),
	}
}

func coderArch(featureDim int) []int {
	return []int{featureDim, featureDim * 4, featureDim * 2, featureDim}
}
