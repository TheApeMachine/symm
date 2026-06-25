package trader

import (
	"context"
	"slices"
	"strings"
	"time"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/errnie"
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
Ingest keys frames by role/scope/timestamp; Measure receives matching rows once.
*/
type Signal struct {
	ctx           context.Context
	cancel        context.CancelFunc
	pool          *qpool.Q[any]
	tree          *dmt.Tree
	signals       map[logic.SourceType]market.Signal
	lastTimestamp int64
}

/*
NewSignal constructs the signal.
*/
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
		signals: map[logic.SourceType]market.Signal{
			logic.SourceHawkes:      hawkes.NewSignal(ctx, tree),
			logic.SourceFluid:       fluid.NewSignal(ctx, pool, tree),
			logic.SourcePumpDump:    pumpdump.NewSignal(ctx, tree),
			logic.SourceCausal:      causal.NewSignal(ctx, tree),
			logic.SourceDepthFlow:   depthflow.NewSignal(ctx, tree),
			logic.SourceLeadLag:     leadlag.NewSignal(ctx, tree),
			logic.SourceLiquidity:   liquidity.NewSignal(ctx, tree),
			logic.SourceSentiment:   sentiment.NewSignal(ctx, tree),
			logic.SourceToxicity:    toxicity.NewSignal(ctx, tree),
			logic.SourceCorrelation: correlation.NewSignal(ctx, tree),
			logic.SourceExhaustion:  exhaust.NewSignal(ctx, tree),
			logic.SourceCVD:         cvd.NewSignal(ctx, tree),
			logic.SourceManifold:    manifold.NewSignal(ctx, tree),
		},
		lastTimestamp: 0,
	}
}

/*
Measure seeks the tree by role and timestamp, which is how the
websocket stores frames. This allows us to implement a simple
cursor mechanism without a lot of complexity.
*/
func (signal *Signal) Measure(crossSection *market.CrossSection) []*datura.Artifact {
	measurements := make([]*datura.Artifact, 0)

	prev := signal.lastTimestamp
	now := time.Now().UTC().UnixNano()
	signal.lastTimestamp = now

	t := prev
	if t == 0 {
		prev = 1
		t = now
	}

	// Slice off the seconds from the timestamp, so we don't miss
	// frames that arrive during processing.
	h1 := strings.Join(strings.Split(
		datura.FormatTimestamp(t), "/",
	)[0:4], "/")

	h2 := strings.Join(strings.Split(
		datura.FormatTimestamp(now), "/",
	)[0:4], "/")

	cursors := []string{h2}
	if h1 != h2 {
		cursors = append(cursors, h1)
	}

	// Iterate over all the roles that signals look at.
	for _, role := range []string{"book", "trade", "ticker"} {
		for _, cursor := range cursors {
			// Start by seeking the tree, so we do this only once per role.
			for artifact := range signal.tree.Seek([]byte(role + "/" + cursor)) {
				// If we already saw this frame, skip it.
				if artifact.Timestamp() <= prev {
					continue
				}

				// Iterate over all the signals that look at this role.
				for origin, sig := range signal.signals {
					// If this signal doesn't look at this role, skip it.
					if !slices.Contains(sig.IngestRoles(), role) {
						continue
					}

					for measured := range sig.Measure(artifact, crossSection) {
						if measured == nil {
							errnie.Error(errnie.Err(
								errnie.Validation,
								"trader: signal returned nil measurement",
								nil,
							))

							continue
						}

						measurements = append(
							measurements, measured.WithOrigin(
								string(origin),
							).WithRole(
								"measurement",
							),
						)
					}
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
