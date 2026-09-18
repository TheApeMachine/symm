package liquidity

import (
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	sequence "github.com/theapemachine/symm/nomagique/data/sequence"
	"github.com/theapemachine/symm/nomagique/statistic"
	"github.com/theapemachine/symm/nomagique/temporal"
)

/*
Reading is one touch morphology observation.
*/
type Reading struct {
	Symbol                                     string
	Bid, Ask, BidQty, AskQty                   float64
	Midpoint, Spread, Relative                 float64
	BidNotional, AskNotional                   float64
	TwoSided, Imbalance                        float64
	BidBaseline, AskBaseline, SpreadBaseline   float64
	BidRatio, AskRatio, SpreadRatio            float64
	BidDivergence, AskDivergence, SpreadDiv    float64
	BidNoise, AskNoise, SpreadNoise            float64
	BidZ, AskZ, SpreadZ                        float64
	BidVelocity, AskVelocity, SpreadVelocity   float64
	BidVelSNR, AskVelSNR, SpreadVelSNR         float64
	HasBaseline                                bool
}

type path struct {
	estimator core.Primitive
	velocity  [3]core.Primitive
	at        int64
}

/*
Touch measures one quoted arrival's touch morphology.
*/
type Touch struct {
	*core.PrimitiveError

	paths map[string]*path
	out   Reading
}

func NewTouch() *Touch {
	return &Touch{
		PrimitiveError: core.NewPrimitiveError(),
		paths:          make(map[string]*path),
	}
}

func (touch *Touch) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			quote := *(*Quote)(arriving)

			if quote.Bid <= 0 || quote.Ask <= 0 {
				continue
			}

			state := touch.paths[quote.Symbol]

			if state == nil {
				state = &path{
					estimator: statistic.NewJoint(3),
					velocity: [3]core.Primitive{
						statistic.NewLocalRegression(),
						statistic.NewLocalRegression(),
						statistic.NewLocalRegression(),
					},
				}
				touch.paths[quote.Symbol] = state
			}

			if state.at != 0 && quote.At < state.at {
				continue
			}

			bidNotional := quote.Bid * quote.BidQty
			askNotional := quote.Ask * quote.AskQty
			midpoint := (quote.Bid + quote.Ask) / 2
			spread := quote.Ask - quote.Bid
			relative := spread / midpoint
			imbalance := 0.0

			if bidNotional+askNotional > 0 {
				imbalance = (bidNotional - askNotional) / (bidNotional + askNotional)
			}

			reading := Reading{
				Symbol:      quote.Symbol,
				Bid:         quote.Bid,
				Ask:         quote.Ask,
				BidQty:      quote.BidQty,
				AskQty:      quote.AskQty,
				Midpoint:    midpoint,
				Spread:      spread,
				Relative:    relative,
				BidNotional: bidNotional,
				AskNotional: askNotional,
				TwoSided:    math.Min(bidNotional, askNotional),
				Imbalance:   imbalance,
			}

			logged := []float64{math.Log(bidNotional), math.Log(askNotional), math.Log(relative)}
			originals := []float64{bidNotional, askNotional, relative}
			var joint statistic.JointReading
			input := statistic.JointInput{Values: logged}

			for out := range state.estimator.Next(sequence.NewOne(unsafe.Pointer(&input)).Next(nil)) {
				joint = *(*statistic.JointReading)(out)
			}

			if err := state.estimator.Error(); err != nil {
				touch.Error(err)
				return
			}

			targets := []*float64{
				&reading.BidBaseline, &reading.AskBaseline, &reading.SpreadBaseline,
			}
			ratios := []*float64{&reading.BidRatio, &reading.AskRatio, &reading.SpreadRatio}
			divs := []*float64{&reading.BidDivergence, &reading.AskDivergence, &reading.SpreadDiv}
			noises := []*float64{&reading.BidNoise, &reading.AskNoise, &reading.SpreadNoise}
			zs := []*float64{&reading.BidZ, &reading.AskZ, &reading.SpreadZ}
			vels := []*float64{&reading.BidVelocity, &reading.AskVelocity, &reading.SpreadVelocity}
			snrs := []*float64{&reading.BidVelSNR, &reading.AskVelSNR, &reading.SpreadVelSNR}

			for index := range joint.Channels {
				channel := joint.Channels[index]

				if !channel.HasPrior {
					continue
				}

				reading.HasBaseline = true
				*targets[index] = channel.Baseline
				*ratios[index] = originals[index] / channel.Baseline
				*divs[index] = channel.Residual

				if channel.ScoreScale > 0 {
					*noises[index] = channel.ScoreScale
					*zs[index] = channel.ZScore
				}

				observation := temporal.Price{At: quote.At, Value: channel.Residual}
				var summary statistic.LocalRegressionReading

				for out := range state.velocity[index].Next(sequence.NewOne(unsafe.Pointer(&observation)).Next(nil)) {
					summary = *(*statistic.LocalRegressionReading)(out)
				}

				if err := state.velocity[index].Error(); err != nil {
					touch.Error(err)
					return
				}

				if summary.SlopeDefined {
					*vels[index] = summary.Slope
				}

				if summary.SNRDefined {
					*snrs[index] = summary.SNR
				}
			}

			state.at = quote.At
			touch.out = reading

			if !yield(unsafe.Pointer(&touch.out)) {
				return
			}
		}
	}
}
