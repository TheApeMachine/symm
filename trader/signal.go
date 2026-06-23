package trader

import (
	"context"
	"sort"
	"sync"

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

type wiredSignal struct {
	measurer market.Signal
	origin   logic.SourceType
}

type MeasureStats struct {
	Rows         int
	Measurements int
	Calibrating  int
}

/*
Signal runs every spectrum signal against the ingest roles it declares.
Each signal advances its own tree cursor so Measure only replays new frames.
*/
type Signal struct {
	ctx            context.Context
	cancel         context.CancelFunc
	pool           *qpool.Q[any]
	tree           *dmt.Tree
	signals        []wiredSignal
	measureCursors sync.Map
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
		signals: []wiredSignal{
			{hawkes.NewSignal(ctx, pool, tree), logic.SourceHawkes},
			{fluid.NewSignal(ctx, pool, tree), logic.SourceFluid},
			{pumpdump.NewSignal(ctx, pool, tree), logic.SourcePumpDump},
			{causal.NewSignal(ctx, pool, tree), logic.SourceCausal},
			{depthflow.NewSignal(ctx, pool, tree), logic.SourceDepthFlow},
			{leadlag.NewSignal(ctx, pool, tree), logic.SourceLeadLag},
			{liquidity.NewSignal(ctx, pool, tree), logic.SourceLiquidity},
			{sentiment.NewSignal(ctx, pool, tree), logic.SourceSentiment},
			{toxicity.NewSignal(ctx, pool, tree), logic.SourceToxicity},
			{correlation.NewSignal(ctx, pool, tree), logic.SourceCorrelation},
			{exhaust.NewSignal(ctx, pool, tree), logic.SourceExhaustion},
			{cvd.NewSignal(ctx, pool, tree), logic.SourceCVD},
			{manifold.NewSignal(ctx, pool, tree), logic.SourceManifold},
		},
	}
}

func (signal *Signal) Measure() []*datura.Artifact {
	measurements := make([]*datura.Artifact, 0)

	signal.MeasureEach(func(artifact *datura.Artifact) {
		measurements = append(measurements, artifact)
	})

	return measurements
}

func (signal *Signal) MeasureEach(emit func(*datura.Artifact)) {
	for _, wired := range signal.signals {
		for _, role := range wired.measurer.IngestRoles() {
			cursorKey := string(wired.origin) + "/" + role
			cursorValue, _ := signal.measureCursors.Load(cursorKey)
			lastTimestamp, _ := cursorValue.(int64)
			artifacts := make([]*datura.Artifact, 0)

			for artifact := range signal.tree.Seek([]byte(role + "/update")) {
				artifacts = append(artifacts, artifact)
			}

			sort.SliceStable(artifacts, func(left, right int) bool {
				return artifacts[left].Timestamp() < artifacts[right].Timestamp()
			})

			newestTimestamp := lastTimestamp

			for _, artifact := range artifacts {
				timestamp := artifact.Timestamp()

				if timestamp <= lastTimestamp {
					continue
				}

				if timestamp > newestTimestamp {
					newestTimestamp = timestamp
				}

				measurement := wired.measurer.Measure(artifact)

				if measurement != nil {
					measurement.WithRole("measurement")
					_ = measurement.SetOrigin(string(wired.origin))

					if emit != nil {
						emit(measurement)
					}
				}
			}

			if newestTimestamp > lastTimestamp {
				signal.measureCursors.Store(cursorKey, newestTimestamp)
			}
		}
	}
}

func (signal *Signal) Close() error {
	signal.cancel()

	for _, wired := range signal.signals {
		wired.measurer.Close()
	}

	return nil
}
