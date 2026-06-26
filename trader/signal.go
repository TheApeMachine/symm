package trader

import (
	"context"
	"sort"
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
	ctx                 context.Context
	cancel              context.CancelFunc
	pool                *qpool.Q[any]
	tree                *dmt.Tree
	signals             map[logic.SourceType]market.Signal
	lastTimestamp       int64
	lastTimestampByRole map[string]int64
	lastRoleCount       map[string]int
	cachedFramesByRole  map[string][]*datura.Artifact
	cachedMaxSeenByRole map[string]int64
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
		logic.SourceManifold:    manifold.NewSignal(ctx, tree),
	}

	return &Signal{
		ctx:                 ctx,
		cancel:              cancel,
		pool:                pool,
		tree:                tree,
		signals:             signals,
		lastTimestamp:       0,
		lastTimestampByRole: make(map[string]int64),
		lastRoleCount:       make(map[string]int),
	}
}

/*
Observe builds the cross-section peer snapshot for this tick. It runs once,
single-threaded, before Measure fans out — so every signal reads a complete,
consistent peer context instead of each racily Observing the same ticker rows
from its own goroutine. Re-observing recent rows is idempotent: the cross-section
caps history and overwrites per-symbol state.
*/
func (signal *Signal) Observe(crossSection *market.CrossSection) {
	if crossSection == nil {
		return
	}

	// Warm the frame cache on the first phase of this tick cycle.
	// This consolidates subsequent reads in Measure into simple cache lookups.
	signal.loadFrames()

	// Feed the cached ticker frames to the cross-section snapshot
	tickerFrames := signal.cachedFramesByRole["ticker"]
	for _, artifact := range tickerFrames {
		for rowIndex := 0; ; rowIndex++ {
			row, err := market.SymbolFromTicker(artifact, rowIndex)
			if err != nil {
				break
			}
			errnie.Error(crossSection.Observe(row))
		}
	}

	// Record one market-wide breadth sample per tick, after the full universe
	// has been observed — breadth is a cross-sectional reading, not per-symbol.
	crossSection.RecordBreadth(crossSection.Breadth(time.Time{}))

	// Warm the peer-snapshot cache single-threaded so the concurrent signal
	// reads in Measure are pure lookups and never mutate the cross-section.
	crossSection.WarmPeers(crossSection.MinBarsRequired())
}

func (signal *Signal) loadRoleFrames(role string, prev int64) ([]*datura.Artifact, int64) {
	roleArtifacts := make([]*datura.Artifact, 0)
	maxSeen := prev

	// Seek by the role prefix alone, not by a timestamp-granular key. Tree keys
	// stamp time only to the second (datura.FormatTimestamp), so a key like
	// "ticker/2026/06/26/07/23/05" has prefix granularity of one second. Seeking
	// with that key makes tree.Seek's HasPrefix guard stop at the end of that
	// single second — it never reaches later seconds, so once the cursor lands in
	// a second it can never advance and every subsequent tick loads zero frames.
	// The role prefix returns the whole role in sorted (time) order; the cursor
	// filter below keeps only frames strictly newer than the last seen timestamp.
	//
	// ponytail: O(frames-this-role) scan per tick. Acceptable while the universe
	// is bounded (market.max_symbols). Upgrade path: a sub-second-granular or
	// monotonic key suffix so Seek can resume from the exact cursor without a
	// full-role prefix walk.
	seekKey := []byte(role + "/")

	for artifact := range signal.tree.Seek(seekKey) {
		artifactRole, err := artifact.Role()
		if err != nil || artifactRole != role {
			break
		}

		if artifact.Timestamp() <= prev {
			continue
		}

		if timestamp := artifact.Timestamp(); timestamp > maxSeen {
			maxSeen = timestamp
		}

		roleArtifacts = append(roleArtifacts, artifact)
	}

	sort.Slice(roleArtifacts, func(indexA, indexB int) bool {
		return roleArtifacts[indexA].Timestamp() < roleArtifacts[indexB].Timestamp()
	})

	return roleArtifacts, maxSeen
}

func coalesceRoleFrames(role string, artifacts []*datura.Artifact) []*datura.Artifact {
	if role != "book" || len(artifacts) < 2 {
		return artifacts
	}

	// ponytail: book floods can carry thousands of same-symbol updates in one
	// trader tick. Score the latest book row per symbol; the raw tree remains
	// the upgrade path for signals that need intra-tick order-book churn.
	latestBySymbol := make(map[string]*datura.Artifact)
	unknown := make([]*datura.Artifact, 0)

	for _, artifact := range artifacts {
		symbol := datura.Peek[string](artifact, "data", 0, "symbol")
		if symbol == "" {
			unknown = append(unknown, artifact)
			continue
		}

		if prior, ok := latestBySymbol[symbol]; !ok || artifact.Timestamp() >= prior.Timestamp() {
			latestBySymbol[symbol] = artifact
		}
	}

	coalesced := make([]*datura.Artifact, 0, len(latestBySymbol)+len(unknown))
	coalesced = append(coalesced, unknown...)

	for _, artifact := range latestBySymbol {
		coalesced = append(coalesced, artifact)
	}

	sort.Slice(coalesced, func(indexA, indexB int) bool {
		return coalesced[indexA].Timestamp() < coalesced[indexB].Timestamp()
	})

	return coalesced
}

/*
loadFrames fetches and caches artifacts from the tree across all statically
declared required roles. Caching prevents redundant tree queries between
Observe and Measure in one trader tick.
*/
func (signal *Signal) loadFrames() {
	if signal.cachedFramesByRole != nil {
		return
	}

	// Initialize per-role cursors from global lastTimestamp if manually set by tests on first run.
	if len(signal.lastTimestampByRole) == 0 && signal.lastTimestamp > 0 {
		for _, roleName := range []string{"book", "ticker", "trade", "ohlc"} {
			signal.lastTimestampByRole[roleName] = signal.lastTimestamp
		}
	}

	framesByRole := make(map[string][]*datura.Artifact)
	roleCount := make(map[string]int)
	maxSeenByRole := make(map[string]int64)

	for _, role := range []string{"book", "ticker", "trade", "ohlc"} {
		roleArtifacts, maxSeen := signal.loadRoleFrames(role, signal.lastTimestampByRole[role])
		framesByRole[role] = coalesceRoleFrames(role, roleArtifacts)
		roleCount[role] = len(framesByRole[role])
		maxSeenByRole[role] = maxSeen
	}

	signal.cachedFramesByRole = framesByRole
	signal.lastRoleCount = roleCount
	signal.cachedMaxSeenByRole = maxSeenByRole
}

/*
Measure seeks the tree by role and timestamp, which is how the
websocket stores frames. This allows us to implement a simple
cursor mechanism without a lot of complexity.
*/
func (signal *Signal) Measure(crossSection *market.CrossSection) []*datura.Artifact {
	// Ensure frames are loaded (e.g., if Measure is called directly without Observe)
	signal.loadFrames()

	measurements := make([]*datura.Artifact, 0)

	framesByRole := signal.cachedFramesByRole
	maxSeenByRole := signal.cachedMaxSeenByRole

	// Advance the cursors for each role
	for role, maxSeen := range maxSeenByRole {
		if maxSeen > signal.lastTimestampByRole[role] {
			signal.lastTimestampByRole[role] = maxSeen
		}

		if maxSeen > signal.lastTimestamp {
			signal.lastTimestamp = maxSeen
		}
	}

	// Reset cached frames for the next tick
	signal.cachedFramesByRole = nil
	signal.cachedMaxSeenByRole = nil

	// Each signal owns mutable state. Score one signal at a time and store the
	// measurement artifacts directly, so qpool is not used as a large artifact
	// transport and measurements still persist in the tree.
	for origin, sig := range signal.signals {
		for _, role := range sig.IngestRoles() {
			for _, artifact := range framesByRole[role] {
				for measured := range sig.Measure(artifact, crossSection) {
					if measured == nil {
						errnie.Error(errnie.Err(
							errnie.Validation,
							"trader: signal returned nil measurement",
							nil,
						))

						continue
					}

					measured.WithOrigin(
						string(origin),
					).WithRole(
						"measurement",
					)

					signal.tree.InsertArtifact(measured.Prefix("role", "scope", "origin", "timestamp"), measured)
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

/*
RoleCount returns the count of loaded frames for a role.
*/
func (signal *Signal) RoleCount(role string) int {
	return signal.lastRoleCount[role]
}
