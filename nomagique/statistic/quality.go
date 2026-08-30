package statistic

import (
	"github.com/theapemachine/symm/nomagique/temporal"
	"github.com/theapemachine/symm/nomagique/types"
)

/*
Quality slots are the estimator facts data.Measurement.Finalize consumes to
derive the scalar signal-to-noise ratio (spec §7.1): SNR = divergence² /
noise_variance. They are deliberately unprefixed — a measurement carries one
quality verdict, whichever of its estimators is the headline one, and
data.Projector reads exactly these two names out of the evaluated frame.
*/
var (
	SymbolDivergence    = types.MustIntern("divergence")
	SymbolNoiseVariance = types.MustIntern("noise_variance")
)

/*
QualityFrom projects one namespaced ZScore estimator's own residual and
dispersion onto the quality slots, so the measurement that carries it reports a
real SNR instead of leaving SNRDefined false.

The estimator's departure from its own causal baseline is the divergence, and
the noise power is that estimator's decayed dispersion squared — the same
identity depthflow, derivatives, and pumpdump already wire by hand.

The projection is gated on the estimator's own readiness: ZScore reports not
ready until a baseline exists, and until then there is no residual to speak of.
An unready estimator leaves both slots absent rather than writing a zero, which
keeps "no noise model yet" distinguishable from a genuine zero departure — the
distinction Finalize's SNRDefined exists to preserve.
*/
func QualityFrom(prefix string) types.Primitive {
	series := temporal.NewSeries(prefix)
	slots := newZScoreSlots(prefix)

	return func(input *types.Frame) {
		ready, hasReady := input.Get(series.ReadySymbol)

		if !hasReady || ready == 0 {
			return
		}

		residual, hasResidual := input.Get(slots.residual)
		dispersion, hasDispersion := input.Get(slots.dispersion)

		if !hasResidual || !hasDispersion {
			return
		}

		input.Put(SymbolDivergence, residual)
		input.Put(SymbolNoiseVariance, dispersion*dispersion)
	}
}
