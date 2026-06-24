package trader

import (
	"cmp"
	"context"
	"slices"
	"strings"
	"sync/atomic"
	"time"

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
Signal replays ingest frames from the tree for each spectrum signal's declared roles.
Websocket ingest keys frames by timestamp; Measure receives matching role/scope rows.
*/
type Signal struct {
	ctx           context.Context
	cancel        context.CancelFunc
	pool          *qpool.Q[any]
	tree          *dmt.Tree
	signals       map[logic.SourceType]market.Signal
	lastTimestamp atomic.Pointer[time.Time]
}

func NewSignal(
	ctx context.Context,
	pool *qpool.Q[any],
	tree *dmt.Tree,
) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	crossSection, err := market.NewCrossSection(market.DefaultCrossSectionConfig())

	if err != nil {
		cancel()

		return nil
	}

	return &Signal{
		ctx:    ctx,
		cancel: cancel,
		pool:   pool,
		tree:   tree,
		signals: map[logic.SourceType]market.Signal{
			logic.SourceHawkes:      hawkes.NewSignal(ctx, pool, tree),
			logic.SourceFluid:       fluid.NewSignal(ctx, pool, tree),
			logic.SourcePumpDump:    pumpdump.NewSignal(ctx, pool, tree),
			logic.SourceCausal:      causal.NewSignal(ctx, pool, tree, crossSection),
			logic.SourceDepthFlow:   depthflow.NewSignal(ctx, pool, tree),
			logic.SourceLeadLag:     leadlag.NewSignal(ctx, pool, tree),
			logic.SourceLiquidity:   liquidity.NewSignal(ctx, pool, tree, crossSection),
			logic.SourceSentiment:   sentiment.NewSignal(ctx, pool, tree, crossSection),
			logic.SourceToxicity:    toxicity.NewSignal(ctx, pool, tree),
			logic.SourceCorrelation: correlation.NewSignal(ctx, pool, tree, crossSection),
			logic.SourceExhaustion:  exhaust.NewSignal(ctx, pool, tree),
			logic.SourceCVD:         cvd.NewSignal(ctx, pool, tree),
			logic.SourceManifold:    manifold.NewSignal(ctx, pool, tree),
		},
	}
}

func (signal *Signal) Measure() []*datura.Artifact {
	measurements := make([]*datura.Artifact, 0)
	last := signal.lastTimestamp.Load()
	now := time.Now().UTC()

	signal.lastTimestamp.Store(&now)

	endSecond := now.Truncate(time.Second)
	startSecond := endSecond

	if last != nil {
		startSecond = last.UTC().Truncate(time.Second)
	}

	sources := make([]logic.SourceType, 0, len(signal.signals))

	for source := range signal.signals {
		sources = append(sources, source)
	}

	slices.SortFunc(sources, func(left, right logic.SourceType) int {
		return cmp.Compare(string(left), string(right))
	})

	for second := startSecond; !second.After(endSecond); second = second.Add(time.Second) {
		seekPrefix := strings.ReplaceAll(
			strings.ReplaceAll(second.Format("2006/01/02 15:04:05"), " ", "/"),
			":", "/",
		)

		for _, source := range sources {
			sig := signal.signals[source]

			for _, role := range sig.IngestRoles() {
				for artifact := range signal.tree.Seek([]byte(seekPrefix)) {
					if !market.IngestFrameMatches(artifact, role) {
						continue
					}

					measured := sig.Measure(artifact)

					if measured == nil {
						continue
					}

					measured.WithRole("measurement")
					measured.WithOrigin(string(source))
					measurements = append(measurements, measured)
				}
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
