package trader

import (
	"context"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/logic"
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
	sources []logic.SourceType
}

func NewSignal(
	ctx context.Context,
	pool *qpool.Q[any],
	tree *dmt.Tree,
) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	return &Signal{
		ctx:    ctx,
		cancel: cancel,
		pool:   pool,
		tree:   tree,
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
		sources: []logic.SourceType{
			logic.SourceCausal,
			logic.SourceCorrelation,
			logic.SourceCVD,
			logic.SourceDepthFlow,
			logic.SourceExhaustion,
			logic.SourceFluid,
			logic.SourceHawkes,
			logic.SourceLeadLag,
			logic.SourceLiquidity,
			logic.SourceManifold,
			logic.SourcePumpDump,
			logic.SourceSentiment,
			logic.SourceToxicity,
		},
	}
}

func (signal *Signal) Measure() []*datura.Artifact {
	measurements := make([]*datura.Artifact, 0, len(signal.signals))

	for _, sig := range signal.signals {
		for _, role := range []string{"ticker", "book", "trade", "ohlc"} {
			for stored := range signal.tree.Seek([]byte(role + "/update")) {
				measurement := sig.Measure(stored)

				if measurement == nil {
					continue
				}

				measurements = append(measurements, measurement)
			}
		}
	}

	return measurements
}

func (signal *Signal) Close() error {
	signal.cancel()

	for _, sig := range signal.signals {
		sig.Close()
	}

	return nil
}
