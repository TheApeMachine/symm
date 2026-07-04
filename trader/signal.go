package trader

import (
	"context"
	"slices"
	"sort"
	"sync"
	"time"

	"github.com/spf13/viper"
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
	"github.com/theapemachine/symm/signal/resonance"
	"github.com/theapemachine/symm/signal/sentiment"
	"github.com/theapemachine/symm/signal/toxicity"
)

/*
ingestRoles are the raw tree roles the trader replays into the signals each tick.
level3 carries per-order add/delete/fill events (authenticated ws-l3) and is the
prerequisite for principled toxicity (cancel vs fill), depthflow spoof shape, and
exhaust per-level thinning — L2 book quantity deltas alone cannot tell a cancel
from a fill. A role absent from the tree simply yields no frames, so listing
level3 here is safe before live capture exists.
*/
var ingestRoles = []string{"book", "level3", "ticker", "trade", "ohlc"}

/*
Signal replays ingest frames from the tree for each spectrum signal's declared roles.
Ingest keys frames by role/scope/timestamp; Measure receives matching rows once.
*/
type Signal struct {
	ctx           context.Context
	cancel        context.CancelFunc
	tree          *dmt.Tree
	signals       map[logic.SourceType]market.Signal
	lastTimestamp int64
	pendingMu     sync.Mutex
	pending       []*datura.Artifact
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

	signals := map[logic.SourceType]market.Signal{
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
		logic.SourceManifold:    manifold.NewSignal(ctx, tree, pool),
		logic.SourceResonance: resonance.NewSignal(
			ctx,
			pool,
			tree,
			viper.GetIntSlice("signals.resonance.arch"),
			viper.GetFloat64("signals.resonance.alpha"),
			viper.GetInt("signals.resonance.batch"),
		),
	}

	signal := &Signal{
		ctx:           ctx,
		cancel:        cancel,
		tree:          tree,
		signals:       signals,
		lastTimestamp: time.Now().Unix(),
	}

	if pool != nil {
		for _, role := range ingestRoles {
			pool.Subscribe(role, signal.onMessage)
		}
	}

	return signal
}

func (signal *Signal) onMessage(artifact *datura.Artifact) error {
	if signal == nil || artifact == nil {
		return nil
	}

	if !slices.Contains(ingestRoles, datura.Peek[string](artifact, "role")) {
		return nil
	}

	signal.pendingMu.Lock()
	signal.pending = append(signal.pending, artifact)
	signal.pendingMu.Unlock()

	return nil
}

func (signal *Signal) Measure() []*datura.Artifact {
	last := signal.lastTimestamp
	now := time.Now().Unix() - 1

	if now <= last {
		return nil
	}

	signal.lastTimestamp = now
	var prefixes []string

	for t := last + 1; t <= now; t++ {
		prefixes = append(prefixes, time.Unix(t, 0).UTC().Format("2006/01/02/15/04/05"))
	}

	artifacts := make(map[string][]*datura.Artifact, len(ingestRoles))

	signal.pendingMu.Lock()
	pending := signal.pending
	signal.pending = nil
	for _, artifact := range pending {
		if time.Unix(0, artifact.Timestamp()).Unix() > now {
			signal.pending = append(signal.pending, artifact)
			continue
		}

		role := datura.Peek[string](artifact, "role")
		if slices.Contains(ingestRoles, role) {
			artifacts[role] = append(artifacts[role], artifact)
		}
	}
	signal.pendingMu.Unlock()

	for _, prefix := range prefixes {
		for _, role := range ingestRoles {
			signal.tree.WalkPrefix([]byte(role+"/update/"+prefix), func(key []byte, value []byte) bool {
				artifact := datura.Acquire(
					role, datura.APPJSON,
				)

				if _, err := artifact.Unpack(value); err != nil {
					errnie.Error(errnie.Err(
						errnie.Validation, "trader signal: unpack ingest frame", err,
					))

					return true
				}

				artifacts[role] = append(artifacts[role], artifact)

				return true
			})
		}
	}

	if len(artifacts["book"]) > 1 {
		latest := make(map[string]*datura.Artifact, len(artifacts["book"]))
		book := make([]*datura.Artifact, 0, len(artifacts["book"]))

		for _, artifact := range artifacts["book"] {
			symbol := datura.Peek[string](artifact, "data", 0, "symbol")

			if symbol == "" || datura.Peek[string](artifact, "data", 1, "symbol") != "" {
				book = append(book, artifact)
				continue
			}

			if _, ok := latest[symbol]; !ok {
				book = append(book, artifact)
			}

			latest[symbol] = artifact
		}

		for index, artifact := range book {
			symbol := datura.Peek[string](artifact, "data", 0, "symbol")

			if symbol == "" || datura.Peek[string](artifact, "data", 1, "symbol") != "" {
				continue
			}

			book[index] = latest[symbol]
		}

		artifacts["book"] = book
	}

	// Sort the artifacts by timestamp
	for role := range artifacts {
		sort.Slice(artifacts[role], func(i, j int) bool {
			return artifacts[role][i].Timestamp() < artifacts[role][j].Timestamp()
		})
	}

	crossSection, err := market.NewCrossSection()

	if err != nil {
		errnie.Error(errnie.Err(
			errnie.Validation, "failed to create cross section", err,
		))

		return nil
	}

	crossSection.Observe(artifacts)

	// Record one market-wide breadth sample per tick, after the full universe
	// has been observed — breadth is a cross-sectional reading, not per-symbol.
	crossSection.RecordBreadth(crossSection.Breadth())

	// Warm the peer-snapshot cache single-threaded so the concurrent signal
	// reads in Measure are pure lookups and never mutate the cross-section.
	crossSection.PeerCache.Warm(crossSection, crossSection.MinBarsRequired())

	measurements := make([]*datura.Artifact, 0)
	timeline := make([]*datura.Artifact, 0)
	signalsByRole := make(map[string][]market.Signal, len(ingestRoles))

	for _, sig := range signal.signals {
		for _, role := range sig.IngestRoles() {
			signalsByRole[role] = append(signalsByRole[role], sig)
		}
	}

	for _, role := range ingestRoles {
		timeline = append(timeline, artifacts[role]...)
	}

	sort.Slice(timeline, func(i, j int) bool {
		return timeline[i].Timestamp() < timeline[j].Timestamp()
	})

	for _, artifact := range timeline {
		role := datura.Peek[string](artifact, "role")

		for _, sig := range signalsByRole[role] {
			for measurement := range sig.Measure(artifact, crossSection) {
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
