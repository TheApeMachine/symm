package trader

import (
	"context"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/market"
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
	"github.com/theapemachine/symm/signal/pumpdump"
	"github.com/theapemachine/symm/signal/sentiment"
	"github.com/theapemachine/symm/signal/toxicity"
)

/*
Signal runs every spectrum signal against ingest rows each desk tick.
*/
type Signal struct {
	ctx     context.Context
	cancel  context.CancelFunc
	pool    *qpool.Q[any]
	tree    *dmt.Tree
	signals []market.Signal
}

func NewSignal(
	ctx context.Context,
	pool *qpool.Q[any],
	tree *dmt.Tree,
) *Signal {
	return &Signal{
		signals: []market.Signal{
			causal.NewSignal(ctx, pool, tree),
			correlation.NewSignal(ctx, pool, tree),
			cvd.NewSignal(ctx, pool, tree),
			depthflow.NewSignal(ctx, pool, tree),
			exhaust.NewSignal(ctx, pool, tree),
			fluid.NewSignal(ctx, pool, tree),
			hawkes.NewSignal(ctx, pool, tree),
			leadlag.NewSignal(ctx, pool, tree),
			liquidity.NewSignal(ctx, pool, tree),
			manifold.NewSignal(ctx, pool, tree),
			pumpdump.NewSignal(ctx, pool, tree),
			sentiment.NewSignal(ctx, pool, tree),
			toxicity.NewSignal(ctx, pool, tree),
		},
	}
}

func (signal *Signal) Measure(artifact *datura.Artifact) []*datura.Artifact {
	if signal == nil {
		return nil
	}

	measurements := make([]*datura.Artifact, 13)

	for idx, sig := range signal.signals {
		artifact = sig.Measure(artifact)

		if artifact == nil {
			continue
		}

		signal.tree.Insert(artifact.Prefix(), errnie.Does(func() ([]byte, error) {
			return artifact.Pack()
		}).Or(func(err error) {
			errnie.Error(errnie.Err(
				errnie.Validation,
				"signal: failed to marshal artifact",
				err,
			))
		}).Value())

		measurements[idx] = artifact
	}

	return measurements
}

func (signal *Signal) Close() error {
	for _, sig := range signal.signals {
		sig.Close()
	}

	return nil
}
