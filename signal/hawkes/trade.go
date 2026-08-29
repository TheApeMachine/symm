package hawkes

import (
	"fmt"
	"time"

	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/algo"
	"github.com/theapemachine/symm/nomagique/data"
	nmhawkes "github.com/theapemachine/symm/nomagique/statistic/hawkes"
	nmtypes "github.com/theapemachine/symm/nomagique/types"
)

/*
Trade is the arrival-dynamics market entity. It owns exactly a Number
pipeline and a projector: every Hawkes fact — empirical counts, pre-arrival
intensity, exact likelihood, branching, excitation, compensator, the fitted
MLE model, and the retained arrival history — lives inside the Frame that
Number commits per symbol. Trade injects only the current event's raw
observation facts (mark, event time) and reads back whatever the composed
algo.Hawkes() pipeline populated.
*/
type Trade struct {
	number    *nomagique.Number[string]
	projector *data.Projector
}

/*
NewTrade constructs the Trade entity: one Number pipeline running the fully
composed Hawkes model (nomagique/algo.Hawkes) and one projector that names
its output slots per signal/hawkes/README.md.
*/
func NewTrade() *Trade {
	return &Trade{
		number: nomagique.NewNumber[string](algo.Hawkes()),
		projector: data.NewProjector(
			data.Binding{From: nmhawkes.SymbolEventCount, Name: "event_count", Unit: data.UnitCount, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: nmhawkes.SymbolEventCountBuy, Name: "event_count:buy", Unit: data.UnitCount, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: nmhawkes.SymbolEventCountSell, Name: "event_count:sell", Unit: data.UnitCount, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: nmhawkes.SymbolEventFracBuy, Name: "event_fraction:buy", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: nmhawkes.SymbolEventFracSell, Name: "event_fraction:sell", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},

			data.Binding{From: nmhawkes.SymbolArrivalRate, Name: "arrival_rate", Unit: data.UnitPerSecond, Timescale: data.TimescalePerSecond},
			data.Binding{From: nmhawkes.SymbolArrivalRateBuy, Name: "arrival_rate:buy", Unit: data.UnitPerSecond, Timescale: data.TimescalePerSecond},
			data.Binding{From: nmhawkes.SymbolArrivalRateSell, Name: "arrival_rate:sell", Unit: data.UnitPerSecond, Timescale: data.TimescalePerSecond},

			data.Binding{From: nmhawkes.SymbolConditionalIntensity, Name: "conditional_intensity", Unit: data.UnitPerSecond, Timescale: data.TimescalePerSecond},
			data.Binding{From: nmhawkes.SymbolConditionalIntensityBuy, Name: "conditional_intensity:buy", Unit: data.UnitPerSecond, Timescale: data.TimescalePerSecond},
			data.Binding{From: nmhawkes.SymbolConditionalIntensitySell, Name: "conditional_intensity:sell", Unit: data.UnitPerSecond, Timescale: data.TimescalePerSecond},
			data.Binding{From: nmhawkes.SymbolBackgroundRate, Name: "background_rate", Unit: data.UnitPerSecond, Timescale: data.TimescalePerSecond},
			data.Binding{From: nmhawkes.SymbolBackgroundRateBuy, Name: "background_rate:buy", Unit: data.UnitPerSecond, Timescale: data.TimescalePerSecond},
			data.Binding{From: nmhawkes.SymbolBackgroundRateSell, Name: "background_rate:sell", Unit: data.UnitPerSecond, Timescale: data.TimescalePerSecond},

			data.Binding{From: nmhawkes.SymbolExcitationIntensityBuy, Name: "excitation_intensity:buy", Unit: data.UnitPerSecond, Timescale: data.TimescalePerSecond},
			data.Binding{From: nmhawkes.SymbolExcitationIntensitySell, Name: "excitation_intensity:sell", Unit: data.UnitPerSecond, Timescale: data.TimescalePerSecond},
			data.Binding{From: nmhawkes.SymbolExcitationFractionBuy, Name: "excitation_fraction:buy", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: nmhawkes.SymbolExcitationFractionSell, Name: "excitation_fraction:sell", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},

			data.Binding{From: nmhawkes.SymbolExcitationAmplitudeBB, Name: "excitation_amplitude:buy_from_buy", Unit: data.UnitPerSecond, Timescale: data.TimescalePerSecond},
			data.Binding{From: nmhawkes.SymbolExcitationAmplitudeBS, Name: "excitation_amplitude:buy_from_sell", Unit: data.UnitPerSecond, Timescale: data.TimescalePerSecond},
			data.Binding{From: nmhawkes.SymbolExcitationAmplitudeSB, Name: "excitation_amplitude:sell_from_buy", Unit: data.UnitPerSecond, Timescale: data.TimescalePerSecond},
			data.Binding{From: nmhawkes.SymbolExcitationAmplitudeSS, Name: "excitation_amplitude:sell_from_sell", Unit: data.UnitPerSecond, Timescale: data.TimescalePerSecond},

			// One fitted decay rate is constrained common (beta_xy = beta); see
			// Excitation's doc comment. All four README names alias it rather
			// than fabricating four independent degrees of freedom.
			data.Binding{From: nmhawkes.SymbolExcitationDecay, Name: "excitation_decay:buy_from_buy", Unit: data.UnitPerSecond, Timescale: data.TimescalePerSecond},
			data.Binding{From: nmhawkes.SymbolExcitationDecay, Name: "excitation_decay:buy_from_sell", Unit: data.UnitPerSecond, Timescale: data.TimescalePerSecond},
			data.Binding{From: nmhawkes.SymbolExcitationDecay, Name: "excitation_decay:sell_from_buy", Unit: data.UnitPerSecond, Timescale: data.TimescalePerSecond},
			data.Binding{From: nmhawkes.SymbolExcitationDecay, Name: "excitation_decay:sell_from_sell", Unit: data.UnitPerSecond, Timescale: data.TimescalePerSecond},
			data.Binding{From: nmhawkes.SymbolExcitationTimescale, Name: "excitation_timescale:buy_from_buy", Unit: data.UnitSecond, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: nmhawkes.SymbolExcitationTimescale, Name: "excitation_timescale:buy_from_sell", Unit: data.UnitSecond, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: nmhawkes.SymbolExcitationTimescale, Name: "excitation_timescale:sell_from_buy", Unit: data.UnitSecond, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: nmhawkes.SymbolExcitationTimescale, Name: "excitation_timescale:sell_from_sell", Unit: data.UnitSecond, Timescale: data.TimescaleInstantaneous},

			data.Binding{From: nmhawkes.SymbolOffspringBuyFromBuy, Name: "offspring:buy_from_buy", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: nmhawkes.SymbolOffspringBuyFromSell, Name: "offspring:buy_from_sell", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: nmhawkes.SymbolOffspringSellFromBuy, Name: "offspring:sell_from_buy", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: nmhawkes.SymbolOffspringSellFromSell, Name: "offspring:sell_from_sell", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: nmhawkes.SymbolSpectralRadius, Name: "branching_spectral_radius", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: nmhawkes.SymbolDescendantsFromBuy, Name: "expected_descendants_from_buy", Unit: data.UnitCount, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: nmhawkes.SymbolDescendantsFromSell, Name: "expected_descendants_from_sell", Unit: data.UnitCount, Timescale: data.TimescaleInstantaneous},

			data.Binding{From: nmhawkes.SymbolLLHawkesTotal, Name: "log_likelihood:hawkes", Unit: data.UnitNat, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: nmhawkes.SymbolLLPoissonTotal, Name: "log_likelihood:poisson", Unit: data.UnitNat, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: nmhawkes.SymbolLLSelfTotal, Name: "log_likelihood:self_only", Unit: data.UnitNat, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: nmhawkes.SymbolLLHawkesPerEvent, Name: "log_likelihood_per_event:hawkes", Unit: data.UnitNat, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: nmhawkes.SymbolLLGainPoisson, Name: "log_likelihood_gain_vs_poisson", Unit: data.UnitNat, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: nmhawkes.SymbolLLGainSelf, Name: "log_likelihood_gain_vs_self_only", Unit: data.UnitNat, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: nmhawkes.SymbolLLGainPoissonPer, Name: "log_likelihood_gain_per_event_vs_poisson", Unit: data.UnitNat, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: nmhawkes.SymbolLLGainSelfPer, Name: "log_likelihood_gain_per_event_vs_self_only", Unit: data.UnitNat, Timescale: data.TimescaleInstantaneous},

			data.Binding{From: nmhawkes.SymbolCompensatorBuy, Name: "compensator:buy", Unit: data.UnitCount, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: nmhawkes.SymbolCompensatorSell, Name: "compensator:sell", Unit: data.UnitCount, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: nmhawkes.SymbolInnovationBuy, Name: "count_innovation:buy", Unit: data.UnitCount, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: nmhawkes.SymbolInnovationSell, Name: "count_innovation:sell", Unit: data.UnitCount, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: nmhawkes.SymbolStandardInnovBuy, Name: "standardized_innovation:buy", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: nmhawkes.SymbolStandardInnovSell, Name: "standardized_innovation:sell", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: nmhawkes.SymbolSNR, Name: "snr", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
		),
	}
}

/*
Step receives one trade, injects its mark and event time as the only raw
observation facts, runs the Number pipeline, and projects exactly one
Measurement. No fitting, history inspection, or refit decision happens here;
algo.Hawkes owns all of that inside the committed Frame.
*/
func (trade *Trade) Step(observation kraken.TradeData) *data.Measurement[float64] {
	if observation.Side != "buy" && observation.Side != "sell" {
		return &data.Measurement[float64]{Err: fmt.Errorf(
			"hawkes: unsupported trade side %q", observation.Side,
		)}
	}

	input := nmtypes.Frame{}
	input.Put(nmhawkes.SymbolMark, markForSide(observation.Side))
	input.Put(nmtypes.EventTimeSec, float64(observation.Timestamp.Unix()))
	input.Put(nmtypes.EventTimeNsec, float64(observation.Timestamp.Nanosecond()))

	output := trade.number.Step(observation.Symbol, input)

	from := observation.Timestamp

	if fromSec, found := output.Get(nmhawkes.SymbolFromSec); found {
		wholeSeconds := int64(fromSec)
		nanoseconds := int64((fromSec - float64(wholeSeconds)) * 1e9)
		from = time.Unix(wholeSeconds, nanoseconds)
	}

	// Maturity (README section 5.4) measures effective FITTED MODEL support,
	// not raw market-event count: before any fit has converged the model has
	// zero support regardless of how many trades have been observed, and
	// after a fit the support is the event count that fit's own optimizer
	// context judged sufficient (nmhawkes.SymbolModelSupport), which is not
	// the same number as every event this signal has ever seen.
	if support, found := output.Get(nmhawkes.SymbolModelSupport); found {
		output.Put(nmtypes.SampleCount, support)
	} else {
		output.Put(nmtypes.SampleCount, 0)
	}

	// Projector.Project only derives Measurement.SNR/SNRDefined from a
	// "mahalanobis/snr" Frame fact; relay the Hawkes-specific SNR (README
	// section 20) onto that exact slot so the measurement envelope reflects
	// it, in addition to it being separately projected as the "snr" metric.
	if snr, found := output.Get(nmhawkes.SymbolSNR); found {
		output.Put(nmtypes.MustIntern("mahalanobis/snr"), snr)
	}

	return trade.projector.Project(observation.Symbol, "hawkes", observation.Timestamp, from, output)
}

func (trade *Trade) Close() error { return nil }

/*
markForSide encodes one trade's aggressor side into the process mark: buys
are the positive (buy-channel) mark, every other side the negative
(sell-channel) mark.
*/
func markForSide(side string) float64 {
	if side == "buy" {
		return 1
	}

	return -1
}
