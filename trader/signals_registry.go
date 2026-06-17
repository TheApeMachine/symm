package trader

import (
	"github.com/spf13/viper"
	"github.com/theapemachine/symm/signal/causal"
	"github.com/theapemachine/symm/signal/correlation"
	"github.com/theapemachine/symm/signal/cvd"
	"github.com/theapemachine/symm/signal/depthflow"
	"github.com/theapemachine/symm/signal/exhaust"
	"github.com/theapemachine/symm/signal/fluid"
	"github.com/theapemachine/symm/signal/hawkes"
	"github.com/theapemachine/symm/signal/leadlag"
	"github.com/theapemachine/symm/signal/liquidity"
	"github.com/theapemachine/symm/signal/manifold"
	"github.com/theapemachine/symm/signal/prediction"
	"github.com/theapemachine/symm/signal/pumpdump"
	"github.com/theapemachine/symm/signal/resonance"
	"github.com/theapemachine/symm/signal/sentiment"
	"github.com/theapemachine/symm/signal/toxicity"
)

/*
initSignals constructs every signal once before market subscribers attach.
*/
func (crypto *Crypto) initSignals() {
	crypto.causalSignal = causal.NewSignal(crypto.ctx, crypto.pool)
	crypto.correlationSignal = correlation.NewSignal(crypto.ctx, crypto.pool)
	crypto.cvdSignal = cvd.NewSignal(crypto.ctx, crypto.pool)
	crypto.depthflowSignal = depthflow.NewSignal(crypto.ctx, crypto.pool)
	crypto.exhaustSignal = exhaust.NewSignal(crypto.ctx, crypto.pool)
	crypto.fluidSignal = fluid.NewSignal(crypto.ctx, crypto.pool)
	crypto.hawkesSignal = hawkes.NewSignal(crypto.ctx, crypto.pool)
	crypto.leadlagSignal = leadlag.NewSignal(crypto.ctx, crypto.pool)
	crypto.liquiditySignal = liquidity.NewSignal(crypto.ctx, crypto.pool)
	crypto.manifoldSignal = manifold.NewSignal(crypto.ctx, crypto.pool)
	crypto.predictionSignal = prediction.NewSignal(crypto.ctx, crypto.pool)
	crypto.pumpdumpSignal = pumpdump.NewSignal(crypto.ctx, crypto.pool)
	crypto.sentimentSignal = sentiment.NewSignal(crypto.ctx, crypto.pool)
	crypto.toxicitySignal = toxicity.NewSignal(crypto.ctx, crypto.pool)

	arch := resonance.DefaultArchitecture()

	if configured := viper.GetIntSlice("signals.resonance.arch"); len(configured) >= 2 {
		arch = configured
	}

	alpha := viper.GetFloat64("signals.resonance.alpha")

	if alpha <= 0 {
		alpha = 0.01
	}

	batchSize := viper.GetInt("signals.resonance.batch")

	if batchSize <= 0 {
		batchSize = 128
	}

	crypto.resonanceSignal = resonance.NewSignal(
		crypto.ctx,
		crypto.pool,
		arch,
		alpha,
		batchSize,
	)
}

/*
dashboardSignalNames lists every specialist signal that feeds dashboard gauges.
*/
func (crypto *Crypto) dashboardSignalNames() []string {
	return []string{
		"causal",
		"correlation",
		"cvd",
		"depthflow",
		"exhaust",
		"fluid",
		"hawkes",
		"leadlag",
		"liquidity",
		"manifold",
		"prediction",
		"pumpdump",
		"sentiment",
		"toxicity",
	}
}
