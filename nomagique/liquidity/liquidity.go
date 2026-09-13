package liquidity

import (
	"errors"
	"fmt"
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/logic"
	"github.com/theapemachine/symm/nomagique/statistic"
	"github.com/theapemachine/symm/nomagique/temporal"
	"github.com/theapemachine/symm/nomagique/transport"
)

/*
drive pushes one payload pointer through one primitive and returns the
answer the primitive yielded.
*/
func drive[From, To any](op core.Primitive, payload *From) To {
	var answer To

	for out := range op.Next(transport.NewOne(unsafe.Pointer(payload)).Next(nil)) {
		answer = *(*To)(out)
	}

	return answer
}

/*
Gate classifies the arrival: it reads the touch quote the feed wrote, and
stamps the measurement's support baseline. Anything invalid fails the
measurement here and never reaches the touch. The metadata baseline is
rewritten on every arrival, so the measurement carries this arrival's facts,
never the prior one's.
*/
type Gate struct {
	err    error
	finite core.Primitive
}

func NewGate() core.Primitive {
	return &Gate{finite: logic.NewFinite()}
}

func (op *Gate) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			m := *(**data.Measurement[float64])(arriving)

			bid, ask := m.Metrics["bid"].Raw, m.Metrics["ask"].Raw
			bidQty, askQty := m.Metrics["bid_qty"].Raw, m.Metrics["ask_qty"].Raw

			m.Metadata = map[string]float64{data.MetadataSupport: 0}

			if bid == 0 || ask == 0 {
				m.Err = fmt.Errorf("%w: liquidity: ticker requires bid and ask", core.ErrDomain)

				if !yield(arriving) {
					return
				}

				continue
			}

			finite := true

			for _, value := range []float64{bid, ask, bidQty, askQty, bid * bidQty, ask * askQty} {
				probe := value

				if holds := drive[float64, bool](op.finite, &probe); !holds || probe <= 0 {
					finite = false
				}
			}

			if !finite {
				m.Err = fmt.Errorf("%w: liquidity: finite positive prices and displayed quantities required", core.ErrDomain)

				if !yield(arriving) {
					return
				}

				continue
			}

			if ask <= bid {
				m.Err = fmt.Errorf("%w: liquidity: positive order violated (%f <= %f)", core.ErrDomain, ask, bid)

				if !yield(arriving) {
					return
				}

				continue
			}

			if !yield(arriving) {
				return
			}
		}
	}
}

func (op *Gate) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}

/*
path is one symbol's touch morphology state: the joint log-moment estimator
over bid, ask, and spread channels, and one causal local regression per
divergence channel.
*/
type path struct {
	estimator core.Primitive
	velocity  [3]core.Primitive
	at        int64
}

var (
	velocityLabels    = []string{"divergence_velocity:bid", "divergence_velocity:ask", "spread_divergence_velocity"}
	velocitySNRLabels = []string{"divergence_velocity_snr:bid", "divergence_velocity_snr:ask", "spread_divergence_velocity_snr"}
	baselineLabels    = []string{"touch_notional_baseline:bid", "touch_notional_baseline:ask", "relative_spread_baseline"}
	ratioLabels       = []string{"depth_ratio:bid", "depth_ratio:ask", "spread_ratio"}
	divergenceLabels  = []string{"depth_divergence:bid", "depth_divergence:ask", "spread_divergence"}
	noiseLabels       = []string{"depth_noise_scale:bid", "depth_noise_scale:ask", "spread_noise_scale"}
	zscoreLabels      = []string{"depth_zscore:bid", "depth_zscore:ask", "spread_zscore"}
)

/*
Touch measures one quoted arrival's touch morphology: notionals, midpoint,
spread, and the causal baseline, divergence, noise, z-score, and divergence
velocity of each channel. Every fact is written where it is computed, and
facts whose evidence is immature stay unwritten.
*/
type Touch struct {
	err   error
	paths map[string]*path
}

func NewTouch() core.Primitive {
	return &Touch{paths: make(map[string]*path)}
}

func (op *Touch) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			m := *(**data.Measurement[float64])(arriving)

			if m.Err != nil {
				if !yield(arriving) {
					return
				}

				continue
			}

			state := op.paths[m.Label]

			if state == nil {
				state = &path{
					estimator: statistic.NewJoint(3),
					velocity: [3]core.Primitive{
						statistic.NewLocalRegression(),
						statistic.NewLocalRegression(),
						statistic.NewLocalRegression(),
					},
				}
				op.paths[m.Label] = state
			}

			if state.at != 0 && m.At.UnixNano() < state.at {
				m.Provenance = map[string]string{"event_time_state": "regressed"}

				if !yield(arriving) {
					return
				}

				continue
			}

			bid, ask := m.Metrics["bid"].Raw, m.Metrics["ask"].Raw
			bidQty, askQty := m.Metrics["bid_qty"].Raw, m.Metrics["ask_qty"].Raw
			bidNotional, askNotional := bid*bidQty, ask*askQty
			midpoint := (bid + ask) / 2
			spread := ask - bid
			relative := spread / midpoint

			m.Metrics["best_bid_price"] = m.Metrics["best_bid_price"].Write(bid)
			m.Metrics["best_ask_price"] = m.Metrics["best_ask_price"].Write(ask)
			m.Metrics["touch_quantity:bid"] = m.Metrics["touch_quantity:bid"].Write(bidQty)
			m.Metrics["touch_quantity:ask"] = m.Metrics["touch_quantity:ask"].Write(askQty)
			m.Metrics["touch_notional:bid"] = m.Metrics["touch_notional:bid"].Write(bidNotional)
			m.Metrics["touch_notional:ask"] = m.Metrics["touch_notional:ask"].Write(askNotional)
			m.Metrics["midpoint"] = m.Metrics["midpoint"].Write(midpoint)
			m.Metrics["spread"] = m.Metrics["spread"].Write(spread)
			m.Metrics["relative_spread"] = m.Metrics["relative_spread"].Write(relative)
			m.Metrics["two_sided_touch_notional"] = m.Metrics["two_sided_touch_notional"].Write(math.Min(bidNotional, askNotional))
			m.Metrics["touch_notional_imbalance"] = m.Metrics["touch_notional_imbalance"].Write((bidNotional - askNotional) / (bidNotional + askNotional))

			logged := []float64{math.Log(bidNotional), math.Log(askNotional), math.Log(relative)}
			originals := []float64{bidNotional, askNotional, relative}
			reading := drive[statistic.JointInput, statistic.JointReading](state.estimator, &statistic.JointInput{Values: logged})

			if err := state.estimator.Error(); err != nil {
				m.Err = errors.Join(m.Err, err)
				op.Error(err)

				if !yield(arriving) {
					return
				}

				continue
			}

			m.Metadata[data.MetadataSupport] = reading.Channels[0].Count

			if reading.SNRDefined {
				m.Metadata[data.MetadataMahalanobisSNR] = reading.SNR
			}

			failed := false

			for index := range reading.Channels {
				channel := reading.Channels[index]

				if !channel.HasPrior {
					continue
				}

				m.Metrics[baselineLabels[index]] = m.Metrics[baselineLabels[index]].Write(channel.Baseline)
				m.Metrics[ratioLabels[index]] = m.Metrics[ratioLabels[index]].Write(originals[index] / channel.Baseline)
				m.Metrics[divergenceLabels[index]] = m.Metrics[divergenceLabels[index]].Write(channel.Residual)

				if channel.ScoreScale > 0 {
					m.Metrics[noiseLabels[index]] = m.Metrics[noiseLabels[index]].Write(channel.ScoreScale)
					m.Metrics[zscoreLabels[index]] = m.Metrics[zscoreLabels[index]].Write(channel.ZScore)
				}

				observation := temporal.Price{At: m.At.UnixNano(), Value: channel.Residual}
				summary := drive[temporal.Price, statistic.LocalRegressionReading](state.velocity[index], &observation)

				if err := state.velocity[index].Error(); err != nil {
					m.Err = errors.Join(m.Err, err)
					op.Error(err)
					failed = true

					break
				}

				if summary.SlopeDefined {
					m.Metrics[velocityLabels[index]] = m.Metrics[velocityLabels[index]].Write(summary.Slope)
				}

				if summary.SNRDefined {
					m.Metrics[velocitySNRLabels[index]] = m.Metrics[velocitySNRLabels[index]].Write(summary.SNR)
				}
			}

			if failed {
				if !yield(arriving) {
					return
				}

				continue
			}

			state.at = m.At.UnixNano()

			if !yield(arriving) {
				return
			}
		}
	}
}

func (op *Touch) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}
